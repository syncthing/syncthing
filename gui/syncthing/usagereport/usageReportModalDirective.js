angular.module('syncthing.usagereport')
    .directive('usageReportModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/usagereport/usageReportModalView.html'
        };
});
