angular.module('syncthing.core')
    .directive('aboutModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/core/aboutModalView.html'
        };
});
