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
            service.addPattern('/Backups');
        });

        it('inserts pattern at array start', function() {
            service.addPattern('!/Photos');
            expect(service.data.patterns[0].text).toEqual('!/Photos');
        });

        it('collapses more specific patterns', function() {
            service.addPattern('/Photos/Raw');
            service.addPattern('!/Photos/Raw/Landscapes');
            service.addPattern('/Photos');
            expect(service.data.patterns.map(p => p.text)).toEqual(['/Photos', '/Backups']);
        });

        it('inserts before bare name matching prefix', function() {
            service.addPattern('/Photoshop');
            service.addPattern('!/Photos');
            expect(service.data.patterns.map(p => p.text)).toEqual(['!/Photos', '/Photoshop', '/Backups']);
        });

        it('ignores advanced patterns when inserting', function() {
            service.addPattern('/Photos/**/folder.jpg');
            service.addPattern('/Photos');
            expect(service.data.patterns[0].text).toEqual('/Photos');
        });

        it('updates folder text', function() {
            service.addPattern('!/Photos');
            expect(service.data.text).toEqual('!/Photos\n/Backups');
        });
    });

    describe('removePattern', function() {
        beforeEach(function () {
            service.addPattern('*');
            service.addPattern('/Backups');
        });

        it('removes pattern from array', function() {
            service.removePattern('/Backups');
            expect(service.data.patterns.map(p => p.text)).toEqual(['*']);
        });

        it('does nothing when pattern is absent', function() {
            service.removePattern('oh no');
            expect(service.data.patterns.length).toEqual(2);
        });

        it('collapses more specific patterns', function() {
            service.addPattern('!/Backups/June');
            service.addPattern('/Backups/June/Images');
            service.removePattern('!/Backups/June');
            expect(service.data.patterns.map(p => p.text)).toEqual(['/Backups', '*']);
        });

        it('updates folder text', function() {
            service.removePattern('/Backups');
            expect(service.data.text).toEqual('*');
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

    describe('parseText', function() {
        beforeEach(function () {
            service.addPattern('*');
            service.addPattern('/Backups');
        });

        it('updates patterns from text', function() {
            service.parseText('/Photos\n/Backups');
            expect(service.data.patterns.map(function(p) { return p.text; })).toEqual(['/Photos', '/Backups']);
        });

        it('accepts empty line', function() {
            service.parseText('/Photos\n\n/Backups');
            expect(service.data.patterns.map(function(p) { return p.text; })).toEqual(['/Photos', '', '/Backups']);
        });

        it('accepts empty text', function() {
            service.parseText('');
            expect(service.data.patterns).toEqual([]);
        });
    });

    describe('refresh', function() {
        var getIgnoresHandler;

        beforeEach(function () {
            getIgnoresHandler = $httpBackend.when('GET', /^rest\/db\/ignores/);
        });

        it('sets error and text when ignore fetch fails', function () {
            $httpBackend.expectGET('rest/db/ignores?folder=default').respond(500);
            service.refresh('default').catch(() => {});
            $httpBackend.flush();
            expect(service.data.text).toEqual('');
            expect(service.data.error.status).toEqual(500);
        });

        it('fetches the folder', function() {
            $httpBackend.expectGET('rest/db/ignores?folder=default').respond({ ignore: [] });
            service.refresh('default');
            $httpBackend.flush();
        });

        it('populates the folder data', function() {
            var folder = service.data;
            getIgnoresHandler.respond({ ignore: ['/some-directory'] });
            service.refresh('default');
            $httpBackend.flush();
            expect(folder.text).toEqual('/some-directory');
        });

        it('handles null ignores', function() {
            getIgnoresHandler.respond({ ignore: null });
            service.refresh('default');
            $httpBackend.flush();
            expect(service.data.text).toEqual('');
            expect(service.data.patterns).toEqual([]);
        });

        it('sets folder state while loading', function() {
            var folder = service.data;
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
                return service.data.patterns;
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

                it('with whitespace', function() {
                    var patterns = mockIgnores([' ', '', '\n']);
                    expect(patterns.every(isSimple)).toBeTrue();
                });

                it('with comment', function() {
                    var patterns = mockIgnores(['// Backups are too big']);
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

                it('by (?d)(?i) prefixes', function() {
                    var patterns = mockIgnores(['(?i)/Backups', '(?d)/Backups', '(?i)']);
                    expect(patterns.every(isAdvanced)).toBeTrue();
                });

                it('by * wildcard', function() {
                    var patterns = mockIgnores(['/Back*ups', '/Backups*', '/Photos/**/Raw', '**/Photos']);
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

    describe('matchingPattern', function() {
        function matchFile(path) {
            return service.matchingPattern({ path: path });
        }

        beforeEach(function() {
            /* A directory like:
             * Backups.zip
             * Backups/
             * Documents/
             * Photostudio.exe
             * Photos/
             */
            $httpBackend.expectGET('rest/db/ignores?folder=default').respond({ ignore: [
                '/Backups',
                '/Photos',
                '!/Photos',
                '*',
            ] });
            service.refresh('default');
            $httpBackend.flush();
        });

        it('references ignore pattern', function() {
            expect(matchFile('Backups')).toBe(service.data.patterns[0]);
        });

        describe('matching', function() {
            it('applies the first matching rule', function() {
                var match = matchFile('Photos');
                expect(match).toBeDefined();
                expect(match.text).toEqual('/Photos');
            });

            describe('with advanced patterns', function() {
                beforeEach(function() {
                    $httpBackend.expectGET('rest/db/ignores?folder=default').respond({ ignore: [
                        '/Backup?',
                        '/Backup*',
                        'Backups',
                    ] });
                    service.refresh('default');
                    $httpBackend.flush();
                });

                it('does not match', function() {
                    var match = matchFile('Backups');
                    expect(match).toBeUndefined();
                });
            });

            describe('when directories are ignored', function() {
                beforeEach(function() {
                    $httpBackend.expectGET('rest/db/ignores?folder=default').respond({ ignore: [
                        '/Backups.zip/',
                        '/Backups',
                        '/Documents/',
                        '!/Photos/Raw',
                        '/Photos',
                        '/Photostudio.exe',
                    ] });
                    service.refresh('default');
                    $httpBackend.flush();
                });

                it('matches files', function() {
                    var match = matchFile('Photostudio.exe');
                    expect(match).toBeDefined();
                    expect(match.text).toEqual('/Photostudio.exe');
                });

                it('does not match files to pattern with trailing slash', function() {
                    var match = matchFile('Backups.zip');
                    expect(match).toBeUndefined();
                });

                it('matches directories', function() {
                    var match = matchFile('Backups');
                    expect(match).toBeDefined();
                    expect(match.text).toEqual('/Backups');
                });

                it('does not match directories to pattern with trailing slash', function() {
                    var match = matchFile('Documents');
                    expect(match).toBeUndefined();
                });

                it('matches directory with more specific negated pattern', function() {
                    var match = matchFile('Photos');
                    expect(match).toBeDefined();
                    expect(match.text).toEqual('/Photos');
                    expect(match.isNegated).toBeFalse();
                });

                describe('in subdirectory', function() {
                    /* A directory like:
                     * Backups/June2008/
                     */

                    it('matches by parent directory', function() {
                        var match = matchFile('Backups/June2008');
                        expect(match).toBeDefined();
                        expect(match.text).toEqual('/Backups');
                    });
                });

                describe('in ignored subdirectory', function() {
                    /* A directory like:
                     * Photos/Cat.jpg
                     * Photos/Rawr.jpg
                     * Photos/Raw/
                     */

                    it('matches ignored files', function() {
                        var match = matchFile('Photos/Cat.jpg');
                        expect(match).toBeDefined();
                        expect(match.text).toEqual('/Photos');
                    });

                    it('does not negate files with common prefix', function() {
                        var match = matchFile('Photos/Rawr.jpg');
                        expect(match).toBeDefined();
                        expect(match.text).toEqual('/Photos');
                    });

                    it('matches negated subdirectory with more specific pattern', function() {
                        var match = matchFile('Photos/Raw');
                        expect(match).toBeDefined();
                        expect(match.text).toEqual('!/Photos/Raw');
                    });
                });
            });

            describe('with root ignored', function() {
                beforeEach(function() {
                    $httpBackend.expectGET('rest/db/ignores?folder=default').respond({ ignore: [
                        '!/Photos',
                        '*',
                    ] });
                    service.refresh('default');
                    $httpBackend.flush();
                });

                it('matches root files and directories', function() {
                    ['Backups.zip', 'Backups'].forEach(function(file) {
                        var match = matchFile(file);
                        expect(match).toBeDefined();
                    });
                });

                it('matches negated directories', function() {
                    var match = matchFile('Photos');
                    expect(match).toBeDefined();
                    expect(match.isNegated).toBeTrue();
                });
            });
        });
    });
});
