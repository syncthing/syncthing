angular.module('syncthing.settings')
    .directive('settingsModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/settings/settingsModalView.html'
        };
});
