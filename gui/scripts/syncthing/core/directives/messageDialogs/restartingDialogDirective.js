angular.module('syncthing.core')
    .directive('restartingDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/core/views/directives/messageDialogs/restartingDialogView.html'
        };
});
