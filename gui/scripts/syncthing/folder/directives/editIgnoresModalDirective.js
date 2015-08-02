angular.module('syncthing.folder')
    .directive('editIgnoresModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/folder/views/directives/editIgnoresModalView.html'
        };
});
