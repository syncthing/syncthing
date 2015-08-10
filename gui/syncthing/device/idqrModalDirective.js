angular.module('syncthing.device')
    .directive('idqrModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/device/idqrModalView.html'
        };
});
