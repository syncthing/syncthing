angular.module('syncthing.core')
    .filter('natural', function () {
        return function (input, valid) {
            return input.toFixed(decimals(input, valid));
        };
    });
