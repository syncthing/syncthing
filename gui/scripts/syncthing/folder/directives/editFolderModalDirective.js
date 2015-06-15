angular.module('syncthing.folder')
    .directive('editFolderModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/folder/views/directives/editFolderModalView.html'
        };
});
