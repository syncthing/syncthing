angular.module('syncthing.core')
    .directive('aboutModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/core/views/directives/aboutModalView.html'
        };
});
