angular.module('syncthing.core')
    .directive('upgradingDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/core/upgradingDialogView.html'
        };
});
