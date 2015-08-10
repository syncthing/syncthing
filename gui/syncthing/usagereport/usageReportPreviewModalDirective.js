angular.module('syncthing.usagereport')
    .directive('usageReportPreviewModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/usagereport/usageReportPreviewModalView.html'
        };
});
