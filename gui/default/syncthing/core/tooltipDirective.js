angular.module('syncthing.core')
    .directive('tooltip', function () {
        return {
            restrict: 'A',
            link: function (scope, element, attributes) {
                $(element).tooltip();
            }
        };
    });
