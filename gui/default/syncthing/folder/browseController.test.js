describe('BrowseController', function() {
    // Set up the module
    beforeEach(module('syncthing.folder'));

    var $controller, $scope, $httpBackend, getIgnoresHandler, getBrowseHandler;
    var controller;

    // Inject angular bits
    beforeEach(inject(function($injector) {
        $httpBackend = $injector.get('$httpBackend');
        // Common controller init requests
        // Tests override these responses where necessary
        getIgnoresHandler = $httpBackend.when('GET', /^rest\/db\/ignores/).respond({ ignore: [] });
        getBrowseHandler = $httpBackend.when('GET', /^rest\/db\/browse/).respond({});

        $controller = $injector.get('$controller');
        $scope = $injector.get('$rootScope');
    }));

    afterEach(function() {
        // Ensure requests are flushed and assertions met
        $httpBackend.verifyNoOutstandingExpectation();
        $httpBackend.verifyNoOutstandingRequest();
    });

    describe('refresh', function() {
        it('does not fetch when current folder is undefined', function() {
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: {} });
            // Without flush(), the test will fail if requests are made.
        });

        it('fetches data when initialized', function() {
            $httpBackend.expectGET('rest/db/ignores?folder=default');
            $httpBackend.expectGET('rest/db/browse?folder=default&levels=0');
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
            $httpBackend.flush();
        });

        it('fetches data when current folder changes', function() {
            var folder = { id: 'default' };
            $httpBackend.expectGET('rest/db/ignores?folder=default');
            $httpBackend.expectGET('rest/db/browse?folder=default&levels=0');
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: folder });
            $httpBackend.flush();
            folder.id = 'documents';
            $httpBackend.expectGET('rest/db/ignores?folder=documents');
            $httpBackend.expectGET('rest/db/browse?folder=documents&levels=0');
            $httpBackend.flush();
        });
    });

    describe('ignores', function() {
        function mockIgnores(ignores) {
            getIgnoresHandler.respond({ ignore: ignores });
            // Initialize the controller, triggering http gets and populating data
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
            $httpBackend.flush();
        }

        it('returns a pattern for each line', function() {
            mockIgnores([
                '/Backups',
                'Photos/**/Raw'
            ]);
            expect(controller.ignorePatterns.length).toEqual(2);
        });

        it('identifies negated patterns', function() {
            mockIgnores(['!/Backups', '(?i)!/Photos/Raw']);
            expect(controller.ignorePatterns.every(function (p) { return p.isNegated })).toBeTrue();

            mockIgnores(['/IMPORTANT FILES!', '(?i)/!Photos/Raw']);
            expect(controller.ignorePatterns.every(function (p) { return !p.isNegated })).toBeTrue();
        });

        describe('identifies simple patterns', function() {
            function isSimple(p) { return p.isSimple; };

            it('by root folder', function() {
                mockIgnores(['/Backups', ' /Photos/Raw', '/']);
                expect(controller.ignorePatterns.every(isSimple)).toBeTrue();
            });

            it('by root wildcard', function() {
                mockIgnores(['*', '**', '/*']);
                expect(controller.ignorePatterns.every(isSimple)).toBeTrue();
            });

            it('when negated', function() {
                mockIgnores(['!/Backups']);
                expect(controller.ignorePatterns.every(isSimple)).toBeTrue();
            });

            it('with escaped special characters', function() {
                mockIgnores(['/IMPORTANT\\*\\*FILES', '/mustache pics :\\{', '/Square\\[shaped\\]Images']);
                expect(controller.ignorePatterns.every(isSimple)).toBeTrue();
            });
        });

        describe('identifies advanced patterns', function() {
            function isAdvanced(p) { return !p.isSimple; };

            it('missing root path', function() {
                mockIgnores(['Backups']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });

            it('with subdirectory', function() {
                mockIgnores(['Backups/June', '!Photos/Raw']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });

            // This pattern looks simple, but differs in behavior:
            // /backups ignores the directory
            // /backups/ or /backups/** includes the directory but ignores the contents
            // The latter is tricky for the browser, so it is "advanced" for now.
            it('with trailing slash', function() {
                mockIgnores(['/Backups/', '/Backups/**']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });

            it('by empty line', function() {
                mockIgnores(['', '(?i)']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });

            it('by (?d)(?i) prefixes', function() {
                mockIgnores(['(?i)/Backups', '(?d)/Backups']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });

            it('by * wildcard', function() {
                mockIgnores(['/Back*ups', '/Backups*', '/Photos/**/Raw', '**/Photos']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });

            it('by // comment', function() {
                mockIgnores(['// Backups are too big']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });

            it('by ? character', function() {
                mockIgnores(['/Backup?/June']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });

            it('by [] character range', function() {
                mockIgnores(['/Backups/June200[0-9]']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });

            it('by {} alternatives', function() {
                mockIgnores(['/Backups/{June,July}2009']);
                expect(controller.ignorePatterns.every(isAdvanced)).toBeTrue();
            });
        });

        describe('path', function() {
            it('returns root path', function() {
                mockIgnores(['*', '**', '/', '/*']);
                expect(controller.ignorePatterns[0].path).toEqual('/');
                expect(controller.ignorePatterns[1].path).toEqual('/');
                expect(controller.ignorePatterns[2].path).toEqual('/');
                expect(controller.ignorePatterns[3].path).toEqual('/');
            });

            it('returns simple file path', function() {
                mockIgnores(['/Backups', '/Backups/', ' /Photos/Raw']);
                expect(controller.ignorePatterns[0].path).toEqual('/Backups');
                expect(controller.ignorePatterns[1].path).toEqual('/Backups/');
                expect(controller.ignorePatterns[2].path).toEqual('/Photos/Raw');
            });

            it('strips prefixes', function() {
                mockIgnores(['!/Backups', '(?i)!/!Photos/Raw']);
                expect(controller.ignorePatterns[0].path).toEqual('/Backups');
                expect(controller.ignorePatterns[1].path).toEqual('/!Photos/Raw');
            });

            it('strips trailing wildcard after path separator', function() {
                mockIgnores(['/Backups*', '!/Photos/Raw/**']);
                expect(controller.ignorePatterns[0].path).toEqual('/Backups*');
                expect(controller.ignorePatterns[1].path).toEqual('/Photos/Raw/');
            });

            it('does not strip leading wildcard', function() {
                mockIgnores(['**Backups', '**/Photos']);
                expect(controller.ignorePatterns[0].path).toEqual('/**Backups');
                expect(controller.ignorePatterns[1].path).toEqual('/**/Photos');
            });

            it('does not add leading slash to bare name', function() {
                mockIgnores(['Backups/', ' Photos/Raw']);
                expect(controller.ignorePatterns[0].path).toEqual('Backups/');
                expect(controller.ignorePatterns[1].path).toEqual('Photos/Raw');
            });
        });
    });

    describe('browse', function() {
        beforeEach(function() {
            getBrowseHandler.respond({
                'homework.txt': ['2015-04-20T22:20:45+09:00', 130940928],
                Photos: {},
            });
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
            $httpBackend.flush();
        });

        describe('navigate', function() {
            it('fetches the given folder', function() {
                getBrowseHandler.respond({ factory: {} });
                $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0');
                controller.navigate('chocolate');
                $httpBackend.flush();
                expect(controller.browse.list[0].name).toEqual('factory');
            });

            it('fetches the given prefix', function() {
                $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0&prefix=factory%2Fsecrets');
                controller.navigate('chocolate', 'factory/secrets');
                $httpBackend.flush();
            });

            it('strips trailing slashes from prefix', function() {
                $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0&prefix=factory');
                controller.navigate('chocolate', 'factory/');
                $httpBackend.flush();
            });
        });

        describe('list', function() {
            it('returns an item for each file or directory', function() {
                expect(controller.browse.list.length).toEqual(2);
            });

            it('identifies files', function() {
                expect(controller.browse.list[0].isFile).toBeTrue();
            });

            it('identifies directories', function() {
                expect(controller.browse.list[1].isFile).toBeFalse();
            });

            it('populates name', function() {
                expect(controller.browse.list[0].name).toEqual('homework.txt');
                expect(controller.browse.list[1].name).toEqual('Photos');
            });

            it('populates file size and time', function() {
                expect(controller.browse.list[0].size).toEqual(130940928);
                expect(controller.browse.list[0].modifiedAt.format('YYYY MMMM D')).toEqual('2015 April 20');
            });

            it('populates file path', function() {
                expect(controller.browse.list[0].path).toEqual('homework.txt');
            });

            it('populates directory path', function() {
                expect(controller.browse.list[1].path).toEqual('Photos');
            });

            it('populates path with parent directory', function() {
                getBrowseHandler.respond({
                    'image.jpg': ['2020-03-15T08:40:45+09:00', 82904],
                    Raw: {},
                });
                controller.navigate('default', 'Photos');
                $httpBackend.flush();
                expect(controller.browse.list[0].path).toEqual('Photos/image.jpg');
                expect(controller.browse.list[1].path).toEqual('Photos/Raw');
            });

            it('does not duplicate slash in prefix', function() {
                getBrowseHandler.respond({
                    'image.jpg': ['2020-03-15T08:40:45+09:00', 82904],
                    Raw: {},
                });
                controller.navigate('default', 'Photos/');
                $httpBackend.flush();
                expect(controller.browse.list[0].path).toEqual('Photos/image.jpg');
                expect(controller.browse.list[1].path).toEqual('Photos/Raw');
            });
        });

        describe('pathParts', function() {
            it('represents root of folder with empty prefix', function() {
                controller.navigate('default');
                $httpBackend.flush();
                expect(controller.browse.pathParts).toEqual([
                    { name: 'default', prefix: '' },
                ]);
            });

            it('includes parent directories', function() {
                controller.navigate('default', 'Photos/Raw');
                $httpBackend.flush();
                expect(controller.browse.pathParts).toEqual([
                    { name: 'default', prefix: '' },
                    { name: 'Photos', prefix: 'Photos' },
                    { name: 'Raw', prefix: 'Photos/Raw' },
                ]);
            });

            it('does not include trailing slash in prefix', function() {
                controller.navigate('default', 'Photos/Raw/');
                $httpBackend.flush();
                expect(controller.browse.pathParts).toEqual([
                    { name: 'default', prefix: '' },
                    { name: 'Photos', prefix: 'Photos' },
                    { name: 'Raw', prefix: 'Photos/Raw' },
                ]);
            });
        });
    });

    describe('matchingPatterns', function() {
        beforeEach(function() {
            getBrowseHandler.respond({
                'Backups.zip': [],
                Backups: {},
                Documents: {},
                'Photostudio.exe': [],
                Photos: {},
            });
        });

        function findFile(name) {
            var file = controller.browse.list.find(function(f) { return f.name === name; })
            if (!file) {
                throw 'No file with name "' + name + '" in scope';
            }
            return file;
        }

        it('applies the first matching rule', function() {
            getIgnoresHandler.respond({ ignore: [
                '/Backups',
                '/Photos',
                '!/Photos',
                '*',
            ] });
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
            $httpBackend.flush();
            var matches = controller.matchingPatterns(findFile('Photos'));
            expect(matches.length).toEqual(3);
            expect(matches[0].text).toEqual('/Photos');
        });

        it('does not match advanced patterns', function() {
            getIgnoresHandler.respond({ ignore: [
                '/Backup?',
                '/Backup*',
                'Backups',
            ] });
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
            $httpBackend.flush();
            var matches = controller.matchingPatterns(findFile('Backups'));
            expect(matches.length).toEqual(0);
        });

        describe('when directories are ignored', function() {
            beforeEach(function() {
                getIgnoresHandler.respond({ ignore: [
                    '/Backups.zip/',
                    '/Backups',
                    '/Documents/',
                    '!/Photos/Raw',
                    '/Photos',
                    '/Photostudio.exe',
                ] });
                controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
                $httpBackend.flush();
            });

            it('matches files', function() {
                var matches = controller.matchingPatterns(findFile('Photostudio.exe'));
                expect(matches.length).toEqual(1);
                expect(matches[0].text).toEqual('/Photostudio.exe');
            });

            it('does not match files to pattern with trailing slash', function() {
                var matches = controller.matchingPatterns(findFile('Backups.zip'));
                expect(matches.length).toEqual(0);
            });

            it('matches directories', function() {
                var matches = controller.matchingPatterns(findFile('Backups'));
                expect(matches.length).toEqual(1);
                expect(matches[0].text).toEqual('/Backups');
            });

            it('does not match directories to pattern with trailing slash', function() {
                var matches = controller.matchingPatterns(findFile('Documents'));
                expect(matches.length).toEqual(0);
            });

            it('matches directory with more specific negated pattern', function() {
                var matches = controller.matchingPatterns(findFile('Photos'));
                expect(matches.length).toEqual(1);
                expect(matches[0].text).toEqual('/Photos');
                expect(matches[0].isNegated).toBeFalse();
            });

            describe('in subdirectory', function() {
                beforeEach(function() {
                    // Same ignore patterns, but viewing the "Backups" directory
                    getBrowseHandler.respond({
                        June2008: {},
                    });
                    controller.navigate('default', 'Backups');
                    $httpBackend.flush();
                });

                it('matches by parent directory', function() {
                    var matches = controller.matchingPatterns(findFile('June2008'));
                    expect(matches.length).toEqual(1);
                    expect(matches[0].text).toEqual('/Backups');
                });
            });

            describe('in ignored subdirectory', function() {
                beforeEach(function() {
                    getBrowseHandler.respond({
                        'Cat.jpg': [],
                        'Rawr.jpg': [],
                        Raw: {},
                    });
                    controller.navigate('default', 'Photos');
                    $httpBackend.flush();
                });

                it('matches ignored files', function() {
                    var matches = controller.matchingPatterns(findFile('Cat.jpg'));
                    expect(matches.length).toEqual(1);
                    expect(matches[0].text).toEqual('/Photos');
                });

                it('does not negate files with common prefix', function() {
                    var matches = controller.matchingPatterns(findFile('Rawr.jpg'));
                    expect(matches.length).toEqual(1);
                    expect(matches[0].text).toEqual('/Photos');
                });

                it('matches negated subdirectory with more specific pattern', function() {
                    var matches = controller.matchingPatterns(findFile('Raw'));
                    expect(matches.length).toEqual(2);
                    expect(matches[0].text).toEqual('!/Photos/Raw');
                });
            });
        });

        describe('with root ignored', function() {
            beforeEach(function() {
                getIgnoresHandler.respond({ ignore: [
                    '!/Photos',
                    '*',
                ] });
                controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
                $httpBackend.flush();
            });

            it('matches root files and directories', function() {
                ['Backups.zip', 'Backups'].forEach(function(file) {
                    var matches = controller.matchingPatterns(findFile(file));
                    expect(matches.length).toEqual(1);
                });
            });

            it('matches negated directories', function() {
                var matches = controller.matchingPatterns(findFile('Photos'));
                expect(matches.length).toEqual(2);
                expect(matches[0].isNegated).toBeTrue();
            });
        });
    });
});
