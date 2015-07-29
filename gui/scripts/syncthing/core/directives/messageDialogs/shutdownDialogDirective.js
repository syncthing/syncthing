angular.module('syncthing.core')
    .directive('shutdownDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/core/views/directives/messageDialogs/shutdownDialogView.html'
        };
});
