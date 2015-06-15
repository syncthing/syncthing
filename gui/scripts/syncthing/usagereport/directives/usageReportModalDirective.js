angular.module('syncthing.usagereport')
    .directive('usageReportModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/usagereport/views/directives/usageReportModalView.html'
        };
});
