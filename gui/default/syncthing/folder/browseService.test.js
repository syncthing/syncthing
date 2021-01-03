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

    describe('refresh', function() {
        it('fetches data for the folder', function() {
            $httpBackend.expectGET('rest/db/browse?folder=default&levels=0').respond({ Backups: {} });
            service.refresh('default');
            $httpBackend.flush();
            expect(service.data.files.length).toEqual(1);
        });

        it('fetches the given prefix', function() {
            $httpBackend.expectGET('rest/db/browse?folder=chocolate&levels=0&prefix=factory%2Fsecrets%2F');
            service.refresh('chocolate', 'factory/secrets/');
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

            describe('files', function() {
                it('returns an item for each file or directory', function() {
                    expect(service.data.files.length).toEqual(2);
                });

                it('identifies files', function() {
                    expect(service.data.files[0].isFile).toBeTrue();
                });

                it('identifies directories', function() {
                    expect(service.data.files[1].isFile).toBeFalse();
                });

                it('populates name', function() {
                    expect(service.data.files[0].name).toEqual('homework.txt');
                    expect(service.data.files[1].name).toEqual('Photos');
                });

                it('populates file path', function() {
                    expect(service.data.files[0].path).toEqual('homework.txt');
                });

                it('populates directory path', function() {
                    expect(service.data.files[1].path).toEqual('Photos');
                });

                it('populates path with parent directory', function() {
                    getBrowseHandler.respond({
                        'image.jpg': ['2020-03-15T08:40:45+09:00', 82904],
                        Raw: {},
                    });
                    service.refresh('default', 'Photos');
                    $httpBackend.flush();
                    expect(service.data.files[0].path).toEqual('Photos/image.jpg');
                    expect(service.data.files[1].path).toEqual('Photos/Raw');
                });

                it('does not duplicate slash in prefix', function() {
                    getBrowseHandler.respond({
                        'image.jpg': ['2020-03-15T08:40:45+09:00', 82904],
                        Raw: {},
                    });
                    service.refresh('default', 'Photos/');
                    $httpBackend.flush();
                    expect(service.data.files[0].path).toEqual('Photos/image.jpg');
                    expect(service.data.files[1].path).toEqual('Photos/Raw');
                });
            });
        });
    });
});
