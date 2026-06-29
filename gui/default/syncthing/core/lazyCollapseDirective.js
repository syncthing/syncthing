angular.module('syncthing.core')
    .directive('lazyCollapse', function () {
        return {
            restrict: 'A',
            link: function (scope, element) {
                // Start collapsed: content is not rendered until first expand.
                // Once expanded, content stays in DOM (Bootstrap needs it for
                // the collapse animation). This avoids thousands of idle
                // watchers for panels that are never opened.
                scope.lazyReady = element.hasClass('in');

                $(element).on('show.bs.collapse', function () {
                    scope.$apply(function () {
                        scope.lazyReady = true;
                    });
                });
            }
        };
    });
