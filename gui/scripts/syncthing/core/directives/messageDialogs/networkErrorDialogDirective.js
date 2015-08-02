angular.module('syncthing.core')
    .directive('networkErrorDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/core/views/directives/messageDialogs/networkErrorDialogView.html'
        };
});
