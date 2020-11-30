describe('IgnoresService', function() {
    beforeEach(module('syncthing.folder'));

    var $httpBackend, service;

    beforeEach(inject(function($injector) {
        $httpBackend = $injector.get('$httpBackend');
        service = $injector.get('Ignores');
    }));

    afterEach(function() {
        // Ensure requests are flushed and assertions met
        $httpBackend.verifyNoOutstandingExpectation();
        $httpBackend.verifyNoOutstandingRequest();
    });

    describe('forFolder', function() {
        it('returns the same object for the same folder', function () {
            var folderA = service.forFolder('default');
            var folderB = service.forFolder('default');
            expect(folderA).toBe(folderB);
        });
    });

    describe('tempFolder', function() {
        it('returns an object shaped like ignore state', function () {
            var temp = service.tempFolder();
            expect(temp.text).toEqual('');
            expect(temp.patterns).toEqual([]);
            expect(temp.disabled).toBeFalse();
        });

        it('does not reference another folder', function () {
            var tempA = service.tempFolder();
            var tempB = service.tempFolder();
            tempA.text = '/Backups';
            expect(tempB.text).toEqual('');
        });
    });

    describe('addPattern', function() {
        beforeEach(function () {
            service.addPattern('default', '*');
            service.addPattern('default', '/Backups');
        });

        it('inserts pattern at array start', function() {
            service.addPattern('default', '!/Photos');
            expect(service.forFolder('default').patterns[0].text).toEqual('!/Photos');
        });

        it('updates folder text', function() {
            expect(service.forFolder('default').text).toEqual('/Backups\n*');
            service.addPattern('default', '!/Photos');
            expect(service.forFolder('default').text).toEqual('!/Photos\n/Backups\n*');
        });
    });

    describe('removePattern', function() {
        beforeEach(function () {
            service.addPattern('default', '*');
            service.addPattern('default', '/Backups');
        });

        it('removes pattern from array', function() {
            service.removePattern('default', '/Backups');
            expect(service.forFolder('default').patterns.map(function (p) { return p.text; })).not.toContain('/Backups');
        });

        it('does nothing when pattern is absent', function() {
            service.removePattern('default', 'oh no');
            expect(service.forFolder('default').patterns.length).toEqual(2);
        });

        it('updates folder text', function() {
            service.removePattern('default', '*');
            expect(service.forFolder('default').text).toEqual('/Backups');
        });
    });

    describe('save', function() {
        it('submits to the folder', function () {
            $httpBackend.expectPOST(
                'rest/db/ignores?folder=default',
                { ignore: '!/Backups\n*' },
            ).respond(200);
            service.save('default', '!/Backups\n*');
            $httpBackend.flush();
        });
    });

    describe('refresh', function() {
        var getIgnoresHandler;

        beforeEach(function () {
            getIgnoresHandler = $httpBackend.when('GET', /^rest\/db\/ignores/);
        });

        it('fetches the folder', function() {
            $httpBackend.expectGET('rest/db/ignores?folder=default').respond({ ignore: [] });
            service.refresh('default');
            $httpBackend.flush();
        });

        it('populates the folder data', function() {
            var folder = service.forFolder('default');
            getIgnoresHandler.respond({ ignore: ['/some-directory'] });
            service.refresh('default');
            $httpBackend.flush();
            expect(folder.text).toEqual('/some-directory');
        });

        it('handles null ignores', function() {
            getIgnoresHandler.respond({ ignore: null });
            service.refresh('default');
            $httpBackend.flush();
            expect(service.forFolder('default').text).toEqual('');
            expect(service.forFolder('default').patterns).toEqual([]);
        });

        it('sets folder state while loading', function() {
            var folder = service.forFolder('default');
            getIgnoresHandler.respond({ ignore: ['/some-directory'] });
            service.refresh('default');
            expect(folder.disabled).toBeTrue();
            expect(folder.text).toEqual('Loading...');
            $httpBackend.flush();
            expect(folder.disabled).toBeFalse();
            expect(folder.text).toEqual('/some-directory');
        });

        describe('patterns', function() {
            function mockIgnores(ignores) {
                getIgnoresHandler.respond({ ignore: ignores });
                // Initialize the controller, triggering http gets and populating data
                service.refresh('default');
                $httpBackend.flush();
                return service.forFolder('default').patterns;
            }

            it('returns a pattern for each line', function() {
                var patterns = mockIgnores([
                    '/Backups',
                    'Photos/**/Raw'
                ]);
                expect(patterns.length).toEqual(2);
            });

            it('identifies negated patterns', function() {
                var patterns = mockIgnores(['!/Backups', '(?i)!/Photos/Raw', '/IMPORTANT FILES!', '(?i)/!Photos/Raw']);
                expect(patterns[0].isNegated).toBeTrue();
                expect(patterns[1].isNegated).toBeTrue();
                expect(patterns[2].isNegated).toBeFalse();
                expect(patterns[3].isNegated).toBeFalse();
            });

            describe('identifies simple patterns', function() {
                function isSimple(p) { return p.isSimple; };

                it('by root folder', function() {
                    var patterns = mockIgnores(['/Backups', ' /Photos/Raw', '/']);
                    expect(patterns.every(isSimple)).toBeTrue();
                });

                it('by root wildcard', function() {
                    var patterns = mockIgnores(['*', '**', '/*']);
                    expect(patterns.every(isSimple)).toBeTrue();
                });

                it('when negated', function() {
                    var patterns = mockIgnores(['!/Backups']);
                    expect(patterns.every(isSimple)).toBeTrue();
                });

                it('with escaped special characters', function() {
                    var patterns = mockIgnores(['/IMPORTANT\\*\\*FILES', '/mustache pics :\\{', '/Square\\[shaped\\]Images']);
                    expect(patterns.every(isSimple)).toBeTrue();
                });
            });

            describe('identifies advanced patterns', function() {
                function isAdvanced(p) { return !p.isSimple; };

                it('missing root path', function() {
                    var patterns = mockIgnores(['Backups']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                it('with subdirectory', function() {
                    var patterns = mockIgnores(['Backups/June', '!Photos/Raw']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                // This pattern looks simple, but differs in behavior:
                // /backups ignores the directory
                // /backups/ or /backups/** includes the directory but ignores the contents
                // The latter is tricky for the browser, so it is "advanced" for now.
                it('with trailing slash', function() {
                    var patterns = mockIgnores(['/Backups/', '/Backups/**']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                it('by empty line', function() {
                    var patterns = mockIgnores(['', '(?i)']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                it('by (?d)(?i) prefixes', function() {
                    var patterns = mockIgnores(['(?i)/Backups', '(?d)/Backups']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                it('by * wildcard', function() {
                    var patterns = mockIgnores(['/Back*ups', '/Backups*', '/Photos/**/Raw', '**/Photos']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                it('by // comment', function() {
                    var patterns = mockIgnores(['// Backups are too big']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                it('by ? character', function() {
                    var patterns = mockIgnores(['/Backup?/June']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                it('by [] character range', function() {
                    var patterns = mockIgnores(['/Backups/June200[0-9]']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                it('by {} alternatives', function() {
                    var patterns = mockIgnores(['/Backups/{June,July}2009']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });
            });

            describe('path', function() {
                it('returns root path', function() {
                    var patterns = mockIgnores(['*', '**', '/', '/*']);
                    expect(patterns[0].path).toEqual('/');
                    expect(patterns[1].path).toEqual('/');
                    expect(patterns[2].path).toEqual('/');
                    expect(patterns[3].path).toEqual('/');
                });

                it('returns simple file path', function() {
                    var patterns = mockIgnores(['/Backups', '/Backups/', ' /Photos/Raw']);
                    expect(patterns[0].path).toEqual('/Backups');
                    expect(patterns[1].path).toEqual('/Backups/');
                    expect(patterns[2].path).toEqual('/Photos/Raw');
                });

                it('strips prefixes', function() {
                    var patterns = mockIgnores(['!/Backups', '(?i)!/!Photos/Raw']);
                    expect(patterns[0].path).toEqual('/Backups');
                    expect(patterns[1].path).toEqual('/!Photos/Raw');
                });

                it('strips trailing wildcard after path separator', function() {
                    var patterns = mockIgnores(['/Backups*', '!/Photos/Raw/**']);
                    expect(patterns[0].path).toEqual('/Backups*');
                    expect(patterns[1].path).toEqual('/Photos/Raw/');
                });

                it('does not strip leading wildcard', function() {
                    var patterns = mockIgnores(['**Backups', '**/Photos']);
                    expect(patterns[0].path).toEqual('/**Backups');
                    expect(patterns[1].path).toEqual('/**/Photos');
                });

                it('does not add leading slash to bare name', function() {
                    var patterns = mockIgnores(['Backups/', ' Photos/Raw']);
                    expect(patterns[0].path).toEqual('Backups/');
                    expect(patterns[1].path).toEqual('Photos/Raw');
                });
            });
        });
    });
});
