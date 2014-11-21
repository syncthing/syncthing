angular.module('syncthing.core')
    .filter('alwaysNumber', function () {
        return function (input) {
            if (input === undefined) {
                return 0;
            }
            return input;
        };
    });
