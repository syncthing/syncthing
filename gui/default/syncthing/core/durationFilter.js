/** convert amount of seconds to string format "d h m s" without zero values
 * precision must be one of 'd', 'h', 'm', 's'(default)
 * Example:
 * {{121020003|duration}}     --> 1400d 16h 40m 3s
 * {{121020003|duration:"m"}} --> 1400d 16h 40m
 * {{121020003|duration:"h"}} --> 1400d 16h
 * {{1|duration:"h"}}         --> <1h
**/
angular.module('syncthing.core')
    .filter('duration', function () {
        'use strict';

        var SECONDS_IN = { "d": 86400, "h": 3600, "m": 60, "s": 1 };
        return function (input, precision) {
            var result = "";
            if (!precision) {
                precision = "s";
            }
            input = parseInt(input, 10);
            for (var k in SECONDS_IN) {
                var t = (input / SECONDS_IN[k] | 0); // Math.floor

                if (t > 0) {
                    if (result) {
                        result += " ";
                    }
                    result += t + k;
                }

                if (precision == k) {
                    return result ? result : "<1" + k;
                } else {
                    input %= SECONDS_IN[k];
                }
            }
            return "[Error: incorrect usage, precision must be one of " + Object.keys(SECONDS_IN) + "]";
        };
    });
