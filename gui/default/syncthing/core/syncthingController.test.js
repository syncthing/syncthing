describe('SyncthingController', function() {
    beforeEach(module('syncthing.core'));

    var $controller, $scope, IgnoresService, IgnoreTreeService;
    var controller;

    beforeEach(inject(function($injector) {
        $controller = $injector.get('$controller');
        IgnoresService = $injector.get('Ignores');
        IgnoreTreeService = $injector.get('IgnoreTree');
        $scope = $injector.get('$rootScope');
        $scope.system = { pathSeparator: '/' };
    }));

    describe('editFolder', function() {
        var ignoreTreeSpy, ignoresSpy;

        beforeEach(function () {
            controller = $controller('SyncthingController', { $scope: $scope });
            // Stub editFolderModal for its form functions
            spyOn($scope, 'editFolderModal');
            ignoreTreeSpy = spyOn(IgnoreTreeService, 'refresh');
            ignoresSpy = spyOn(IgnoresService, 'refresh').and
                .returnValue(Promise.resolve({ patterns: [
                    IgnoresService.addPattern('/Backups'),
                    IgnoresService.addPattern('*'),
                ] }));
        });

        it('binds to IgnoresService', function () {
            $scope.editFolder({ id: 'default', path: '/var/sync', devices: [] });
            expect($scope.ignores.text).toBe(IgnoresService.data.text);
        });

        it('fetches folder ignore patterns', function () {
            $scope.editFolder({ id: 'default', path: '/var/sync', devices: [] });
            expect(ignoresSpy).toHaveBeenCalledWith('default');
        });

        it('enables basic ignore UI', async function () {
            $scope.editFolder({ id: 'default', path: '/var/sync', devices: [] });
            await Promise.resolve();
            expect($scope.currentFolder.ignoreIsBasic).toBeTrue();
            expect($scope.currentFolder.ignoreIsEditingAdvanced).toBeFalse();
        });

        it('reloads basic UI', async function () {
            $scope.editFolder({ id: 'default', path: '/var/sync', devices: [] });
            await Promise.resolve();
            expect(ignoreTreeSpy).toHaveBeenCalledWith('default');
        });

        it('sets ignore patterns on currentFolder', async function () {
            $scope.editFolder({ id: 'default', path: '/var/sync', devices: [] });
            await Promise.resolve();
            // currentFolder.ignores is separate from IgnoresService data. It is
            // not updated when patterns are modified in the UI, then is used to
            // check for changes when it's time to save the folder.
            expect($scope.currentFolder.ignores).toEqual(['/Backups', '*']);
        });

        it('sets error text when ignore fetch fails', async function () {
            ignoresSpy.and.returnValue(Promise.reject())
            $scope.editFolder({ id: 'default', path: '/var/sync', devices: [] });
            await Promise.resolve();
            await Promise.resolve();
            expect($scope.ignores.error).toMatch(/Failed/);
        });
    });

    describe('parseIgnores', function() {
        it('determines patterns are too advanced for basic UI', function () {
            controller = $controller('SyncthingController', { $scope: $scope });
            $scope.parseIgnores('/Backups\n/*.txt');
            expect($scope.currentFolder.ignoreIsBasic).toBeFalse();
        });

        it('determines basic UI is available', function () {
            controller = $controller('SyncthingController', { $scope: $scope });
            $scope.parseIgnores('/Backups/Archived\n*');
            expect($scope.currentFolder.ignoreIsBasic).toBeTrue();
        });
    });
});
