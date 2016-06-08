angular.module('syncthing.core')
    .directive('modal', function () {
        return {
            restrict: 'E',
            templateUrl: 'modal.html',
            replace: true,
            transclude: true,
            scope: {
                heading: '@',
                status: '@',
                icon: '@',
                close: '@',
                large: '@'
            }
        };
    });
