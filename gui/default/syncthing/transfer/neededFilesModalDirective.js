angular.module('syncthing.transfer')
    .directive('neededFilesModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/transfer/neededFilesModalView.html'
        };
});
