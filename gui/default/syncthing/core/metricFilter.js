angular.module('syncthing.core')
    .filter('metric', function () {
        return function (input) {
            if (input === undefined || isNaN(input)) {
                return '0 ';
            }
            if (input > 1000 * 1000 * 1000 * 1000 * 1000) {
                // Don't show any decimals for more than 4 digits
                input /= 1000 * 1000 * 1000 * 1000;
                return input.toLocaleString(undefined, {maximumFractionDigits: 0}) + ' T';
            }
            // Show 3 significant digits (e.g. 123T or 2.54T)
            if (input > 1000 * 1000 * 1000 * 1000) {
                input /= 1000 * 1000 * 1000 * 1000;
                return input.toLocaleString(undefined, {maximumSignificantDigits: 3}) + ' T';
            }
            if (input > 1000 * 1000 * 1000) {
                input /= 1000 * 1000 * 1000;
                return input.toLocaleString(undefined, {maximumSignificantDigits: 3}) + ' G';
            }
            if (input > 1000 * 1000) {
                input /= 1000 * 1000;
                return input.toLocaleString(undefined, {maximumSignificantDigits: 3}) + ' M';
            }
            if (input > 1000) {
                input /= 1000;
                return input.toLocaleString(undefined, {maximumSignificantDigits: 3}) + ' k';
            }
            return Math.round(input).toLocaleString() + ' ';
        };
    });
