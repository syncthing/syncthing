angular.module('syncthing.core')
    .filter('localeNumber', function () {
        return function (input) {
            return input.toLocaleString();
        };
    });
