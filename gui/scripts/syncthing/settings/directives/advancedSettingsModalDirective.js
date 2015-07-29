angular.module('syncthing.settings')
    .directive('advancedSettingsModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/settings/views/directives/advancedSettingsModalView.html'
        };
});
