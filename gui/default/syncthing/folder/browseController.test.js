describe('BrowseController', function() {
    // Set up the module
    beforeEach(module('syncthing.folder'));

    var $controller, $scope, $httpBackend, getIgnoresHandler, getBrowseHandler;
    var controller, browseService, ignoresService;

    // Inject angular bits
    beforeEach(inject(function($injector) {
        $httpBackend = $injector.get('$httpBackend');
        // Common controller init requests
        // Tests override these responses where necessary
        getIgnoresHandler = $httpBackend.when('GET', /^rest\/db\/ignores/).respond({ ignore: [] });
        getBrowseHandler = $httpBackend.when('GET', /^rest\/db\/browse/).respond({});

        browseService = $injector.get('Browse');
        ignoresService = $injector.get('Ignores');
        $controller = $injector.get('$controller');
        $scope = $injector.get('$rootScope');
    }));

    afterEach(function() {
        // Ensure requests are flushed and assertions met
        $httpBackend.verifyNoOutstandingExpectation();
        $httpBackend.verifyNoOutstandingRequest();
    });

    describe('CurrentFolder watch', function() {
        it('does not set Browse reference when current folder is undefined', function() {
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: {} });
            $scope.$apply();
            expect(controller.browse).toBeUndefined();
        });

        it('updates browse reference when initialized', function() {
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
            $scope.$apply();
            expect(controller.browse).toBe(browseService.forFolder('default'));
        });

        it('updates browse reference when current folder changes', function() {
            var folder = { id: 'default' };
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: folder });
            $scope.$apply();
            folder.id = 'documents';
            $scope.$apply();
            expect(controller.browse).toBe(browseService.forFolder('documents'));
        });
    });

    describe('navigate', function() {
        it('fetches the given folder and prefix', function() {
            $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0&prefix=factory%2Fsecrets');
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
            controller.navigate('chocolate', 'factory/secrets');
            $httpBackend.flush();
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
            browseService.refresh('default');
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
            ignoresService.refresh('default');
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
            ignoresService.refresh('default');
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
                ignoresService.refresh('default');
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
                ignoresService.refresh('default');
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
