angular.module('syncthing.core')
    .directive('majorUpgradeModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/core/views/directives/majorUpgradeModalView.html'
        };
});
