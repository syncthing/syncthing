angular.module('syncthing.core')
    .controller('EventController', function ($scope, $http) {
        'use strict';

        $scope.lastEvent = null;
        var lastID = 0;

        var successFn = function (data) {
            // When Syncthing restarts while the long polling connection is in
            // progress the browser on some platforms returns a 200 (since the
            // headers has been flushed with the return code 200), with no data.
            // This basically means that the connection has been reset, and the call
            // was not actually sucessful.
            if (!data) {
                errorFn(data);
                return;
            }

            $scope.$emit('UIOnline');

            if (lastID > 0) {
                data.forEach(function (event) {
                    console.log("event", event.id, event.type, event.data);
                    $scope.$emit(event.type, event);
                });
            }

            $scope.lastEvent = data[data.length - 1];
            lastID = $scope.lastEvent.id;

            setTimeout(function () {
                $http.get(urlbase + '/events?since=' + lastID)
                    .success(successFn)
                    .error(errorFn);
            }, 500);
        };

        var errorFn = function (data) {
            $scope.$emit('UIOffline');

            setTimeout(function () {
                $http.get(urlbase + '/events?limit=1')
                    .success(successFn)
                    .error(errorFn);
            }, 1000);
        };

        $http.get(urlbase + '/events?limit=1')
            .success(successFn)
            .error(errorFn);
    });
