angular.module('syncthing.transfer')
    .directive('failedFilesModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/transfer/failedFilesModalView.html'
        };
});
