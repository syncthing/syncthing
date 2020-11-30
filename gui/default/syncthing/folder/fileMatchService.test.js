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
            var fileMatches = service.update('default', files, patterns);
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
            files = BrowseService.forFolder('default').files;
            patterns = IgnoresService.forFolder('default').patterns;
        });

        it('returns an item for each file', function() {
            expect(service.update('default', files, patterns).length).toEqual(5);
        });

        it('updates array with new contents', function() {
            var folder = service.forFolder('default');
            expect(folder.length).toEqual(0);
            service.update('default', files, patterns);
            expect(folder.length).toEqual(5);
        });

        it('references ignore pattern', function() {
            expect(matchFile('Backups').matches[0]).toBe(patterns[0]);
        });

        describe('matching', function() {
            it('applies the first matching rule', function() {
                var match = matchFile('Photos');
                expect(match.matches.length).toEqual(3);
                expect(match.matches[0].text).toEqual('/Photos');
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
                    patterns = IgnoresService.forFolder('default').patterns;
                });

                it('does not match', function() {
                    var match = matchFile('Backups');
                    expect(match.matches.length).toEqual(0);
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
                    patterns = IgnoresService.forFolder('default').patterns;
                });

                it('matches files', function() {
                    var match = matchFile('Photostudio.exe');
                    expect(match.matches.length).toEqual(1);
                    expect(match.matches[0].text).toEqual('/Photostudio.exe');
                });

                it('does not match files to pattern with trailing slash', function() {
                    var match = matchFile('Backups.zip');
                    expect(match.matches.length).toEqual(0);
                });

                it('matches directories', function() {
                    var match = matchFile('Backups');
                    expect(match.matches.length).toEqual(1);
                    expect(match.matches[0].text).toEqual('/Backups');
                });

                it('does not match directories to pattern with trailing slash', function() {
                    var match = matchFile('Documents');
                    expect(match.matches.length).toEqual(0);
                });

                it('matches directory with more specific negated pattern', function() {
                    var match = matchFile('Photos');
                    expect(match.matches.length).toEqual(1);
                    expect(match.matches[0].text).toEqual('/Photos');
                    expect(match.matches[0].isNegated).toBeFalse();
                });

                describe('in subdirectory', function() {
                    beforeEach(function() {
                        // Same ignore patterns, but viewing the "Backups" directory
                        getBrowseHandler.respond({
                            June2008: {},
                        });
                        BrowseService.refresh('default', 'Backups');
                        $httpBackend.flush();
                        files = BrowseService.forFolder('default').files;
                    });

                    it('matches by parent directory', function() {
                        var match = matchFile('June2008');
                        expect(match.matches.length).toEqual(1);
                        expect(match.matches[0].text).toEqual('/Backups');
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
                        files = BrowseService.forFolder('default').files;
                    });

                    it('matches ignored files', function() {
                        var match = matchFile('Cat.jpg');
                        expect(match.matches.length).toEqual(1);
                        expect(match.matches[0].text).toEqual('/Photos');
                    });

                    it('does not negate files with common prefix', function() {
                        var match = matchFile('Rawr.jpg');
                        expect(match.matches.length).toEqual(1);
                        expect(match.matches[0].text).toEqual('/Photos');
                    });

                    it('matches negated subdirectory with more specific pattern', function() {
                        var match = matchFile('Raw');
                        expect(match.matches.length).toEqual(2);
                        expect(match.matches[0].text).toEqual('!/Photos/Raw');
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
                    patterns = IgnoresService.forFolder('default').patterns;
                });

                it('matches root files and directories', function() {
                    ['Backups.zip', 'Backups'].forEach(function(file) {
                        var match = matchFile(file);
                        expect(match.matches.length).toEqual(1);
                    });
                });

                it('matches negated directories', function() {
                    var match = matchFile('Photos');
                    expect(match.matches.length).toEqual(2);
                    expect(match.matches[0].isNegated).toBeTrue();
                });
            });
        });
    });
});
