/**
 * Groups input in chunks of the specified size
 *
 * E.g. [1, 2, 3, 4, 5] with groupSize = 3 => [[1, 2, 3], [4, 5]]
 * Uses pmkr.memoize to avoid infdig, see 'Johnny Hauser's "Filter Stablize" Solution'
 * here: http://sobrepere.com/blog/2014/10/14/creating-groupby-filter-angularjs/
 */
angular.module('syncthing.core')
    .filter('group', [
        'pmkr.filterStabilize', 
        function (stabilize) {
            return stabilize(function(items, groupSize) {
                var groups = [];
                var inner;
                for (var i = 0; i < items.length; i++) {
                    if (i % groupSize === 0) {
                        inner = [];
                        groups.push(inner);
                    }
                    inner.push(items[i]);
                }
                return groups;
            });
        }
    ]);
