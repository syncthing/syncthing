angular.module('syncthing.core')
    .filter('metric', function () {
        return function (input) {
            if (input === undefined || isNaN(input)) {
                return '0 ';
            }
            if (input > 1000 * 1000 * 1000) {
                input /= 1000 * 1000 * 1000;
                return input.toFixed(decimals(input, 2)) + ' G';
            }
            if (input > 1000 * 1000) {
                input /= 1000 * 1000;
                return input.toFixed(decimals(input, 2)) + ' M';
            }
            if (input > 1000) {
                input /= 1000;
                return input.toFixed(decimals(input, 2)) + ' k';
            }
            return Math.round(input) + ' ';
        };
    });
