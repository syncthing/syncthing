angular.module('syncthing.core')
    .filter('binary', function () {
        return function (input) {
            if (input === undefined || isNaN(input)) {
                return '0 ';
            }
            if (input > 1024 * 1024 * 1024 * 1024 * 1024) {
                // Don't show any decimals for more than 4 digits
                input /= 1024 * 1024 * 1024 * 1024;
                return input.toLocaleString(undefined, {maximumFractionDigits: 0}) + ' Ti';
            }
            // Show 3 significant digits (e.g. 123Ti or 2.54Ti)
            if (input > 1024 * 1024 * 1024 * 1024) {
                input /= 1024 * 1024 * 1024 * 1024;
                return input.toLocaleString(undefined, {maximumSignificantDigits: 3}) + ' Ti';
            }
            if (input > 1024 * 1024 * 1024) {
                input /= 1024 * 1024 * 1024;
                return input.toLocaleString(undefined, {maximumSignificantDigits: 3}) + ' Gi';
            }
            if (input > 1024 * 1024) {
                input /= 1024 * 1024;
                return input.toLocaleString(undefined, {maximumSignificantDigits: 3}) + ' Mi';
            }
            if (input > 1024) {
                input /= 1024;
                return input.toLocaleString(undefined, {maximumSignificantDigits: 3}) + ' Ki';
            }
            return Math.round(input).toLocaleString() + ' ';
        };
    });
