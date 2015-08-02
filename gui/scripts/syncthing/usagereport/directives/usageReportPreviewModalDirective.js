angular.module('syncthing.usagereport')
    .directive('usageReportPreviewModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/usagereport/views/directives/usageReportPreviewModalView.html'
        };
});
