angular.module('syncthing.device')
    .directive('editDeviceModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/device/views/directives/editDeviceModalView.html'
        };
});
