angular.module('syncthing.core')
    .directive('notification', function () {
        return {
            restrict: 'E',
            scope: true,
            transclude: true,
            template: '<div class="row" ng-if="visible()"><div class="col-md-12" ng-transclude></div></div>',
            link: function (scope, elm, attrs) {
                scope.visible = function () {
                    return scope.config.options.unackedNotificationIDs.indexOf(attrs.id) > -1;
                }
                scope.dismiss = function () {
                    var idx = scope.config.options.unackedNotificationIDs.indexOf(attrs.id);
                    if (idx > -1) {
                        scope.config.options.unackedNotificationIDs.splice(idx, 1);
                        scope.saveConfig();
                    }
                }
            }
        };
});
