angular.module('syncthing.core')
    .filter('metric', function () {
        return function (input) {
            if (input === undefined || isNaN(input)) {
                return '0 ';
            }
            if (input > 1000 * 1000 * 1000) {
                input /= 1000 * 1000 * 1000;
                return input.toLocaleString(undefined, {maximumFractionDigits: 2}) + ' G';
            }
            if (input > 1000 * 1000) {
                input /= 1000 * 1000;
                return input.toLocaleString(undefined, {maximumFractionDigits: 2}) + ' M';
            }
            if (input > 1000) {
                input /= 1000;
                return input.toLocaleString(undefined, {maximumFractionDigits: 2}) + ' k';
            }
            return Math.round(input).toLocaleString(undefined, {maximumFractionDigits: 2}) + ' ';
        };
    });
