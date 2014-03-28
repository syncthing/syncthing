/*jslint browser: true, continue: true, plusplus: true */
/*global $: false, angular: false */

'use strict';

var syncthing = angular.module('syncthing', []);

syncthing.controller('SyncthingCtrl', function ($scope, $http) {
    var prevDate = 0,
        modelGetOK = true;

    $scope.connections = {};
    $scope.config = {};
    $scope.myID = '';
    $scope.nodes = [];
    $scope.configInSync = true;
    $scope.errors = [];
    $scope.seenError = '';

    // Strings before bools look better
    $scope.settings = [
        {id: 'ListenStr', descr: 'Sync Protocol Listen Addresses', type: 'text', restart: true},
        {id: 'GUIAddress', descr: 'GUI Listen Address', type: 'text', restart: true},
        {id: 'MaxSendKbps', descr: 'Outgoing Rate Limit (KBps)', type: 'number', restart: true},
        {id: 'RescanIntervalS', descr: 'Rescan Interval (s)', type: 'number', restart: true},
        {id: 'ReconnectIntervalS', descr: 'Reconnect Interval (s)', type: 'number', restart: true},
        {id: 'ParallelRequests', descr: 'Max Outstanding Requests', type: 'number', restart: true},
        {id: 'MaxChangeKbps', descr: 'Max File Change Rate (KBps)', type: 'number', restart: true},

        {id: 'ReadOnly', descr: 'Read Only', type: 'bool', restart: true},
        {id: 'FollowSymlinks', descr: 'Follow Symlinks', type: 'bool', restart: true},
        {id: 'GlobalAnnEnabled', descr: 'Global Announce', type: 'bool', restart: true},
        {id: 'LocalAnnEnabled', descr: 'Local Announce', type: 'bool', restart: true},
        {id: 'StartBrowser', descr: 'Start Browser', type: 'bool'},
    ];

    function modelGetSucceeded() {
        if (!modelGetOK) {
            $('#networkError').modal('hide');
            modelGetOK = true;
        }
    }

    function modelGetFailed() {
        if (modelGetOK) {
            $('#networkError').modal({backdrop: 'static', keyboard: false});
            modelGetOK = false;
        }
    }

    function nodeCompare(a, b) {
        if (a.NodeID === $scope.myID) {
            return -1;
        }
        if (b.NodeID === $scope.myID) {
            return 1;
        }
        if (a.NodeID < b.NodeID) {
            return -1;
        }
        return a.NodeID > b.NodeID;
    }

    $http.get('/rest/version').success(function (data) {
        $scope.version = data;
    });
    $http.get('/rest/system').success(function (data) {
        $scope.system = data;
        $scope.myID = data.myID;

        $http.get('/rest/config').success(function (data) {
            $scope.config = data;
            $scope.config.Options.ListenStr = $scope.config.Options.ListenAddress.join(', ');

            var nodes = $scope.config.Repositories[0].Nodes;
            nodes.sort(nodeCompare);
            $scope.nodes = nodes;
        });
        $http.get('/rest/config/sync').success(function (data) {
            $scope.configInSync = data.configInSync;
        });
    });

    $scope.refresh = function () {
        $http.get('/rest/system').success(function (data) {
            $scope.system = data;
        });
        $http.get('/rest/model').success(function (data) {
            $scope.model = data;
            modelGetSucceeded();
        }).error(function () {
            modelGetFailed();
        });
        $http.get('/rest/connections').success(function (data) {
            var now = Date.now(),
                td = (now - prevDate) / 1000,
                id;

            prevDate = now;
            $scope.inbps = 0;
            $scope.outbps = 0;

            for (id in data) {
                if (!data.hasOwnProperty(id)) {
                    continue;
                }
                try {
                    data[id].inbps = Math.max(0, 8 * (data[id].InBytesTotal - $scope.connections[id].InBytesTotal) / td);
                    data[id].outbps = Math.max(0, 8 * (data[id].OutBytesTotal - $scope.connections[id].OutBytesTotal) / td);
                } catch (e) {
                    data[id].inbps = 0;
                    data[id].outbps = 0;
                }
                $scope.inbps += data[id].inbps;
                $scope.outbps += data[id].outbps;
            }
            $scope.connections = data;
        });
        $http.get('/rest/need').success(function (data) {
            var i, name;
            for (i = 0; i < data.length; i++) {
                name = data[i].Name.split('/');
                data[i].ShortName = name[name.length - 1];
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
        $http.get('/rest/errors').success(function (data) {
            $scope.errors = data;
        });
    };

    $scope.nodeStatus = function (nodeCfg) {
        var conn = $scope.connections[nodeCfg.NodeID];
        if (conn) {
            if (conn.Completion === 100) {
                return 'In Sync';
            } else {
                return 'Syncing (' + conn.Completion + '%)';
            }
        }

        return 'Disconnected';
    };

    $scope.nodeIcon = function (nodeCfg) {
        var conn = $scope.connections[nodeCfg.NodeID];
        if (conn) {
            if (conn.Completion === 100) {
                return 'ok';
            } else {
                return 'refresh';
            }
        }

        return 'minus';
    };

    $scope.nodeClass = function (nodeCfg) {
        var conn = $scope.connections[nodeCfg.NodeID];
        if (conn) {
            if (conn.Completion === 100) {
                return 'success';
            } else {
                return 'primary';
            }
        }

        return 'info';
    };

    $scope.nodeAddr = function (nodeCfg) {
        var conn = $scope.connections[nodeCfg.NodeID];
        if (conn) {
            return conn.Address;
        }
        return '(unknown address)';
    };

    $scope.nodeCompletion = function (nodeCfg) {
        var conn = $scope.connections[nodeCfg.NodeID];
        if (conn) {
            return conn.Completion + '%';
        }
        return '';
    };

    $scope.nodeVer = function (nodeCfg) {
        if (nodeCfg.NodeID === $scope.myID) {
            return $scope.version;
        }
        var conn = $scope.connections[nodeCfg.NodeID];
        if (conn) {
            return conn.ClientVersion;
        }
        return '(unknown version)';
    };

    $scope.nodeName = function (nodeCfg) {
        if (nodeCfg.Name) {
            return nodeCfg.Name;
        }
        return nodeCfg.NodeID.substr(0, 6);
    };

    $scope.saveSettings = function () {
        $scope.configInSync = false;
        $scope.config.Options.ListenAddress = $scope.config.Options.ListenStr.split(',').map(function (x) { return x.trim(); });
        $http.post('/rest/config', JSON.stringify($scope.config), {headers: {'Content-Type': 'application/json'}});
        $('#settingsTable').collapse('hide');
    };

    $scope.restart = function () {
        $http.post('/rest/restart');
        $scope.configInSync = true;
    };

    $scope.editNode = function (nodeCfg) {
        $scope.currentNode = nodeCfg;
        $scope.editingExisting = true;
        $scope.currentNode.AddressesStr = nodeCfg.Addresses.join(', ');
        $('#editNode').modal({backdrop: 'static', keyboard: false});
    };

    $scope.addNode = function () {
        $scope.currentNode = {NodeID: '', AddressesStr: 'dynamic'};
        $scope.editingExisting = false;
        $('#editNode').modal({backdrop: 'static', keyboard: false});
    };

    $scope.deleteNode = function () {
        var newNodes = [], i;

        $('#editNode').modal('hide');
        if (!$scope.editingExisting) {
            return;
        }

        for (i = 0; i < $scope.nodes.length; i++) {
            if ($scope.nodes[i].NodeID !== $scope.currentNode.NodeID) {
                newNodes.push($scope.nodes[i]);
            }
        }

        $scope.nodes = newNodes;
        $scope.config.Repositories[0].Nodes = newNodes;

        $scope.configInSync = false;
        $http.post('/rest/config', JSON.stringify($scope.config), {headers: {'Content-Type': 'application/json'}});
    };

    $scope.saveNode = function () {
        var nodeCfg, done, i;

        $scope.configInSync = false;
        $('#editNode').modal('hide');
        nodeCfg = $scope.currentNode;
        nodeCfg.Addresses = nodeCfg.AddressesStr.split(',').map(function (x) { return x.trim(); });

        done = false;
        for (i = 0; i < $scope.nodes.length; i++) {
            if ($scope.nodes[i].NodeID === nodeCfg.NodeID) {
                $scope.nodes[i] = nodeCfg;
                done = true;
                break;
            }
        }

        if (!done) {
            $scope.nodes.push(nodeCfg);
        }

        $scope.nodes.sort(nodeCompare);
        $scope.config.Repositories[0].Nodes = $scope.nodes;

        $http.post('/rest/config', JSON.stringify($scope.config), {headers: {'Content-Type': 'application/json'}});
    };

    $scope.otherNodes = function () {
        var nodes = [], i, n;

        for (i = 0; i < $scope.nodes.length; i++) {
            n = $scope.nodes[i];
            if (n.NodeID !== $scope.myID) {
                nodes.push(n);
            }
        }
        return nodes;
    };

    $scope.thisNode = function () {
        var i, n;

        for (i = 0; i < $scope.nodes.length; i++) {
            n = $scope.nodes[i];
            if (n.NodeID === $scope.myID) {
                return [n];
            }
        }
    };

    $scope.errorList = function () {
        var errors = [];
        for (var i = 0; i < $scope.errors.length; i++) {
            var e = $scope.errors[i];
            if (e.Time > $scope.seenError) {
                errors.push(e);
            }
        }
        return errors;
    };

    $scope.clearErrors = function () {
        $scope.seenError = $scope.errors[$scope.errors.length - 1].Time;
    };

    $scope.friendlyNodes = function (str) {
        for (var i = 0; i < $scope.nodes.length; i++) {
            var cfg = $scope.nodes[i];
            str = str.replace(cfg.NodeID, $scope.nodeName(cfg));
        }
        return str;
    };

    $scope.refresh();
    setInterval($scope.refresh, 10000);
});

function decimals(val, num) {
    var digits, decs;

    if (val === 0) {
        return 0;
    }

    digits = Math.floor(Math.log(Math.abs(val)) / Math.log(10));
    decs = Math.max(0, num - digits);
    return decs;
}

syncthing.filter('natural', function () {
    return function (input, valid) {
        return input.toFixed(decimals(input, valid));
    };
});

syncthing.filter('binary', function () {
    return function (input) {
        if (input === undefined) {
            return '0 ';
        }
        if (input > 1024 * 1024 * 1024) {
            input /= 1024 * 1024 * 1024;
            return input.toFixed(decimals(input, 2)) + ' Gi';
        }
        if (input > 1024 * 1024) {
            input /= 1024 * 1024;
            return input.toFixed(decimals(input, 2)) + ' Mi';
        }
        if (input > 1024) {
            input /= 1024;
            return input.toFixed(decimals(input, 2)) + ' Ki';
        }
        return Math.round(input) + ' ';
    };
});

syncthing.filter('metric', function () {
    return function (input) {
        if (input === undefined) {
            return '0 ';
        }
        if (input > 1000 * 1000 * 1000) {
            input /= 1000 * 1000 * 1000;
            return input.toFixed(decimals(input, 2)) + ' G';
        }
        if (input > 1000 * 1000) {
            input /= 1000 * 1000;
            return input.toFixed(decimals(input, 2)) + ' M';
        }
        if (input > 1000) {
            input /= 1000;
            return input.toFixed(decimals(input, 2)) + ' k';
        }
        return Math.round(input) + ' ';
    };
});

syncthing.filter('short', function () {
    return function (input) {
        return input.substr(0, 6);
    };
});

syncthing.filter('alwaysNumber', function () {
    return function (input) {
        if (input === undefined) {
            return 0;
        }
        return input;
    };
});

syncthing.directive('optionEditor', function () {
    return {
        restrict: 'C',
        replace: true,
        transclude: true,
        scope: {
            setting: '=setting',
        },
        template: '<input type="text" ng-model="config.Options[setting.id]"></input>',
    };
});
