/** convert amount of seconds to string format "d h m s" without zero values
 * precision must be one of 'd', 'h', 'm', 's'(default)
 * Example:
 * {{121020003|duration}}     --> 1400d 16h 40m 3s
 * {{121020003|duration:"m"}} --> 1400d 16h 40m
 * {{121020003|duration:"h"}} --> 1400d 16h
 * {{1|duration:"h"}}         --> <1h
**/
angular.module('syncthing.core')
    .filter('duration', function ($translate) {
        'use strict';

        var SECONDS_IN = { "d": 86400, "h": 3600, "m": 60, "s": 1 };
        return function (input, precision) {
            var milliseconds = parseInt(input, 10) * 1000;
            var units = ["d", "h", "m", "s"];
            switch (precision) {
                case "d":
                    units.pop();
                    // fallthrough
                case "h":
                    units.pop();
                    // fallthrough
                case "m":
                    units.pop();
                    // fallthrough
                case "s":
                    break
                default:
                    return "[Error: incorrect usage, precision must be one of d, h, m or s]";
            }

            let language_cc = $translate.use();
            if (language_cc != null) {
                language_cc = language_cc.replace("-", "_");
                var fallbacks = [];
                var language = language_cc.substr(0, 2);
                switch (language) {
                case "zh":
                    // Use zh_TW for zh_HK
                    fallbacks.push("zh_TW");
                    break
                }
                if (language != language_cc) {
                    fallbacks.push(language);
                }
                // Fallback to english, if the language isn't found
                fallbacks.push("en");
                try {
                    return humanizeDuration(milliseconds, {
                        language: language_cc,
                        maxDecimalPoints: 0,
                        units: units,
                        fallbacks: fallbacks
                    });
                } catch(err) {
                    console.log(err.message + ": language_cc=" + language_cc)
                    // if we crash, fallthrough to english
                }
            }
            var result = "";
            if (!precision) {
                precision = "s";
            }
            input = parseInt(input, 10);
            for (var k in SECONDS_IN) {
                var t = (input / SECONDS_IN[k] | 0); // Math.floor

                if (t > 0) {
                    if (!result) {
                        result = t + k;
                    } else {
                        result += " " + t + k;
                    }
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
