angular.module('syncthing.core')
    .directive('httpErrorDialog', function () {
        return {
            restrict: 'A',
            templateUrl: 'syncthing/core/httpErrorDialogView.html'
        };
});
