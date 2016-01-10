angular.module('syncthing.core')
    .filter('lastErrorComponent', function () {
        return function (input) {
            if (input === undefined)
                return "";
            var parts = input.split(/:\s*/);
            if (!parts || parts.length < 1) {
                return input;
            }
            return parts[parts.length - 1];
        };
    });
