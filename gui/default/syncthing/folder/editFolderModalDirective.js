angular.module('syncthing.folder')
    .directive('editFolderModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/folder/editFolderModalView.html'
        };
});
