/**
 * m59peacemaker's memoize
 *
 * See https://github.com/m59peacemaker/angular-pmkr-components/tree/master/src/memoize
 * Released under the MIT license
 */
angular.module('syncthing.core')
    .factory('pmkr.memoize', [
        function() {
            function service() {
                return memoizeFactory.apply(this, arguments);
            }
            function memoizeFactory(fn) {
                var cache = {};
                function memoized() {
                    var args = [].slice.call(arguments);
                    var key = JSON.stringify(args);
                    if (cache.hasOwnProperty(key)) {
                        return cache[key];
                    }
                    cache[key] = fn.apply(this, arguments);
                    return cache[key];
                }
                return memoized;
            }
            return service;
        }
    ]);
