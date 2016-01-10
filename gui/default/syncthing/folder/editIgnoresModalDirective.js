angular.module('syncthing.folder')
    .directive('editIgnoresModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/folder/editIgnoresModalView.html'
        };
});
