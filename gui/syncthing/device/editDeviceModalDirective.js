angular.module('syncthing.device')
    .directive('editDeviceModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/device/editDeviceModalView.html'
        };
});
