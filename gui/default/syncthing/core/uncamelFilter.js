angular.module('syncthing.core')
    .filter('uncamel', function () {
        return function (input) {
            input = input.replace(/(.)([A-Z][a-z]+)/g, '$1 $2').replace(/([a-z0-9])([A-Z])/g, '$1 $2');
            var parts = input.split(' ');
            var lastPart = parts.splice(-1)[0];
            switch (lastPart) {
                case "S":
                    parts.push('(seconds)');
                    break;
                case "M":
                    parts.push('(minutes)');
                    break;
                case "H":
                    parts.push('(hours)');
                    break;
                case "Ms":
                    parts.push('(milliseconds)');
                    break;
                default:
                    parts.push(lastPart);
                    break;
            }
            input = parts.join(' ');
            return input.charAt(0).toUpperCase() + input.slice(1);
        };
    });
