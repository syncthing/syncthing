angular.module('syncthing.core')
    .directive('networkErrorDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/core/networkErrorDialogView.html'
        };
});
