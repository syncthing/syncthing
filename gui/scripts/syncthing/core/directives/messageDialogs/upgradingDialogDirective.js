angular.module('syncthing.core')
    .directive('upgradingDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/core/views/directives/messageDialogs/upgradingDialogView.html'
        };
});
