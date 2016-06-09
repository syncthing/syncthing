angular.module('syncthing.core')
    .directive('view', function () {
        return {
            restrict: 'E',
            templateUrl: function(element, attrs) {
                return attrs.templateUrl;
            }
        };
});
