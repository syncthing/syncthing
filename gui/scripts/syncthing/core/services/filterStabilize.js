/**
 * m59peacemaker's filterStabilize
 *
 * See https://github.com/m59peacemaker/angular-pmkr-components/tree/master/src/services/filterStabilize
 * Released under the MIT license
 */
angular.module('syncthing.core')
    .factory('pmkr.filterStabilize', [
        'pmkr.memoize',
        function(memoize) {
            function service(fn) {
                function filter() {
                    var args = [].slice.call(arguments);
                    // always pass a copy of the args so that the original input can't be modified
                    args = angular.copy(args);
                    // return the `fn` return value or input reference (makes `fn` return optional)
                    var filtered = fn.apply(this, args) || args[0];
                    return filtered;
                }

                var memoized = memoize(filter);
                return memoized;

            }
            return service;
        }
    ]);
