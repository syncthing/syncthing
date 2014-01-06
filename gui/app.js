var syncthing = angular.module('syncthing', []);

syncthing.controller('SyncthingCtrl', function ($scope, $http) {
    $http.get("/rest/version").success(function (data) {
        $scope.version = data;
    });
    $http.get("/rest/config").success(function (data) {
        $scope.config = data;
    });
    
    var prevDate = 0;

    $scope.refresh = function () {
        $http.get("/rest/model").success(function (data) {
            $scope.model = data;
        });
        $http.get("/rest/connections").success(function (data) {
            var now = Date.now();
            var td = (now - prevDate) / 1000;
            prevDate = now;

            for (var id in data) {
                try {
                    data[id].inbps = 8 * (data[id].InBytesTotal - $scope.connections[id].InBytesTotal) / td;
                    data[id].outbps = 8 * (data[id].OutBytesTotal - $scope.connections[id].OutBytesTotal) / td;
                } catch (e) {
                    data[id].inbps = 0;
                    data[id].outbps = 0;
                }
            }
            $scope.connections = data;
        });
        $http.get("/rest/need").success(function (data) {
            var i, name;
            for (i = 0; i < data.length; i++) {
                name = data[i].Name.split("/");
                data[i].ShortName = name[name.length-1];
            }
            data.sort(function (a, b) {
                if (a.ShortName < b.ShortName) {
                    return -1;
                }
                if (a.ShortName > b.ShortName) {
                    return 1;
                }
                return 0;
            });
            $scope.need = data;
        });
    };

    $scope.refresh();
    setInterval($scope.refresh, 10000);
});

function decimals(num) {
    if (num > 100) {
        return 0;
    }
    if (num > 10) {
        return 1;
    }
    return 2;
}

syncthing.filter('binary', function() {
    return function(input) {
        if (input === undefined) {
            return '- '
        }
        if (input > 1024 * 1024 * 1024) {
            input /= 1024 * 1024 * 1024;
            return input.toFixed(decimals(input)) + ' Gi';
        }
        if (input > 1024 * 1024) {
            input /= 1024 * 1024;
            return input.toFixed(decimals(input)) + ' Mi';
        }
        if (input > 1024) {
            input /= 1024;
            return input.toFixed(decimals(input)) + ' Ki';
        }
        return Math.round(input) + ' ';
    }
});

syncthing.filter('metric', function() {
    return function(input) {
        if (input === undefined) {
            return '- '
        }
        if (input > 1000 * 1000 * 1000) {
            input /= 1000 * 1000 * 1000;
            return input.toFixed(decimals(input)) + ' G';
        }
        if (input > 1000 * 1000) {
            input /= 1000 * 1000;
            return input.toFixed(decimals(input)) + ' M';
        }
        if (input > 1000) {
            input /= 1000;
            return input.toFixed(decimals(input)) + ' k';
        }
        return Math.round(input) + ' ';
    }
});

syncthing.filter('short', function() {
    return function(input) {
        return input.substr(0, 6);
    }
});

syncthing.filter('alwaysNumber', function() {
    return function(input) {
        if (input === undefined) {
            return 0;
        }
        return input;
    }
});
