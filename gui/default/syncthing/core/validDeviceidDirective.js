angular.module('syncthing.core')
    .directive('validDeviceid', function ($http) {
        return {
            require: 'ngModel',
            link: function (scope, elm, attrs, ctrl) {
                ctrl.$parsers.unshift(function (viewValue) {
                    $http.get(urlbase + '/svc/deviceid?id=' + viewValue).success(function (resp) {
                        let isValid = !resp.error;
                        let isUnique = !isValid || !scope.devices.hasOwnProperty(resp.id);

                        ctrl.$setValidity('validDeviceid', isValid);
                        ctrl.$setValidity('unique', isUnique);
                    });
                    return viewValue;
                });
            }
        };
    });
