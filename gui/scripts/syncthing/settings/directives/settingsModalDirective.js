angular.module('syncthing.settings')
    .directive('settingsModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/settings/views/directives/settingsModalView.html'
        };
});
