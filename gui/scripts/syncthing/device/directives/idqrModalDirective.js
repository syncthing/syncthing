angular.module('syncthing.device')
    .directive('idqrModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/device/views/directives/idqrModalView.html'
        };
});
