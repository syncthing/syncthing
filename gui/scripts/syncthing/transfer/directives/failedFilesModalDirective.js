angular.module('syncthing.transfer')
    .directive('failedFilesModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/transfer/views/directives/failedFilesModalView.html'
        };
});
