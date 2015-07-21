angular.module('syncthing.transfer')
    .directive('neededFilesModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/transfer/views/directives/neededFilesModalView.html'
        };
});
