angular.module('syncthing.core')
    .directive('uniqueFolder', function () {
        return {
            require: 'ngModel',
            link: function (scope, elm, attrs, ctrl) {
                ctrl.$parsers.unshift(function (viewValue) {
                    if (!scope.editingFolderNew()) {
                        // we shouldn't validate
                        ctrl.$setValidity('uniqueFolder', true);
                    } else if (scope.folders.hasOwnProperty(viewValue)) {
                        // the folder exists already
                        ctrl.$setValidity('uniqueFolder', false);
                    } else {
                        // the folder is unique
                        ctrl.$setValidity('uniqueFolder', true);
                    }
                    return viewValue;
                });
            }
        };
    });
