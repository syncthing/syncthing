angular.module('syncthing.core')
    .directive('shutdownDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/core/shutdownDialogView.html'
        };
});
