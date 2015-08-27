angular.module('syncthing.core')
    .config(function($locationProvider) {
        $locationProvider.html5Mode(true).hashPrefix('!');
    })
    .controller('SyncthingController', function ($scope, $http, $location, LocaleService, Events) {
        'use strict';

        // private/helper definitions

        var prevDate = 0;
        var navigatingAway = false;
        var online = false;
        var restarting = false;

        function initController() {
            LocaleService.autoConfigLocale();
            setInterval($scope.refresh, 10000);
            Events.start();
        }

        // public/scope definitions

        $scope.completion = {};
        $scope.config = {};
        $scope.configInSync = true;
        $scope.connections = {};
        $scope.errors = [];
        $scope.model = {};
        $scope.myID = '';
        $scope.devices = [];
        $scope.deviceRejections = {};
        $scope.folderRejections = {};
        $scope.protocolChanged = false;
        $scope.reportData = {};
        $scope.reportPreview = false;
        $scope.folders = {};
        $scope.seenError = '';
        $scope.upgradeInfo = null;
        $scope.deviceStats = {};
        $scope.folderStats = {};
        $scope.progress = {};
        $scope.version = {};
        $scope.needed = [];
        $scope.neededTotal = 0;
        $scope.neededCurrentPage = 1;
        $scope.neededPageSize = 10;
        $scope.failed = {};
        $scope.failedCurrentPage = 1;
        $scope.failedCurrentFolder = undefined;
        $scope.failedPageSize = 10;
        $scope.scanProgress = {};

        $scope.localStateTotal = {
            bytes: 0,
            files: 0
        };

        $(window).bind('beforeunload', function () {
            navigatingAway = true;
        });

        $scope.$on("$locationChangeSuccess", function () {
            LocaleService.useLocale($location.search().lang);
        });

        $scope.needActions = {
            'rm': 'Del',
            'rmdir': 'Del (dir)',
            'sync': 'Sync',
            'touch': 'Update'
        };
        $scope.needIcons = {
            'rm': 'trash-o',
            'rmdir': 'trash-o',
            'sync': 'arrow-circle-o-down',
            'touch': 'asterisk'
        };

        $scope.$on(Events.ONLINE, function () {
            if (online && !restarting) {
                return;
            }

            console.log('UIOnline');

            refreshSystem();
            refreshConfig();
            refreshConnectionStats();
            refreshDeviceStats();
            refreshFolderStats();

            $http.get(urlbase + '/system/version').success(function (data) {
                if ($scope.version.version && $scope.version.version != data.version) {
                    // We already have a version response, but it differs from
                    // the new one. Reload the full GUI in case it's changed.
                    document.location.reload(true);
                }

                $scope.version = data;
            }).error($scope.emitHTTPError);

            $http.get(urlbase + '/svc/report').success(function (data) {
                $scope.reportData = data;
            }).error($scope.emitHTTPError);

            $http.get(urlbase + '/system/upgrade').success(function (data) {
                $scope.upgradeInfo = data;
            }).error(function () {
                $scope.upgradeInfo = null;
            });

            online = true;
            restarting = false;
            $('#networkError').modal('hide');
            $('#restarting').modal('hide');
            $('#shutdown').modal('hide');
        });

        $scope.$on(Events.OFFLINE, function () {
            if (navigatingAway || !online) {
                return;
            }

            console.log('UIOffline');
            online = false;
            if (!restarting) {
                $('#networkError').modal();
            }
        });

        $scope.$on('HTTPError', function (event, arg) {
            // Emitted when a HTTP call fails. We use the status code to try
            // to figure out what's wrong.

            if (navigatingAway || !online) {
                return;
            }

            console.log('HTTPError', arg);
            online = false;
            if (!restarting) {
                if (arg.status === 0) {
                    // A network error, not an HTTP error
                    $scope.$emit(Events.OFFLINE);
                } else if (arg.status >= 400 && arg.status <= 599) {
                    // A genuine HTTP error
                    $('#networkError').modal('hide');
                    $('#restarting').modal('hide');
                    $('#shutdown').modal('hide');
                    $('#httpError').modal();
                }
            }
        });

        $scope.$on(Events.STATE_CHANGED, function (event, arg) {
            var data = arg.data;
            if ($scope.model[data.folder]) {
                $scope.model[data.folder].state = data.to;
                $scope.model[data.folder].error = data.error;

                // If a folder has started syncing, then any old list of
                // errors is obsolete. We may get a new list of errors very
                // shortly though.
                if (data.to === 'syncing') {
                    $scope.failed[data.folder] = [];
                }

                // If a folder has started scanning, then any scan progress is
                // also obsolete.
                if (data.to === 'scanning') {
                    delete $scope.scanProgress[data.folder];
                }
            }
        });

        $scope.$on(Events.LOCAL_INDEX_UPDATED, function (event, arg) {
            refreshFolderStats();
        });

        $scope.$on(Events.DEVICE_DISCONNECTED, function (event, arg) {
            $scope.connections[arg.data.id].connected = false;
            refreshDeviceStats();
        });

        $scope.$on(Events.DEVICE_CONNECTED, function (event, arg) {
            if (!$scope.connections[arg.data.id]) {
                $scope.connections[arg.data.id] = {
                    inbps: 0,
                    outbps: 0,
                    inBytesTotal: 0,
                    outBytesTotal: 0,
                    type: arg.data.type,
                    address: arg.data.addr
                };
                $scope.completion[arg.data.id] = {
                    _total: 100
                };
            }
        });

        $scope.$on('ConfigLoaded', function () {
            if ($scope.config.options.urAccepted === 0) {
                // If usage reporting has been neither accepted nor declined,
                // we want to ask the user to make a choice. But we don't want
                // to bug them during initial setup, so we set a cookie with
                // the time of the first visit. When that cookie is present
                // and the time is more than four hours ago, we ask the
                // question.

                var firstVisit = document.cookie.replace(/(?:(?:^|.*;\s*)firstVisit\s*\=\s*([^;]*).*$)|^.*$/, "$1");
                if (!firstVisit) {
                    document.cookie = "firstVisit=" + Date.now() + ";max-age=" + 30 * 24 * 3600;
                } else {
                    if (+firstVisit < Date.now() - 4 * 3600 * 1000) {
                        $('#ur').modal();
                    }
                }
            }
        });

        $scope.$on(Events.DEVICE_REJECTED, function (event, arg) {
            $scope.deviceRejections[arg.data.device] = arg;
        });

        $scope.$on(Events.DEVICE_PAUSED, function (event, arg) {
            $scope.connections[arg.data.device].paused = true;
        });

        $scope.$on(Events.DEVICE_RESUMED, function (event, arg) {
            $scope.connections[arg.data.device].paused = false;
        });

        $scope.$on(Events.FOLDER_REJECTED, function (event, arg) {
            $scope.folderRejections[arg.data.folder + "-" + arg.data.device] = arg;
        });

        $scope.$on(Events.CONFIG_SAVED, function (event, arg) {
            updateLocalConfig(arg.data);

            $http.get(urlbase + '/system/config/insync').success(function (data) {
                $scope.configInSync = data.configInSync;
            }).error($scope.emitHTTPError);
        });

        $scope.$on(Events.DOWNLOAD_PROGRESS, function (event, arg) {
            var stats = arg.data;
            var progress = {};
            for (var folder in stats) {
                progress[folder] = {};
                for (var file in stats[folder]) {
                    var s = stats[folder][file];
                    var reused = 100 * s.reused / s.total;
                    var copiedFromOrigin = 100 * s.copiedFromOrigin / s.total;
                    var copiedFromElsewhere = 100 * s.copiedFromElsewhere / s.total;
                    var pulled = 100 * s.pulled / s.total;
                    var pulling = 100 * s.pulling / s.total;
                    // We try to round up pulling to at least a percent so that it would be at least a bit visible.
                    if (pulling < 1 && pulled + copiedFromElsewhere + copiedFromOrigin + reused <= 99) {
                        pulling = 1;
                    }
                    progress[folder][file] = {
                        reused: reused,
                        copiedFromOrigin: copiedFromOrigin,
                        copiedFromElsewhere: copiedFromElsewhere,
                        pulled: pulled,
                        pulling: pulling,
                        bytesTotal: s.bytesTotal,
                        bytesDone: s.bytesDone,
                    };
                }
            }
            for (var folder in $scope.progress) {
                if (!(folder in progress)) {
                    if ($scope.neededFolder == folder) {
                        refreshNeed(folder);
                    }
                } else if ($scope.neededFolder == folder) {
                    for (file in $scope.progress[folder]) {
                        if (!(file in progress[folder])) {
                            refreshNeed(folder);
                            break;
                        }
                    }
                }
            }
            $scope.progress = progress;
            console.log("DownloadProgress", $scope.progress);
        });

        $scope.$on(Events.FOLDER_SUMMARY, function (event, arg) {
            var data = arg.data;
            $scope.model[data.folder] = data.summary;
            recalcLocalStateTotal();
        });

        $scope.$on(Events.FOLDER_COMPLETION, function (event, arg) {
            var data = arg.data;
            if (!$scope.completion[data.device]) {
                $scope.completion[data.device] = {};
            }
            $scope.completion[data.device][data.folder] = data.completion;

            var tot = 0,
                cnt = 0;
            for (var cmp in $scope.completion[data.device]) {
                if (cmp === "_total") {
                    continue;
                }
                tot += $scope.completion[data.device][cmp];
                cnt += 1;
            }
            $scope.completion[data.device]._total = tot / cnt;
        });

        $scope.$on(Events.FOLDER_ERRORS, function (event, arg) {
            var data = arg.data;
            $scope.failed[data.folder] = data.errors;
        });

        $scope.$on(Events.FOLDER_SCAN_PROGRESS, function (event, arg) {
            var data = arg.data;
            $scope.scanProgress[data.folder] = {
                current: data.current,
                total: data.total
            };
            console.log("FolderScanProgress", data);
        });

        $scope.emitHTTPError = function (data, status, headers, config) {
            $scope.$emit('HTTPError', {data: data, status: status, headers: headers, config: config});
        };

        var debouncedFuncs = {};

        function refreshFolder(folder) {
            var key = "refreshFolder" + folder;
            if (!debouncedFuncs[key]) {
                debouncedFuncs[key] = debounce(function () {
                    $http.get(urlbase + '/db/status?folder=' + encodeURIComponent(folder)).success(function (data) {
                        $scope.model[folder] = data;
                        recalcLocalStateTotal();
                        console.log("refreshFolder", folder, data);
                    }).error($scope.emitHTTPError);
                }, 1000, true);
            }
            debouncedFuncs[key]();
        }

        function updateLocalConfig(config) {
            var hasConfig = !isEmptyObject($scope.config);

            $scope.config = config;
            $scope.config.options._listenAddressStr = $scope.config.options.listenAddress.join(', ');
            $scope.config.options._globalAnnounceServersStr = $scope.config.options.globalAnnounceServers.join(', ');

            $scope.devices = $scope.config.devices;
            $scope.devices.forEach(function (deviceCfg) {
                $scope.completion[deviceCfg.deviceID] = {
                    _total: 100
                };
            });
            $scope.devices.sort(deviceCompare);
            $scope.folders = folderMap($scope.config.folders);
            Object.keys($scope.folders).forEach(function (folder) {
                refreshFolder(folder);
                $scope.folders[folder].devices.forEach(function (deviceCfg) {
                    refreshCompletion(deviceCfg.deviceID, folder);
                });
            });

            if (!hasConfig) {
                $scope.$emit('ConfigLoaded');
            }
        }

        function refreshSystem() {
            $http.get(urlbase + '/system/status').success(function (data) {
                $scope.myID = data.myID;
                $scope.system = data;

                $scope.announceServersTotal = data.extAnnounceOK ? Object.keys(data.extAnnounceOK).length : 0;
                var failedAnnounce = [];
                for (var server in data.extAnnounceOK) {
                    if (!data.extAnnounceOK[server]) {
                        failedAnnounce.push(server);
                    }
                }
                $scope.announceServersFailed = failedAnnounce;

                $scope.relayClientsTotal = data.relayClientStatus ? Object.keys(data.relayClientStatus).length : 0;
                var failedRelays = [];
                for (var relay in data.relayClientStatus) {
                    if (!data.relayClientStatus[relay]) {
                        failedRelays.push(relay);
                    }
                }
                $scope.relayClientsFailed = failedRelays;


                console.log("refreshSystem", data);
            }).error($scope.emitHTTPError);
        }

        function recalcLocalStateTotal () {
            $scope.localStateTotal = {
                bytes: 0,
                files: 0
            };

            for (var f in $scope.model) {
               $scope.localStateTotal.bytes += $scope.model[f].localBytes;
               $scope.localStateTotal.files += $scope.model[f].localFiles;
            }
        }

        function refreshCompletion(device, folder) {
            if (device === $scope.myID) {
                return;
            }

            $http.get(urlbase + '/db/completion?device=' + device + '&folder=' + encodeURIComponent(folder)).success(function (data) {
                if (!$scope.completion[device]) {
                    $scope.completion[device] = {};
                }
                $scope.completion[device][folder] = data.completion;

                var tot = 0,
                    cnt = 0;
                for (var cmp in $scope.completion[device]) {
                    if (cmp === "_total") {
                        continue;
                    }
                    tot += $scope.completion[device][cmp];
                    cnt += 1;
                }
                $scope.completion[device]._total = tot / cnt;

                console.log("refreshCompletion", device, folder, $scope.completion[device]);
            }).error($scope.emitHTTPError);
        }

        function refreshConnectionStats() {
            $http.get(urlbase + '/system/connections').success(function (data) {
                var now = Date.now(),
                    td = (now - prevDate) / 1000,
                    id;

                prevDate = now;

                try {
                    data.total.inbps = Math.max(0, (data.total.inBytesTotal - $scope.connectionsTotal.inBytesTotal) / td);
                    data.total.outbps = Math.max(0, (data.total.outBytesTotal - $scope.connectionsTotal.outBytesTotal) / td);
                } catch (e) {
                    data.total.inbps = 0;
                    data.total.outbps = 0;
                }
                $scope.connectionsTotal = data.total;

                data = data.connections;
                for (id in data) {
                    if (!data.hasOwnProperty(id)) {
                        continue;
                    }
                    try {
                        data[id].inbps = Math.max(0, (data[id].inBytesTotal - $scope.connections[id].inBytesTotal) / td);
                        data[id].outbps = Math.max(0, (data[id].outBytesTotal - $scope.connections[id].outBytesTotal) / td);
                    } catch (e) {
                        data[id].inbps = 0;
                        data[id].outbps = 0;
                    }
                }
                $scope.connections = data;
                console.log("refreshConnections", data);
            }).error($scope.emitHTTPError);
        }

        function refreshErrors() {
            $http.get(urlbase + '/system/error').success(function (data) {
                $scope.errors = data.errors;
                console.log("refreshErrors", data);
            }).error($scope.emitHTTPError);
        }

        function refreshConfig() {
            $http.get(urlbase + '/system/config').success(function (data) {
                updateLocalConfig(data);
                console.log("refreshConfig", data);
            }).error($scope.emitHTTPError);

            $http.get(urlbase + '/system/config/insync').success(function (data) {
                $scope.configInSync = data.configInSync;
            }).error($scope.emitHTTPError);
        }

        function refreshNeed(folder) {
            var url = urlbase + "/db/need?folder=" + encodeURIComponent(folder);
            url += "&page=" + $scope.neededCurrentPage;
            url += "&perpage=" + $scope.neededPageSize;
            $http.get(url).success(function (data) {
                if ($scope.neededFolder == folder) {
                    console.log("refreshNeed", folder, data);
                    parseNeeded(data);
                }
            }).error($scope.emitHTTPError);
        }

        function needAction(file) {
            var fDelete = 4096;
            var fDirectory = 16384;

            if ((file.flags & (fDelete + fDirectory)) === fDelete + fDirectory) {
                return 'rmdir';
            } else if ((file.flags & fDelete) === fDelete) {
                return 'rm';
            } else if ((file.flags & fDirectory) === fDirectory) {
                return 'touch';
            } else {
                return 'sync';
            }
        }

        function parseNeeded(data) {
            var merged = [];
            data.progress.forEach(function (item) {
                item.type = "progress";
                item.action = needAction(item);
                merged.push(item);
            });
            data.queued.forEach(function (item) {
                item.type = "queued";
                item.action = needAction(item);
                merged.push(item);
            });
            data.rest.forEach(function (item) {
                item.type = "rest";
                item.action = needAction(item);
                merged.push(item);
            });
            $scope.needed = merged;
            $scope.neededTotal = data.total;
        }

        $scope.neededPageChanged = function (page) {
            $scope.neededCurrentPage = page;
            refreshNeed($scope.neededFolder);
        };

        $scope.neededChangePageSize = function (perpage) {
            $scope.neededPageSize = perpage;
            refreshNeed($scope.neededFolder);
        };

        $scope.failedPageChanged = function (page) {
            $scope.failedCurrentPage = page;
        };

        $scope.failedChangePageSize = function (perpage) {
            $scope.failedPageSize = perpage;
        };

        var refreshDeviceStats = debounce(function () {
            $http.get(urlbase + "/stats/device").success(function (data) {
                $scope.deviceStats = data;
                for (var device in $scope.deviceStats) {
                    $scope.deviceStats[device].lastSeen = new Date($scope.deviceStats[device].lastSeen);
                    $scope.deviceStats[device].lastSeenDays = (new Date() - $scope.deviceStats[device].lastSeen) / 1000 / 86400;
                }
                console.log("refreshDeviceStats", data);
            }).error($scope.emitHTTPError);
        }, 2500);

        var refreshFolderStats = debounce(function () {
            $http.get(urlbase + "/stats/folder").success(function (data) {
                $scope.folderStats = data;
                for (var folder in $scope.folderStats) {
                    if ($scope.folderStats[folder].lastFile) {
                        $scope.folderStats[folder].lastFile.at = new Date($scope.folderStats[folder].lastFile.at);
                    }
                }
                console.log("refreshfolderStats", data);
            }).error($scope.emitHTTPError);
        }, 2500);

        $scope.refresh = function () {
            refreshSystem();
            refreshConnectionStats();
            refreshErrors();
        };

        $scope.folderStatus = function (folderCfg) {
            if (typeof $scope.model[folderCfg.id] === 'undefined') {
                return 'unknown';
            }

            // after restart syncthing process state may be empty
            if (!$scope.model[folderCfg.id].state) {
                return 'unknown';
            }

            if (folderCfg.devices.length <= 1) {
                return 'unshared';
            }

            if ($scope.model[folderCfg.id].invalid) {
                return 'stopped';
            }

            var state = '' + $scope.model[folderCfg.id].state;
            if (state === 'error') {
                return 'stopped'; // legacy, the state is called "stopped" in the GUI
            }
            if (state === 'idle' && $scope.model[folderCfg.id].needFiles > 0) {
                return 'outofsync';
            }

            return state;
        };

        $scope.folderClass = function (folderCfg) {
            var status = $scope.folderStatus(folderCfg);

            if (status == 'idle') {
                return 'success';
            }
            if (status == 'syncing' || status == 'scanning') {
                return 'primary';
            }
            if (status === 'unknown') {
                return 'info';
            }
            if (status === 'unshared') {
                return 'warning';
            }
            if (status === 'stopped' || status === 'outofsync' || status === 'error') {
                return 'danger';
            }

            return 'info';
        };

        $scope.syncPercentage = function (folder) {
            if (typeof $scope.model[folder] === 'undefined') {
                return 100;
            }
            if ($scope.model[folder].globalBytes === 0) {
                return 100;
            }

            var pct = 100 * $scope.model[folder].inSyncBytes / $scope.model[folder].globalBytes;
            return Math.floor(pct);
        };

        $scope.scanPercentage = function (folder) {
            if (!$scope.scanProgress[folder]) {
                return undefined;
            }
            var pct = 100 * $scope.scanProgress[folder].current / $scope.scanProgress[folder].total;
            return Math.floor(pct);
        }

        $scope.deviceStatus = function (deviceCfg) {
            if ($scope.deviceFolders(deviceCfg).length === 0) {
                return 'unused';
            }

            if (typeof $scope.connections[deviceCfg.deviceID] === 'undefined') {
                return 'unknown';
            }

            if ($scope.connections[deviceCfg.deviceID].paused) {
                return 'paused';
            }

            if ($scope.connections[deviceCfg.deviceID].connected) {
                if ($scope.completion[deviceCfg.deviceID] && $scope.completion[deviceCfg.deviceID]._total === 100) {
                    return 'insync';
                } else {
                    return 'syncing';
                }
            }

            // Disconnected
            return 'disconnected';
        };

        $scope.deviceClass = function (deviceCfg) {
            if ($scope.deviceFolders(deviceCfg).length === 0) {
                // Unused
                return 'warning';
            }

            if (typeof $scope.connections[deviceCfg.deviceID] === 'undefined') {
                return 'info';
            }

            if ($scope.connections[deviceCfg.deviceID].paused) {
                return 'default';
            }

            if ($scope.connections[deviceCfg.deviceID].connected) {
                if ($scope.completion[deviceCfg.deviceID] && $scope.completion[deviceCfg.deviceID]._total === 100) {
                    return 'success';
                } else {
                    return 'primary';
                }
            }

            // Disconnected
            return 'info';
        };

        $scope.deviceAddr = function (deviceCfg) {
            var conn = $scope.connections[deviceCfg.deviceID];
            if (conn && conn.connected) {
                return conn.address;
            }
            return '?';
        };

        $scope.deviceCompletion = function (deviceCfg) {
            var conn = $scope.connections[deviceCfg.deviceID];
            if (conn) {
                return conn.completion + '%';
            }
            return '';
        };

        $scope.findDevice = function (deviceID) {
            var matches = $scope.devices.filter(function (n) {
                return n.deviceID == deviceID;
            });
            if (matches.length != 1) {
                return undefined;
            }
            return matches[0];
        };

        $scope.deviceName = function (deviceCfg) {
            if (typeof deviceCfg === 'undefined') {
                return "";
            }
            if (deviceCfg.name) {
                return deviceCfg.name;
            }
            return deviceCfg.deviceID.substr(0, 6);
        };

        $scope.thisDeviceName = function () {
            var device = $scope.thisDevice();
            if (typeof device === 'undefined') {
                return "(unknown device)";
            }
            if (device.name) {
                return device.name;
            }
            return device.deviceID.substr(0, 6);
        };

        $scope.pauseDevice = function (device) {
            $http.post(urlbase + "/system/pause?device=" + device);
        };

        $scope.resumeDevice = function (device) {
            $http.post(urlbase + "/system/resume?device=" + device);
        };

        $scope.editSettings = function () {
            // Make a working copy
            $scope.tmpOptions = angular.copy($scope.config.options);
            $scope.tmpOptions.urEnabled = ($scope.tmpOptions.urAccepted > 0);
            $scope.tmpOptions.deviceName = $scope.thisDevice().name;
            $scope.tmpOptions.autoUpgradeEnabled = ($scope.tmpOptions.autoUpgradeIntervalH > 0);
            $scope.tmpGUI = angular.copy($scope.config.gui);
            $('#settings').modal();
        };

        $scope.saveConfig = function () {
            var cfg = JSON.stringify($scope.config);
            var opts = {
                headers: {
                    'Content-Type': 'application/json'
                }
            };
            $http.post(urlbase + '/system/config', cfg, opts).success(function () {
                $http.get(urlbase + '/system/config/insync').success(function (data) {
                    $scope.configInSync = data.configInSync;
                });
            }).error($scope.emitHTTPError);
        };

        $scope.saveSettings = function () {
            // Make sure something changed
            var changed = !angular.equals($scope.config.options, $scope.tmpOptions) || !angular.equals($scope.config.gui, $scope.tmpGUI);
            if (changed) {
                // Check if usage reporting has been enabled or disabled
                if ($scope.tmpOptions.urEnabled && $scope.tmpOptions.urAccepted <= 0) {
                    $scope.tmpOptions.urAccepted = 1000;
                } else if (!$scope.tmpOptions.urEnabled && $scope.tmpOptions.urAccepted > 0) {
                    $scope.tmpOptions.urAccepted = -1;
                }

                // Check if auto-upgrade has been enabled or disabled
                if ($scope.tmpOptions.autoUpgradeEnabled) {
                    $scope.tmpOptions.autoUpgradeIntervalH = $scope.tmpOptions.autoUpgradeIntervalH || 12;
                } else {
                    $scope.tmpOptions.autoUpgradeIntervalH = 0;
                }

                // Check if protocol will need to be changed on restart
                if ($scope.config.gui.useTLS !== $scope.tmpGUI.useTLS) {
                    $scope.protocolChanged = true;
                }

                // Apply new settings locally
                $scope.thisDevice().name = $scope.tmpOptions.deviceName;
                $scope.config.options = angular.copy($scope.tmpOptions);
                $scope.config.gui = angular.copy($scope.tmpGUI);

                ['listenAddress', 'globalAnnounceServers'].forEach(function (key) {
                    $scope.config.options[key] = $scope.config.options["_" + key + "Str"].split(/[ ,]+/).map(function (x) {
                        return x.trim();
                    });
                });

                $scope.saveConfig();
            }

            $('#settings').modal("hide");
        };

        $scope.saveAdvanced = function () {
            $scope.config = $scope.advancedConfig;
            $scope.saveConfig();
            $('#advanced').modal("hide");
        };

        $scope.restart = function () {
            restarting = true;
            $('#restarting').modal();
            $http.post(urlbase + '/system/restart');
            $scope.configInSync = true;

            // Switch webpage protocol if needed
            if ($scope.protocolChanged) {
                var protocol = 'http';

                if ($scope.config.gui.useTLS) {
                    protocol = 'https';
                }

                setTimeout(function () {
                    window.location.protocol = protocol;
                }, 2500);

                $scope.protocolChanged = false;
            }
        };

        $scope.upgrade = function () {
            restarting = true;
            $('#majorUpgrade').modal('hide');
            $('#upgrading').modal();
            $http.post(urlbase + '/system/upgrade').success(function () {
                $('#restarting').modal();
                $('#upgrading').modal('hide');
            }).error(function () {
                $('#upgrading').modal('hide');
            });
        };

        $scope.upgradeMajor = function () {
            $('#majorUpgrade').modal();
        };

        $scope.shutdown = function () {
            restarting = true;
            $http.post(urlbase + '/system/shutdown').success(function () {
                $('#shutdown').modal();
            }).error($scope.emitHTTPError);
            $scope.configInSync = true;
        };

        $scope.editDevice = function (deviceCfg) {
            $scope.currentDevice = $.extend({}, deviceCfg);
            $scope.editingExisting = true;
            $scope.currentDevice._addressesStr = deviceCfg.addresses.join(', ');
            $scope.currentDevice.selectedFolders = {};
            $scope.deviceFolders($scope.currentDevice).forEach(function (folder) {
                $scope.currentDevice.selectedFolders[folder] = true;
            });
            $scope.deviceEditor.$setPristine();
            $('#editDevice').modal();
        };

        $scope.idDevice = function () {
            $('#idqr').modal('show');
        };

        $scope.addDevice = function () {
            $http.get(urlbase + '/system/discovery')
                .success(function (registry) {
                    $scope.discovery = registry;
                })
                .then(function () {
                    $scope.currentDevice = {
                        _addressesStr: 'dynamic',
                        compression: 'metadata',
                        introducer: false,
                        selectedFolders: {}
                    };
                    $scope.editingExisting = false;
                    $scope.deviceEditor.$setPristine();
                    $('#editDevice').modal();
                });
        };

        $scope.deleteDevice = function () {
            $('#editDevice').modal('hide');
            if (!$scope.editingExisting) {
                return;
            }

            $scope.devices = $scope.devices.filter(function (n) {
                return n.deviceID !== $scope.currentDevice.deviceID;
            });
            $scope.config.devices = $scope.devices;
            // In case we later added the device manually, remove the ignoral
            // record.
            $scope.config.ignoredDevices = $scope.config.ignoredDevices.filter(function (id) {
                return id !== $scope.currentDevice.deviceID;
            });

            for (var id in $scope.folders) {
                $scope.folders[id].devices = $scope.folders[id].devices.filter(function (n) {
                    return n.deviceID !== $scope.currentDevice.deviceID;
                });
            }

            $scope.saveConfig();
        };

        $scope.saveDevice = function () {
            $('#editDevice').modal('hide');
            $scope.saveDeviceConfig($scope.currentDevice);
        };

        $scope.addNewDeviceID = function (device) {
            var deviceCfg = {
                deviceID: device,
                _addressesStr: 'dynamic',
                compression: 'metadata',
                introducer: false,
                selectedFolders: {}
            };
            $scope.saveDeviceConfig(deviceCfg);
            $scope.dismissDeviceRejection(device);
        };

        $scope.saveDeviceConfig = function (deviceCfg) {
            var done, i;
            deviceCfg.addresses = deviceCfg._addressesStr.split(',').map(function (x) {
                return x.trim();
            });

            done = false;
            for (i = 0; i < $scope.devices.length; i++) {
                if ($scope.devices[i].deviceID === deviceCfg.deviceID) {
                    $scope.devices[i] = deviceCfg;
                    done = true;
                    break;
                }
            }

            if (!done) {
                $scope.devices.push(deviceCfg);
            }

            $scope.devices.sort(deviceCompare);
            $scope.config.devices = $scope.devices;
            // In case we are adding the device manually, remove the ignoral
            // record.
            $scope.config.ignoredDevices = $scope.config.ignoredDevices.filter(function (id) {
                return id !== deviceCfg.deviceID;
            });

            for (var id in deviceCfg.selectedFolders) {
                if (deviceCfg.selectedFolders[id]) {
                    var found = false;
                    for (i = 0; i < $scope.folders[id].devices.length; i++) {
                        if ($scope.folders[id].devices[i].deviceID == deviceCfg.deviceID) {
                            found = true;
                            break;
                        }
                    }

                    if (!found) {
                        $scope.folders[id].devices.push({
                            deviceID: deviceCfg.deviceID
                        });
                    }
                } else {
                    $scope.folders[id].devices = $scope.folders[id].devices.filter(function (n) {
                        return n.deviceID != deviceCfg.deviceID;
                    });
                }
            }

            $scope.saveConfig();
        };

        $scope.dismissDeviceRejection = function (device) {
            delete $scope.deviceRejections[device];
        };

        $scope.ignoreRejectedDevice = function (device) {
            $scope.config.ignoredDevices.push(device);
            $scope.saveConfig();
            $scope.dismissDeviceRejection(device);
        };

        $scope.otherDevices = function () {
            return $scope.devices.filter(function (n) {
                return n.deviceID !== $scope.myID;
            });
        };

        $scope.thisDevice = function () {
            var i, n;

            for (i = 0; i < $scope.devices.length; i++) {
                n = $scope.devices[i];
                if (n.deviceID === $scope.myID) {
                    return n;
                }
            }
        };

        $scope.allDevices = function () {
            var devices = $scope.otherDevices();
            devices.push($scope.thisDevice());
            return devices;
        };

        $scope.errorList = function () {
            return $scope.errors.filter(function (e) {
                return e.time > $scope.seenError;
            });
        };

        $scope.clearErrors = function () {
            $scope.seenError = $scope.errors[$scope.errors.length - 1].time;
            $http.post(urlbase + '/system/error/clear');
        };

        $scope.friendlyDevices = function (str) {
            for (var i = 0; i < $scope.devices.length; i++) {
                var cfg = $scope.devices[i];
                str = str.replace(cfg.deviceID, $scope.deviceName(cfg));
            }
            return str;
        };

        $scope.folderList = function () {
            return folderList($scope.folders);
        };

        $scope.directoryList = [];

        $scope.$watch('currentFolder.path', function (newvalue) {
            if (newvalue && newvalue.trim().charAt(0) == '~') {
                $scope.currentFolder.path = $scope.system.tilde + newvalue.trim().substring(1);
            }
            $http.get(urlbase + '/system/browse', {
                params: { current: newvalue }
            }).success(function (data) {
                $scope.directoryList = data;
            }).error($scope.emitHTTPError);
        });

        $scope.editFolder = function (folderCfg) {
            $scope.currentFolder = angular.copy(folderCfg);
            if ($scope.currentFolder.path.slice(-1) == $scope.system.pathSeparator) {
                $scope.currentFolder.path = $scope.currentFolder.path.slice(0, -1);
            }
            $scope.currentFolder.selectedDevices = {};
            $scope.currentFolder.devices.forEach(function (n) {
                $scope.currentFolder.selectedDevices[n.deviceID] = true;
            });
            if ($scope.currentFolder.versioning && $scope.currentFolder.versioning.type === "trashcan") {
                $scope.currentFolder.trashcanFileVersioning = true;
                $scope.currentFolder.fileVersioningSelector = "trashcan";
                $scope.currentFolder.trashcanClean = +$scope.currentFolder.versioning.params.cleanoutDays;
            } else if ($scope.currentFolder.versioning && $scope.currentFolder.versioning.type === "simple") {
                $scope.currentFolder.simpleFileVersioning = true;
                $scope.currentFolder.fileVersioningSelector = "simple";
                $scope.currentFolder.simpleKeep = +$scope.currentFolder.versioning.params.keep;
            } else if ($scope.currentFolder.versioning && $scope.currentFolder.versioning.type === "staggered") {
                $scope.currentFolder.staggeredFileVersioning = true;
                $scope.currentFolder.fileVersioningSelector = "staggered";
                $scope.currentFolder.staggeredMaxAge = Math.floor(+$scope.currentFolder.versioning.params.maxAge / 86400);
                $scope.currentFolder.staggeredCleanInterval = +$scope.currentFolder.versioning.params.cleanInterval;
                $scope.currentFolder.staggeredVersionsPath = $scope.currentFolder.versioning.params.versionsPath;
            } else if ($scope.currentFolder.versioning && $scope.currentFolder.versioning.type === "external") {
                $scope.currentFolder.externalFileVersioning = true;
                $scope.currentFolder.fileVersioningSelector = "external";
                $scope.currentFolder.externalCommand = $scope.currentFolder.versioning.params.command;
            } else {
                $scope.currentFolder.fileVersioningSelector = "none";
            }
            $scope.currentFolder.trashcanClean = $scope.currentFolder.trashcanClean || 0; // weeds out nulls and undefineds
            $scope.currentFolder.simpleKeep = $scope.currentFolder.simpleKeep || 5;
            $scope.currentFolder.staggeredCleanInterval = $scope.currentFolder.staggeredCleanInterval || 3600;
            $scope.currentFolder.staggeredVersionsPath = $scope.currentFolder.staggeredVersionsPath || "";

            // staggeredMaxAge can validly be zero, which we should not replace
            // with the default value of 365. So only set the default if it's
            // actually undefined.
            if (typeof $scope.currentFolder.staggeredMaxAge === 'undefined') {
                $scope.currentFolder.staggeredMaxAge = 365;
            }
            $scope.currentFolder.externalCommand = $scope.currentFolder.externalCommand || "";

            $scope.editingExisting = true;
            $scope.folderEditor.$setPristine();
            $('#editFolder').modal();
        };

        $scope.addFolder = function () {
            $scope.currentFolder = {
                selectedDevices: {}
            };
            $scope.currentFolder.rescanIntervalS = 60;
            $scope.currentFolder.minDiskFreePct = 1;
            $scope.currentFolder.order = "random";
            $scope.currentFolder.fileVersioningSelector = "none";
            $scope.currentFolder.trashcanClean = 0;
            $scope.currentFolder.simpleKeep = 5;
            $scope.currentFolder.staggeredMaxAge = 365;
            $scope.currentFolder.staggeredCleanInterval = 3600;
            $scope.currentFolder.staggeredVersionsPath = "";
            $scope.currentFolder.externalCommand = "";
            $scope.currentFolder.autoNormalize = true;
            $scope.editingExisting = false;
            $scope.folderEditor.$setPristine();
            $('#editFolder').modal();
        };

        $scope.addFolderAndShare = function (folder, device) {
            $scope.dismissFolderRejection(folder, device);
            $scope.currentFolder = {
                id: folder,
                selectedDevices: {},
                rescanIntervalS: 60,
                minDiskFreePct: 1,
                order: "random",
                fileVersioningSelector: "none",
                trashcanClean: 0,
                simpleKeep: 5,
                staggeredMaxAge: 365,
                staggeredCleanInterval: 3600,
                staggeredVersionsPath: "",
                externalCommand: "",
                autoNormalize: true
            };
            $scope.currentFolder.selectedDevices[device] = true;

            $scope.editingExisting = false;
            $scope.folderEditor.$setPristine();
            $('#editFolder').modal();
        };

        $scope.shareFolderWithDevice = function (folder, device) {
            $scope.folders[folder].devices.push({
                deviceID: device
            });
            $scope.config.folders = folderList($scope.folders);
            $scope.saveConfig();
            $scope.dismissFolderRejection(folder, device);
        };

        $scope.saveFolder = function () {
            var folderCfg, done, i;

            $('#editFolder').modal('hide');
            folderCfg = $scope.currentFolder;
            folderCfg.devices = [];
            folderCfg.selectedDevices[$scope.myID] = true;
            for (var deviceID in folderCfg.selectedDevices) {
                if (folderCfg.selectedDevices[deviceID] === true) {
                    folderCfg.devices.push({
                        deviceID: deviceID
                    });
                }
            }
            delete folderCfg.selectedDevices;

            if (folderCfg.fileVersioningSelector === "trashcan") {
                folderCfg.versioning = {
                    'Type': 'trashcan',
                    'Params': {
                        'cleanoutDays': '' + folderCfg.trashcanClean
                    }
                };
                delete folderCfg.trashcanFileVersioning;
                delete folderCfg.trashcanClean;
            } else if (folderCfg.fileVersioningSelector === "simple") {
                folderCfg.versioning = {
                    'Type': 'simple',
                    'Params': {
                        'keep': '' + folderCfg.simpleKeep
                    }
                };
                delete folderCfg.simpleFileVersioning;
                delete folderCfg.simpleKeep;
            } else if (folderCfg.fileVersioningSelector === "staggered") {
                folderCfg.versioning = {
                    'type': 'staggered',
                    'params': {
                        'maxAge': '' + (folderCfg.staggeredMaxAge * 86400),
                        'cleanInterval': '' + folderCfg.staggeredCleanInterval,
                        'versionsPath': '' + folderCfg.staggeredVersionsPath
                    }
                };
                delete folderCfg.staggeredFileVersioning;
                delete folderCfg.staggeredMaxAge;
                delete folderCfg.staggeredCleanInterval;
                delete folderCfg.staggeredVersionsPath;

            } else if (folderCfg.fileVersioningSelector === "external") {
                folderCfg.versioning = {
                    'Type': 'external',
                    'Params': {
                        'command': '' + folderCfg.externalCommand
                    }
                };
                delete folderCfg.externalFileVersioning;
                delete folderCfg.externalCommand;
            } else {
                delete folderCfg.versioning;
            }

            $scope.folders[folderCfg.id] = folderCfg;
            $scope.config.folders = folderList($scope.folders);

            $scope.saveConfig();
        };

        $scope.dismissFolderRejection = function (folder, device) {
            delete $scope.folderRejections[folder + "-" + device];
        };

        $scope.sharesFolder = function (folderCfg) {
            var names = [];
            folderCfg.devices.forEach(function (device) {
                if (device.deviceID != $scope.myID) {
                    names.push($scope.deviceName($scope.findDevice(device.deviceID)));
                }
            });
            names.sort();
            return names.join(", ");
        };

        $scope.deviceFolders = function (deviceCfg) {
            var folders = [];
            for (var folderID in $scope.folders) {
                var devices = $scope.folders[folderID].devices;
                for (var i = 0; i < devices.length; i++) {
                    if (devices[i].deviceID == deviceCfg.deviceID) {
                        folders.push(folderID);
                        break;
                    }
                }
            }

            folders.sort();
            return folders;
        };

        $scope.deleteFolder = function (id) {
            $('#editFolder').modal('hide');
            if (!$scope.editingExisting) {
                return;
            }

            delete $scope.folders[id];
            delete $scope.model[id];
            $scope.config.folders = folderList($scope.folders);
            recalcLocalStateTotal();

            $scope.saveConfig();
        };

        $scope.editIgnores = function () {
            if (!$scope.editingExisting) {
                return;
            }

            $('#editIgnoresButton').attr('disabled', 'disabled');
            $http.get(urlbase + '/db/ignores?folder=' + encodeURIComponent($scope.currentFolder.id))
                .success(function (data) {
                    data.ignore = data.ignore || [];

                    $('#editFolder').modal('hide')
                        .one('hidden.bs.modal', function() {
                            var textArea = $('#editIgnores textarea');

                            textArea.val(data.ignore.join('\n'));

                            $('#editIgnores').modal()
                                .one('hidden.bs.modal', function () {
                                    $('#editFolder').modal();
                                })
                                .one('shown.bs.modal', function () {
                                    textArea.focus();
                                });
                        });
                })
                .then(function () {
                    $('#editIgnoresButton').removeAttr('disabled');
                });
        };

        $scope.saveIgnores = function () {
            if (!$scope.editingExisting) {
                return;
            }

            $http.post(urlbase + '/db/ignores?folder=' + encodeURIComponent($scope.currentFolder.id), {
                ignore: $('#editIgnores textarea').val().split('\n')
            });
        };

        $scope.setAPIKey = function (cfg) {
            cfg.apiKey = randomString(32);
        };

        $scope.showURPreview = function () {
            $('#settings').modal('hide');
            $('#urPreview').modal().on('hidden.bs.modal', function () {
                $('#settings').modal();
            });
        };

        $scope.acceptUR = function () {
            $scope.config.options.urAccepted = 1000; // Larger than the largest existing report version
            $scope.saveConfig();
            $('#ur').modal('hide');
        };

        $scope.declineUR = function () {
            $scope.config.options.urAccepted = -1;
            $scope.saveConfig();
            $('#ur').modal('hide');
        };

        $scope.showNeed = function (folder) {
            $scope.neededFolder = folder;
            refreshNeed(folder);
            $('#needed').modal().on('hidden.bs.modal', function () {
                $scope.neededFolder = undefined;
                $scope.needed = undefined;
                $scope.neededTotal = 0;
                $scope.neededCurrentPage = 1;
            });
        };

        $scope.showFailed = function (folder) {
            $scope.failedCurrent = $scope.failed[folder]
            $('#failed').modal().on('hidden.bs.modal', function () {
                $scope.failedCurrent = undefined;
            });
        };

        $scope.hasFailedFiles = function (folder) {
            if (!$scope.failed[folder]) {
                return false;
            }
            if ($scope.failed[folder].length == 0) {
                return false;
            }
            return true
        };

        $scope.override = function (folder) {
            $http.post(urlbase + "/db/override?folder=" + encodeURIComponent(folder));
        };

        $scope.about = function () {
            $('#about').modal('show');
        };

        $scope.advanced = function () {
            $scope.advancedConfig = angular.copy($scope.config);
            $('#advanced').modal('show');
        };

        $scope.showReportPreview = function () {
            $scope.reportPreview = true;
        };

        $scope.rescanAllFolders = function () {
            $http.post(urlbase + "/db/scan");
        };

        $scope.rescanFolder = function (folder) {
            $http.post(urlbase + "/db/scan?folder=" + encodeURIComponent(folder));
        };

        $scope.bumpFile = function (folder, file) {
            var url = urlbase + "/db/prio?folder=" + encodeURIComponent(folder) + "&file=" + encodeURIComponent(file);
            // In order to get the right view of data in the response.
            url += "&page=" + $scope.neededCurrentPage;
            url += "&perpage=" + $scope.neededPageSize;
            $http.post(url).success(function (data) {
                if ($scope.neededFolder == folder) {
                    console.log("bumpFile", folder, data);
                    parseNeeded(data);
                }
            }).error($scope.emitHTTPError);
        };

        $scope.versionString = function () {
            if (!$scope.version.version) {
                return '';
            }

            var os = {
                'darwin': 'Mac OS X',
                'dragonfly': 'DragonFly BSD',
                'freebsd': 'FreeBSD',
                'openbsd': 'OpenBSD',
                'netbsd': 'NetBSD',
                'linux': 'Linux',
                'windows': 'Windows',
                'solaris': 'Solaris',
            }[$scope.version.os] || $scope.version.os;

            var arch ={
                '386': '32 bit',
                'amd64': '64 bit',
                'arm': 'ARM',
            }[$scope.version.arch] || $scope.version.arch;

            return $scope.version.version + ', ' + os + ' (' + arch + ')';
        };

        $scope.inputTypeFor = function (key, value) {
            if (key.substr(0, 1) === '_') {
                return 'skip';
            }
            if (typeof value === 'number') {
                return 'number';
            }
            if (typeof value === 'boolean') {
                return 'checkbox';
            }
            if (typeof value === 'object') {
                return 'skip';
            }
            return 'text';
        };

        // pseudo main. called on all definitions assigned
        initController();
    });
