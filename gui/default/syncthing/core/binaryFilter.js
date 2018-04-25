angular.module('syncthing.core')
    .filter('binary', function () {
        return function (input) {
            if (input === undefined || isNaN(input)) {
                return '0 ';
            }
            if (input > 1024 * 1024 * 1024) {
                input /= 1024 * 1024 * 1024;
                return input.toLocaleString(undefined, {maximumFractionDigits: 2}) + ' Gi';
            }
            if (input > 1024 * 1024) {
                input /= 1024 * 1024;
                return input.toLocaleString(undefined, {maximumFractionDigits: 2}) + ' Mi';
            }
            if (input > 1024) {
                input /= 1024;
                return input.toLocaleString(undefined, {maximumFractionDigits: 2}) + ' Ki';
            }
            return Math.round(input).toLocaleString(undefined, {maximumFractionDigits: 2}) + ' ';
        };
    });
