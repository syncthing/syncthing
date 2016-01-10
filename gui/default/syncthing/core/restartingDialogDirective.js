angular.module('syncthing.core')
    .directive('restartingDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/core/restartingDialogView.html'
        };
});
