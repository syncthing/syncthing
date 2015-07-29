angular.module('syncthing.core')
    .directive('httpErrorDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'scripts/syncthing/core/views/directives/messageDialogs/httpErrorDialogView.html'
        };
});
