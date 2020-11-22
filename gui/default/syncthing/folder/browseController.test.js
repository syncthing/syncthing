describe('BrowseController', function() {
    // Set up the module
    beforeEach(module('syncthing.folder'));

    var $controller, $scope, $httpBackend, getBrowseHandler;
    var controller;

    // Inject angular bits
    beforeEach(inject(function($injector) {
        $httpBackend = $injector.get('$httpBackend');
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
            $httpBackend.expectGET('rest/db/browse?folder=default&levels=0');
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: { id: 'default' } });
            $httpBackend.flush();
        });

        it('fetches data when current folder changes', function() {
            var folder = { id: 'default' };
            $httpBackend.expectGET('rest/db/browse?folder=default&levels=0');
            controller = $controller('BrowseController', { $scope: $scope, CurrentFolder: folder });
            $httpBackend.flush();
            folder.id = 'documents';
            $httpBackend.expectGET('rest/db/browse?folder=documents&levels=0');
            $httpBackend.flush();
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

            it('populates directory path with trailing slash', function() {
                expect(controller.browse.list[1].path).toEqual('Photos/');
            });

            it('populates path with parent directory', function() {
                getBrowseHandler.respond({
                    'image.jpg': ['2020-03-15T08:40:45+09:00', 82904],
                    Raw: {},
                });
                controller.navigate('default', 'Photos');
                $httpBackend.flush();
                expect(controller.browse.list[0].path).toEqual('Photos/image.jpg');
                expect(controller.browse.list[1].path).toEqual('Photos/Raw/');
            });

            it('does not duplicate slash in prefix', function() {
                getBrowseHandler.respond({
                    'image.jpg': ['2020-03-15T08:40:45+09:00', 82904],
                    Raw: {},
                });
                controller.navigate('default', 'Photos/');
                $httpBackend.flush();
                expect(controller.browse.list[0].path).toEqual('Photos/image.jpg');
                expect(controller.browse.list[1].path).toEqual('Photos/Raw/');
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
});
