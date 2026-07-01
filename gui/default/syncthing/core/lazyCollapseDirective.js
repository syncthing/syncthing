angular.module('syncthing.core')
    .directive('lazyCollapse', function () {
        return {
            restrict: 'A',
            link: function (scope, element) {
                // Render panel content only while expanded. Collapsed panels
                // contribute zero watchers. Bootstrap's show/hidden events
                // fire before/after the animation, so the DOM is present
                // during the transition.
                scope.lazyReady = element.hasClass('in');

                $(element).on('show.bs.collapse', function () {
                    scope.$apply(function () {
                        scope.lazyReady = true;
                    });
                });

                $(element).on('hidden.bs.collapse', function () {
                    scope.$apply(function () {
                        scope.lazyReady = false;
                    });
                });
            }
        };
    });
