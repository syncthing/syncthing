angular.module('syncthing.core')
    .filter('localeNumber', function () {
        return function (input, decimals) {
            if (typeof(decimals) !== 'undefined') {
                return input.toLocaleString(undefined, {maximumFractionDigits: decimals});
            }
            return input.toLocaleString();
        };
    });