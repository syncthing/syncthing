angular.module('syncthing.settings')
    .directive('advancedSettingsModal', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/settings/advancedSettingsModalView.html'
        };
});
