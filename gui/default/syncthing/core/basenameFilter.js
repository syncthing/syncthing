angular.module('syncthing.core')
    .filter('basename', function () {
        return function (input) {
            if (input === undefined)
                return "";
            var parts = input.split(/[\/\\]/);
            if (!parts || parts.length < 1) {
                return input;
            }
            return parts[parts.length - 1];
        };
    });
