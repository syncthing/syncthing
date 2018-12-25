angular.module('syncthing.core')
    .directive('notification', function () {
        return {
            restrict: 'E',
            scope: {},
            transclude: true,
            template: '<div class="row" ng-if="visible()"><div class="col-md-12" ng-transclude></div></div>',
            link: function (scope, elm, attrs) {
                scope.visible = function () {
                    return scope.$parent.config.options.unackedNotificationIDs.indexOf(attrs.id) > -1;
                };
            }
        };
    });
