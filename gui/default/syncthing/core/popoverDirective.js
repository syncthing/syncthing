angular.module('syncthing.core')
    .directive('popover', function () {
        return {
            restrict: 'A',
            link: function (scope, element, attributes) {
                $(element).popover();
            }
        };
    });
