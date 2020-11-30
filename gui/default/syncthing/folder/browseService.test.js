describe('BrowseService', function() {
    beforeEach(module('syncthing.folder'));

    var $httpBackend, getBrowseHandler;
    var service;

    beforeEach(inject(function($injector) {
        $httpBackend = $injector.get('$httpBackend');
        getBrowseHandler = $httpBackend.when('GET', /^rest\/db\/browse/).respond({});

        service = $injector.get('Browse');
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

    describe('refresh', function() {
        it('fetches data for the folder', function() {
            $httpBackend.expectGET('rest/db/browse?folder=default&levels=0').respond({ Backups: {} });
            service.refresh('default');
            $httpBackend.flush();
            expect(service.forFolder('default').files.length).toEqual(1);
        });

        it('fetches the given prefix', function() {
            $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0&prefix=factory%2Fsecrets');
            service.refresh('chocolate', 'factory/secrets');
            $httpBackend.flush();
        });

        it('strips trailing slashes from prefix', function() {
            $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0&prefix=factory');
            service.refresh('chocolate', 'factory/');
            $httpBackend.flush();
        });

        describe('browse', function() {
            beforeEach(function() {
                getBrowseHandler.respond({
                    'homework.txt': ['2015-04-20T22:20:45+09:00', 130940928],
                    Photos: {},
                });
                service.refresh('default');
                $httpBackend.flush();
            });

            describe('refresh', function() {
                it('fetches the given folder', function() {
                    getBrowseHandler.respond({ factory: {} });
                    $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0');
                    service.refresh('chocolate');
                    $httpBackend.flush();
                    expect(service.browse['chocolate'].files[0].name).toEqual('factory');
                });

                it('fetches the given prefix', function() {
                    $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0&prefix=factory%2Fsecrets');
                    service.refresh('chocolate', 'factory/secrets');
                    $httpBackend.flush();
                });

                it('strips trailing slashes from prefix', function() {
                    $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0&prefix=factory');
                    service.refresh('chocolate', 'factory/');
                    $httpBackend.flush();
                });
            });

            describe('files', function() {
                it('returns an item for each file or directory', function() {
                    expect(service.browse['default'].files.length).toEqual(2);
                });

                it('identifies files', function() {
                    expect(service.browse['default'].files[0].isFile).toBeTrue();
                });

                it('identifies directories', function() {
                    expect(service.browse['default'].files[1].isFile).toBeFalse();
                });

                it('populates name', function() {
                    expect(service.browse['default'].files[0].name).toEqual('homework.txt');
                    expect(service.browse['default'].files[1].name).toEqual('Photos');
                });

                it('populates file size and time', function() {
                    expect(service.browse['default'].files[0].size).toEqual(130940928);
                    expect(service.browse['default'].files[0].modifiedAt.format('YYYY MMMM D')).toEqual('2015 April 20');
                });

                it('populates file path', function() {
                    expect(service.browse['default'].files[0].path).toEqual('homework.txt');
                });

                it('populates directory path', function() {
                    expect(service.browse['default'].files[1].path).toEqual('Photos');
                });

                it('populates path with parent directory', function() {
                    getBrowseHandler.respond({
                        'image.jpg': ['2020-03-15T08:40:45+09:00', 82904],
                        Raw: {},
                    });
                    service.refresh('default', 'Photos');
                    $httpBackend.flush();
                    expect(service.browse['default'].files[0].path).toEqual('Photos/image.jpg');
                    expect(service.browse['default'].files[1].path).toEqual('Photos/Raw');
                });

                it('does not duplicate slash in prefix', function() {
                    getBrowseHandler.respond({
                        'image.jpg': ['2020-03-15T08:40:45+09:00', 82904],
                        Raw: {},
                    });
                    service.refresh('default', 'Photos/');
                    $httpBackend.flush();
                    expect(service.browse['default'].files[0].path).toEqual('Photos/image.jpg');
                    expect(service.browse['default'].files[1].path).toEqual('Photos/Raw');
                });
            });

            describe('pathParts', function() {
                it('represents root of folder with empty prefix', function() {
                    service.refresh('default');
                    $httpBackend.flush();
                    expect(service.browse['default'].pathParts).toEqual([
                        { name: 'default', prefix: '' },
                    ]);
                });

                it('includes parent directories', function() {
                    service.refresh('default', 'Photos/Raw');
                    $httpBackend.flush();
                    expect(service.browse['default'].pathParts).toEqual([
                        { name: 'default', prefix: '' },
                        { name: 'Photos', prefix: 'Photos' },
                        { name: 'Raw', prefix: 'Photos/Raw' },
                    ]);
                });

                it('does not include trailing slash in prefix', function() {
                    service.refresh('default', 'Photos/Raw/');
                    $httpBackend.flush();
                    expect(service.browse['default'].pathParts).toEqual([
                        { name: 'default', prefix: '' },
                        { name: 'Photos', prefix: 'Photos' },
                        { name: 'Raw', prefix: 'Photos/Raw' },
                    ]);
                });
            });
        });
    });
});
