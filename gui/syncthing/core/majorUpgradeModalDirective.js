angular.module('syncthing.core')
    .directive('majorUpgradeModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/core/majorUpgradeModalView.html'
        };
});
