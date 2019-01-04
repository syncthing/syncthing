angular.module('syncthing.core')
    .filter('percent', function () {
        return function (input) {
            // Prevent 0.00%
            if (input === undefined || input < 0.01) {
                return 0 + '%';
            }
            // Hard limit at two decimals
            if (input < 0.1) {
                return input.toLocaleString(undefined, { maximumFractionDigits: 2 }) + '%';
            }
            // "Soft" limit at two significant digits (e.g. 1.2%, not 1.27%)
            return input.toLocaleString(undefined, { maximumSignificantDigits: 2 }) + '%';
        };
    });
