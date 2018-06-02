angular.module('syncthing.core')
    .filter('binary', function () {
        return function (input) {
            return unitPrefixed(input, true);
        };
    });
