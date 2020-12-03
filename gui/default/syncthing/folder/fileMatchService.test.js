describe('FileMatchesService', function() {
    beforeEach(module('syncthing.folder'));

    var $httpBackend, getBrowseHandler, getIgnoresHandler;
    var service, BrowseService, IgnoresService;

    beforeEach(inject(function($injector) {
        $httpBackend = $injector.get('$httpBackend');
        getBrowseHandler = $httpBackend.when('GET', /^rest\/db\/browse/);
        getIgnoresHandler = $httpBackend.when('GET', /^rest\/db\/ignores/);

        BrowseService = $injector.get('Browse');
        IgnoresService = $injector.get('Ignores');
        service = $injector.get('FileMatches');
    }));

    afterEach(function() {
        // Ensure requests are flushed and assertions met
        $httpBackend.verifyNoOutstandingExpectation();
        $httpBackend.verifyNoOutstandingRequest();
    });

    describe('update', function() {
        var files, patterns;

        function matchFile(name) {
            var fileMatches = service.update(files, patterns);
            var match = fileMatches.find(function(fm) { return fm.file.name === name; })
            if (!match) {
                throw 'No match with name "' + name + '"';
            }
            return match;
        }

        beforeEach(function() {
            getBrowseHandler.respond({
                'Backups.zip': [],
                Backups: {},
                Documents: {},
                'Photostudio.exe': [],
                Photos: {},
            });
            getIgnoresHandler.respond({ ignore: [
                '/Backups',
                '/Photos',
                '!/Photos',
                '*',
            ] });
            BrowseService.refresh('default');
            IgnoresService.refresh('default');
            $httpBackend.flush();
            files = BrowseService.data.files;
            patterns = IgnoresService.data.patterns;
        });

        it('returns an item for each file', function() {
            expect(service.update(files, patterns).length).toEqual(5);
        });

        it('updates array with new contents', function() {
            var folder = service.data;
            expect(folder.length).toEqual(0);
            service.update(files, patterns);
            expect(folder.length).toEqual(5);
        });

        it('references ignore pattern', function() {
            expect(matchFile('Backups').match).toBe(patterns[0]);
        });

        describe('matching', function() {
            it('applies the first matching rule', function() {
                var match = matchFile('Photos');
                expect(match.match).toBeDefined();
                expect(match.match.text).toEqual('/Photos');
            });

            describe('with advanced patterns', function() {
                beforeEach(function() {
                    getIgnoresHandler.respond({ ignore: [
                        '/Backup?',
                        '/Backup*',
                        'Backups',
                    ] });
                    IgnoresService.refresh('default');
                    $httpBackend.flush();
                    patterns = IgnoresService.data.patterns;
                });

                it('does not match', function() {
                    var match = matchFile('Backups');
                    expect(match.match).toBeUndefined();
                });
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
                    IgnoresService.refresh('default');
                    $httpBackend.flush();
                    patterns = IgnoresService.data.patterns;
                });

                it('matches files', function() {
                    var match = matchFile('Photostudio.exe');
                    expect(match.match).toBeDefined();
                    expect(match.match.text).toEqual('/Photostudio.exe');
                });

                it('does not match files to pattern with trailing slash', function() {
                    var match = matchFile('Backups.zip');
                    expect(match.match).toBeUndefined();
                });

                it('matches directories', function() {
                    var match = matchFile('Backups');
                    expect(match.match).toBeDefined();
                    expect(match.match.text).toEqual('/Backups');
                });

                it('does not match directories to pattern with trailing slash', function() {
                    var match = matchFile('Documents');
                    expect(match.match).toBeUndefined();
                });

                it('matches directory with more specific negated pattern', function() {
                    var match = matchFile('Photos');
                    expect(match.match).toBeDefined();
                    expect(match.match.text).toEqual('/Photos');
                    expect(match.match.isNegated).toBeFalse();
                });

                describe('in subdirectory', function() {
                    beforeEach(function() {
                        // Same ignore patterns, but viewing the "Backups" directory
                        getBrowseHandler.respond({
                            June2008: {},
                        });
                        BrowseService.refresh('default', 'Backups');
                        $httpBackend.flush();
                        files = BrowseService.data.files;
                    });

                    it('matches by parent directory', function() {
                        var match = matchFile('June2008');
                        expect(match.match).toBeDefined();
                        expect(match.match.text).toEqual('/Backups');
                    });
                });

                describe('in ignored subdirectory', function() {
                    beforeEach(function() {
                        getBrowseHandler.respond({
                            'Cat.jpg': [],
                            'Rawr.jpg': [],
                            Raw: {},
                        });
                        BrowseService.refresh('default', 'Photos');
                        $httpBackend.flush();
                        files = BrowseService.data.files;
                    });

                    it('matches ignored files', function() {
                        var match = matchFile('Cat.jpg');
                        expect(match.match).toBeDefined();
                        expect(match.match.text).toEqual('/Photos');
                    });

                    it('does not negate files with common prefix', function() {
                        var match = matchFile('Rawr.jpg');
                        expect(match.match).toBeDefined();
                        expect(match.match.text).toEqual('/Photos');
                    });

                    it('matches negated subdirectory with more specific pattern', function() {
                        var match = matchFile('Raw');
                        expect(match.match).toBeDefined();
                        expect(match.match.text).toEqual('!/Photos/Raw');
                    });
                });
            });

            describe('with root ignored', function() {
                beforeEach(function() {
                    getIgnoresHandler.respond({ ignore: [
                        '!/Photos',
                        '*',
                    ] });
                    IgnoresService.refresh('default');
                    $httpBackend.flush();
                    patterns = IgnoresService.data.patterns;
                });

                it('matches root files and directories', function() {
                    ['Backups.zip', 'Backups'].forEach(function(file) {
                        var match = matchFile(file);
                        expect(match.match).toBeDefined();
                    });
                });

                it('matches negated directories', function() {
                    var match = matchFile('Photos');
                    expect(match.match).toBeDefined();
                    expect(match.match.isNegated).toBeTrue();
                });
            });
        });
    });
});
