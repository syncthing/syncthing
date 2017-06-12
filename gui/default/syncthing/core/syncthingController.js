angular.module('syncthing.core')
    .config(function($locationProvider) {
        $locationProvider.html5Mode({enabled: true, requireBase: false}).hashPrefix('!');
    })
    .controller('SyncthingController', function ($scope, $http, $location, LocaleService, Events, $filter) {
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
        $scope.discoveryCache = {};
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
        $scope.themes = [];
        $scope.globalChangeEvents = {};
        $scope.metricRates = false;
        $scope.folderPathErrors = {};

        try {
            $scope.metricRates = (window.localStorage["metricRates"] == "true");
        } catch (exception) { }

        $scope.folderDefaults = {
            selectedDevices: {},
            type: "readwrite",
            rescanIntervalS: 60,
            minDiskFree: {value: 1, unit: "%"},
            maxConflicts: 10,
            fsync: true,
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

        $scope.localStateTotal = {
            bytes: 0,
            directories: 0,
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
            refreshDiscoveryCache();
            refreshConfig();
            refreshConnectionStats();
            refreshDeviceStats();
            refreshFolderStats();
            refreshGlobalChanges();
            refreshThemes();

            $http.get(urlbase + '/system/version').success(function (data) {
                if ($scope.version.version && $scope.version.version !== data.version) {
                    // We already have a version response, but it differs from
                    // the new one. Reload the full GUI in case it's changed.
                    document.location.reload(true);
                }

                $scope.version = data;
                $scope.version.isDevelopmentVersion = data.version.indexOf('-')>0;
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

                // If a folder finished scanning, then refresh folder stats
                // to update last scan time.
                if(data.from === 'scanning' && data.to === 'idle') {
                    refreshFolderStats();
                }
            }
        });

        $scope.$on(Events.LOCAL_INDEX_UPDATED, function (event, arg) {
            refreshFolderStats();
            refreshGlobalChanges();
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
                    if ($scope.neededFolder === folder) {
                        refreshNeed(folder);
                    }
                } else if ($scope.neededFolder === folder) {
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
            $scope.completion[data.device][data.folder] = data;
            recalcCompletion(data.device);
        });

        $scope.$on(Events.FOLDER_ERRORS, function (event, arg) {
            var data = arg.data;
            $scope.failed[data.folder] = data.errors;
        });

        $scope.$on(Events.FOLDER_SCAN_PROGRESS, function (event, arg) {
            var data = arg.data;
            $scope.scanProgress[data.folder] = {
                current: data.current,
                total: data.total,
                rate: data.rate
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
            $scope.config.options._listenAddressesStr = $scope.config.options.listenAddresses.join(', ');
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

            // If we're not listening on localhost, and there is no
            // authentication configured, and the magic setting to silence the
            // warning isn't set, then yell at the user.
            var guiCfg = $scope.config.gui;
            $scope.openNoAuth = guiCfg.address.substr(0, 4) !== "127."
                && guiCfg.address.substr(0, 6) !== "[::1]:"
                && (!guiCfg.user || !guiCfg.password)
                && !guiCfg.insecureAdminAccess;

            if (!hasConfig) {
                $scope.$emit('ConfigLoaded');
            }
        }

        function refreshSystem() {
            $http.get(urlbase + '/system/status').success(function (data) {
                $scope.myID = data.myID;
                $scope.system = data;

                var listenersFailed = [];
                for (var address in data.connectionServiceStatus) {
                    if (data.connectionServiceStatus[address].error) {
                        listenersFailed.push(address + ": " + data.connectionServiceStatus[address].error);
                    }
                }
                $scope.listenersFailed = listenersFailed;
                $scope.listenersTotal = Object.keys(data.connectionServiceStatus).length;

                $scope.discoveryTotal = data.discoveryMethods;
                var discoveryFailed = [];
                for (var disco in data.discoveryErrors) {
                    if (data.discoveryErrors[disco]) {
                        discoveryFailed.push(disco + ": " + data.discoveryErrors[disco]);
                    }
                }
                $scope.discoveryFailed = discoveryFailed;
                console.log("refreshSystem", data);
            }).error($scope.emitHTTPError);
        }

        function refreshDiscoveryCache() {
            $http.get(urlbase + '/system/discovery').success(function (data) {
                for (var device in data) {
                    for (var i = 0; i < data[device].addresses.length; i++) {
                        // Relay addresses are URLs with
                        // .../?foo=barlongstuff that we strip away here. We
                        // remove the final slash as well for symmetry with
                        // tcp://192.0.2.42:1234 type addresses.
                        data[device].addresses[i] = data[device].addresses[i].replace(/\/\?.*/, '');
                    }
                }
                $scope.discoveryCache = data;
                console.log("refreshDiscoveryCache", data);
            }).error($scope.emitHTTPError);
        }

        function recalcLocalStateTotal () {
            $scope.localStateTotal = {
                bytes: 0,
                directories: 0,
                files: 0
            };

            for (var f in $scope.model) {
               $scope.localStateTotal.bytes += $scope.model[f].localBytes;
               $scope.localStateTotal.files += $scope.model[f].localFiles;
               $scope.localStateTotal.directories += $scope.model[f].localDirectories;
            }
        }

        function recalcCompletion(device) {
            var total = 0, needed = 0, deletes = 0;
            for (var folder in $scope.completion[device]) {
                if (folder === "_total") {
                    continue;
                }
                total += $scope.completion[device][folder].globalBytes;
                needed += $scope.completion[device][folder].needBytes;
                deletes += $scope.completion[device][folder].needDeletes;
            }
            if (total == 0) {
                $scope.completion[device]._total = 100;
            } else {
                $scope.completion[device]._total = 100 * (1 - needed / total);
            }

            if (needed == 0 && deletes > 0) {
                // We don't need any data, but we have deletes that we need
                // to do. Drop down the completion percentage to indicate
                // that we have stuff to do.
                $scope.completion[device]._total = 95;
            }

            console.log("recalcCompletion", device, $scope.completion[device]);
        }

        function refreshCompletion(device, folder) {
            if (device === $scope.myID) {
                return;
            }

            $http.get(urlbase + '/db/completion?device=' + device + '&folder=' + encodeURIComponent(folder)).success(function (data) {
                if (!$scope.completion[device]) {
                    $scope.completion[device] = {};
                }
                $scope.completion[device][folder] = data;
                recalcCompletion(device);
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
                if ($scope.neededFolder === folder) {
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

                    $scope.folderStats[folder].lastScan = new Date($scope.folderStats[folder].lastScan);
                    $scope.folderStats[folder].lastScanDays = (new Date() - $scope.folderStats[folder].lastScan) / 1000 / 86400;
                }
                console.log("refreshfolderStats", data);
            }).error($scope.emitHTTPError);
        }, 2500);

        var refreshThemes = debounce(function () {
            $http.get("themes.json").success(function (data) { // no urlbase here as this is served by the asset handler
                $scope.themes = data.themes;
            }).error($scope.emitHTTPError);
        }, 2500);

        var refreshGlobalChanges = debounce(function () {
            $http.get(urlbase + "/events/disk?limit=25").success(function (data) {
                data = data.reverse();
                $scope.globalChangeEvents = data;

                console.log("refreshGlobalChanges", data);
            }).error($scope.emitHTTPError);
        }, 2500);

        $scope.refresh = function () {
            refreshSystem();
            refreshDiscoveryCache();
            refreshConnectionStats();
            refreshErrors();
        };

        $scope.folderStatus = function (folderCfg) {
            if (typeof $scope.model[folderCfg.id] === 'undefined') {
                return 'unknown';
            }

            if (folderCfg.paused) {
                return 'paused';
            }

            // after restart syncthing process state may be empty
            if (!$scope.model[folderCfg.id].state) {
                return 'unknown';
            }

            if ($scope.model[folderCfg.id].invalid) {
                return 'stopped';
            }

            var state = '' + $scope.model[folderCfg.id].state;
            if (state === 'error') {
                return 'stopped'; // legacy, the state is called "stopped" in the GUI
            }
            if (state === 'idle' && $scope.neededItems(folderCfg.id) > 0) {
                return 'outofsync';
            }
            if (state === 'scanning') {
                return state;
            }

            if (folderCfg.devices.length <= 1) {
                return 'unshared';
            }

            return state;
        };

        $scope.folderClass = function (folderCfg) {
            var status = $scope.folderStatus(folderCfg);

            if (status === 'idle') {
                return 'success';
            }
            if (status == 'paused') {
                return 'default';
            }
            if (status === 'syncing' || status === 'scanning') {
                return 'primary';
            }
            if (status === 'unknown') {
                return 'info';
            }
            if (status === 'stopped' || status === 'outofsync' || status === 'error') {
                return 'danger';
            }
            if (status === 'unshared') {
                return 'warning';
            }

            return 'info';
        };

        $scope.neededItems = function (folderID) {
            if (!$scope.model[folderID]) {
                return 0
            }

            return $scope.model[folderID].needFiles + $scope.model[folderID].needDirectories +
                $scope.model[folderID].needSymlinks + $scope.model[folderID].needDeletes;
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

        $scope.syncRemaining = function (folder) {
            // Remaining sync bytes
            if (typeof $scope.model[folder] === 'undefined') {
                return 0;
            }
            if ($scope.model[folder].globalBytes === 0) {
                return 0;
            }

            var bytes = $scope.model[folder].globalBytes - $scope.model[folder].inSyncBytes;
            if (isNaN(bytes) || bytes < 0) {
                return 0;
            }
            return bytes;
        };

        $scope.scanPercentage = function (folder) {
            if (!$scope.scanProgress[folder]) {
                return undefined;
            }
            var pct = 100 * $scope.scanProgress[folder].current / $scope.scanProgress[folder].total;
            return Math.floor(pct);
        };

        $scope.scanRate = function (folder) {
            if (!$scope.scanProgress[folder]) {
                return 0;
            }
            return $scope.scanProgress[folder].rate;
        };

        $scope.scanRemaining = function (folder) {
            // Formats the remaining scan time as a string. Includes days and
            // hours only when relevant, resulting in time stamps like:
            // 00m 40s
            // 32m 40s
            // 2h 32m
            // 4d 2h

            if (!$scope.scanProgress[folder]) {
                return "";
            }
            // Calculate remaining bytes and seconds based on our current
            // rate.

            var remainingBytes = $scope.scanProgress[folder].total - $scope.scanProgress[folder].current;
            var seconds = remainingBytes / $scope.scanProgress[folder].rate;
            // Round up to closest ten seconds to avoid flapping too much to
            // and fro.

            seconds = Math.ceil(seconds / 10) * 10;

            // Separate out the number of days.
            var days = 0;
            var res = [];
            if (seconds >= 86400) {
                days = Math.floor(seconds / 86400);
                res.push('' + days + 'd')
                seconds = seconds % 86400;
            }

            // Separate out the number of hours.
            var hours = 0;
            if (seconds > 3600) {
                hours = Math.floor(seconds / 3600);
                res.push('' + hours + 'h')
                seconds = seconds % 3600;
            }

            var d = new Date(1970, 0, 1).setSeconds(seconds);

            if (days === 0) {
                // Format minutes only if we're within a day of completion.
                var f = $filter('date')(d, "m'm'");
                res.push(f);
            }

            if (days === 0 && hours === 0) {
                // Format seconds only when we're within an hour of completion.
                var f = $filter('date')(d, "ss's'");
                res.push(f);
            }

            return res.join(' ');
        };

        $scope.deviceStatus = function (deviceCfg) {
            if ($scope.deviceFolders(deviceCfg).length === 0) {
                return 'unused';
            }

            if (typeof $scope.connections[deviceCfg.deviceID] === 'undefined') {
                return 'unknown';
            }

            if (deviceCfg.paused) {
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

            if (deviceCfg.paused) {
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

        $scope.syncthingStatus = function () {
            var syncCount = 0;
            var notifyCount = 0;
            var pauseCount = 0;

            // loop through all folders
            var folderListCache = $scope.folderList();
            for (var i = 0; i < folderListCache.length; i++) {
                var status = $scope.folderStatus(folderListCache[i]);
                switch (status) {
                    case 'syncing':
                        syncCount++;
                        break;
                    case 'stopped':
                    case 'unknown':
                    case 'outofsync':
                    case 'error':
                        notifyCount++;
                        break;
                }
            }

            // loop through all devices
            var deviceCount = $scope.devices.length;
            for (var i = 0; i < $scope.devices.length; i++) {
                var status = $scope.deviceStatus({
                    deviceID:$scope.devices[i].deviceID
                });
                switch (status) {
                    case 'unknown':
                        notifyCount++;
                        break;
                    case 'paused':
                        pauseCount++;
                        break;
                    case 'unused':
                        deviceCount--;
                        break;
                }
            }

            // enumerate notifications
            if ($scope.openNoAuth || !$scope.configInSync || Object.keys($scope.deviceRejections).length > 0 || Object.keys($scope.folderRejections).length > 0 || $scope.errorList().length > 0 || !online) {
                notifyCount++;
            }

            // at least one folder is syncing
            if (syncCount > 0) {
                return 'sync';
            }

            // a device is unknown or a folder is stopped/unknown/outofsync/error or some other notification is open or gui offline
            if (notifyCount > 0) {
                return 'notify';
            }

            // all used devices are paused except (this) one
            if (pauseCount === deviceCount-1) {
                return 'pause';
            }

            return 'default';
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

        $scope.friendlyNameFromShort = function (shortID) {
            var matches = $scope.devices.filter(function (n) {
                return n.deviceID.substr(0, 7) === shortID;
            });
            if (matches.length !== 1) {
                return shortID;
            }
            return matches[0].name;
        };

        $scope.findDevice = function (deviceID) {
            var matches = $scope.devices.filter(function (n) {
                return n.deviceID === deviceID;
            });
            if (matches.length !== 1) {
                return undefined;
            }
            return matches[0];
        };

        $scope.deviceName = function (deviceCfg) {
            if (typeof deviceCfg === 'undefined' || typeof deviceCfg.deviceID === 'undefined') {
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

        $scope.setDevicePause = function (device, pause) {
            $scope.devices.forEach(function (cfg) {
                if (cfg.deviceID == device) {
                    cfg.paused = pause;
                }
            });
            $scope.config.devices = $scope.devices;
            $scope.saveConfig();
        };

        $scope.setFolderPause = function (folder, pause) {
            var cfg = $scope.folders[folder];
            if (cfg) {
                cfg.paused = pause;
                $scope.config.folders = folderList($scope.folders);
                $scope.saveConfig();
            }
        };

        $scope.showDiscoveryFailures = function () {
            $('#discovery-failures').modal();
        };

        $scope.editSettings = function () {
            // Make a working copy
            $scope.tmpOptions = angular.copy($scope.config.options);
            $scope.tmpOptions.urEnabled = ($scope.tmpOptions.urAccepted > 0);
            $scope.tmpOptions.deviceName = $scope.thisDevice().name;
            $scope.tmpOptions.upgrades = "none";
            if ($scope.tmpOptions.autoUpgradeIntervalH > 0) {
                $scope.tmpOptions.upgrades = "stable";
            }
            if ($scope.tmpOptions.upgradeToPreReleases) {
                $scope.tmpOptions.upgrades = "candidate";
            }
            $scope.tmpGUI = angular.copy($scope.config.gui);
            $('#settings').modal();
        };

        $scope.saveConfig = function (cb) {
            var cfg = JSON.stringify($scope.config);
            var opts = {
                headers: {
                    'Content-Type': 'application/json'
                }
            };
            $http.post(urlbase + '/system/config', cfg, opts).success(function () {
                $http.get(urlbase + '/system/config/insync').success(function (data) {
                    $scope.configInSync = data.configInSync;
                    if (cb) {
                        cb();
                    }
                });
            }).error($scope.emitHTTPError);
        };

        $scope.saveSettings = function () {
            // Make sure something changed
            var changed = !angular.equals($scope.config.options, $scope.tmpOptions) || !angular.equals($scope.config.gui, $scope.tmpGUI);
            var themeChanged = $scope.config.gui.theme !== $scope.tmpGUI.theme;
            if (changed) {
                // Check if auto-upgrade has been enabled or disabled. This
                // also has an effect on usage reporting, so do the check
                // for that later.
                if ($scope.tmpOptions.upgrades == "candidate") {
                    $scope.tmpOptions.autoUpgradeIntervalH = $scope.tmpOptions.autoUpgradeIntervalH || 12;
                    $scope.tmpOptions.upgradeToPreReleases = true;
                    $scope.tmpOptions.urEnabled = true;
                } else if ($scope.tmpOptions.upgrades == "stable") {
                    $scope.tmpOptions.autoUpgradeIntervalH = $scope.tmpOptions.autoUpgradeIntervalH || 12;
                    $scope.tmpOptions.upgradeToPreReleases = false;
                } else {
                    $scope.tmpOptions.autoUpgradeIntervalH = 0;
                }

                // Check if usage reporting has been enabled or disabled
                if ($scope.tmpOptions.urEnabled && $scope.tmpOptions.urAccepted <= 0) {
                    $scope.tmpOptions.urAccepted = 1000;
                } else if (!$scope.tmpOptions.urEnabled && $scope.tmpOptions.urAccepted > 0) {
                    $scope.tmpOptions.urAccepted = -1;
                }

                // Check if protocol will need to be changed on restart
                if ($scope.config.gui.useTLS !== $scope.tmpGUI.useTLS) {
                    $scope.protocolChanged = true;
                }

                // Apply new settings locally
                $scope.thisDevice().name = $scope.tmpOptions.deviceName;
                $scope.config.options = angular.copy($scope.tmpOptions);
                $scope.config.gui = angular.copy($scope.tmpGUI);

                ['listenAddresses', 'globalAnnounceServers'].forEach(function (key) {
                    $scope.config.options[key] = $scope.config.options["_" + key + "Str"].split(/[ ,]+/).map(function (x) {
                        return x.trim();
                    });
                });

                $scope.saveConfig(function () {
                    if (themeChanged) {
                        document.location.reload(true);
                    }
                });
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
            $scope.willBeReintroducedBy = undefined;
             if (deviceCfg.introducedBy) {
                var introducerDevice = $scope.findDevice(deviceCfg.introducedBy);
                if (introducerDevice && introducerDevice.introducer) {
                    $scope.willBeReintroducedBy = $scope.deviceName(introducerDevice);
                }
            }
            $scope.currentDevice._addressesStr = deviceCfg.addresses.join(', ');
            $scope.currentDevice.selectedFolders = {};
            $scope.deviceFolders($scope.currentDevice).forEach(function (folder) {
                $scope.currentDevice.selectedFolders[folder] = true;
            });
            $scope.deviceEditor.$setPristine();
            $('#editDevice').modal();
        };

        $scope.addDevice = function (deviceID, name) {
            return $http.get(urlbase + '/system/discovery')
                .success(function (registry) {
                    $scope.discovery = registry;
                })
                .then(function () {
                    $scope.currentDevice = {
                        name: name,
                        deviceID: deviceID,
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
            $scope.dismissDeviceRejection($scope.currentDevice.deviceID);
        };

        $scope.saveDeviceConfig = function (deviceCfg) {
            deviceCfg.addresses = deviceCfg._addressesStr.split(',').map(function (x) {
                return x.trim();
            });

            var done = false;
            for (var i = 0; i < $scope.devices.length && !done; i++) {
                if ($scope.devices[i].deviceID === deviceCfg.deviceID) {
                    $scope.devices[i] = deviceCfg;
                    done = true;
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
                        if ($scope.folders[id].devices[i].deviceID === deviceCfg.deviceID) {
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
                        return n.deviceID !== deviceCfg.deviceID;
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
            for (var i = 0; i < $scope.devices.length; i++) {
                var n = $scope.devices[i];
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
            if (!$scope.errors) {
                return [];
            }
            return $scope.errors.filter(function (e) {
                return e.when > $scope.seenError;
            });
        };

        $scope.clearErrors = function () {
            $scope.seenError = $scope.errors[$scope.errors.length - 1].when;
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
            if (newvalue && newvalue.trim().charAt(0) === '~') {
                $scope.currentFolder.path = $scope.system.tilde + newvalue.trim().substring(1);
            }
            $http.get(urlbase + '/system/browse', {
                params: { current: newvalue }
            }).success(function (data) {
                $scope.directoryList = data;
            }).error($scope.emitHTTPError);
        });

        $scope.loadFormIntoScope = function (form) {
            console.log('loadFormIntoScope',form.$name);
            switch (form.$name) {
                case 'deviceEditor':
                    $scope.deviceEditor = form;
                    break;
                case 'folderEditor':
                    $scope.folderEditor = form;
                    break;
            }
        };

        $scope.globalChanges = function () {
            $('#globalChanges').modal();
        };

        $scope.editFolderModal = function () {
            $scope.folderPathErrors = {};
            $scope.folderEditor.$setPristine();
            $('#editIgnores textarea').val("");
            $('#editFolder').modal();
        };

        $scope.editFolder = function (folderCfg) {
            $scope.currentFolder = angular.copy(folderCfg);
            if ($scope.currentFolder.path.slice(-1) === $scope.system.pathSeparator) {
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
            $scope.editFolderModal();
        };

        $scope.addFolder = function () {
            $http.get(urlbase + '/svc/random/string?length=10').success(function (data) {
                $scope.currentFolder = angular.copy($scope.folderDefaults);
                $scope.currentFolder.id = (data.random.substr(0, 5) + '-' + data.random.substr(5, 5)).toLowerCase();
                $scope.editingExisting = false;
                $scope.editFolderModal();
            });
        };

        $scope.addFolderAndShare = function (folder, folderLabel, device) {
            $scope.dismissFolderRejection(folder, device);
            $scope.currentFolder = angular.copy($scope.folderDefaults);
            $scope.currentFolder.id = folder;
            $scope.currentFolder.label = folderLabel;
            $scope.currentFolder.viewFlags = {
                importFromOtherDevice: true
            };
            $scope.currentFolder.selectedDevices[device] = true;

            $scope.editingExisting = false;
            $scope.editFolderModal();
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
            $('#editFolder').modal('hide');
            var folderCfg = $scope.currentFolder;
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

            var ignores = $('#editIgnores textarea').val().trim();
            if (!$scope.editingExisting && ignores) {
                folderCfg.paused = true;
            };

            $scope.folders[folderCfg.id] = folderCfg;
            $scope.config.folders = folderList($scope.folders);

            $scope.saveConfig(function () {
                if (!$scope.editingExisting && ignores) {
                    $scope.saveIgnores(function () {
                        $scope.setFolderPause(folderCfg.id, false);
                    });
                }
            });
        };

        $scope.dismissFolderRejection = function (folder, device) {
            delete $scope.folderRejections[folder + "-" + device];
        };

        $scope.ignoreRejectedFolder = function (folder, device) {
            $scope.config.ignoredFolders.push(folder);
            $scope.saveConfig();
            $scope.dismissFolderRejection(folder, device);
        };

        $scope.sharesFolder = function (folderCfg) {
            var names = [];
            folderCfg.devices.forEach(function (device) {
                if (device.deviceID !== $scope.myID) {
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
                    if (devices[i].deviceID === deviceCfg.deviceID) {
                        folders.push(folderID);
                        break;
                    }
                }
            }

            folders.sort(folderCompare);
            return folders;
        };

        $scope.folderLabel = function (folderID) {
            var label = $scope.folders[folderID].label;
            return label.length > 0 ? label : folderID;
        }

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
                    var textArea = $('#editIgnores textarea');
                    textArea.val(data.ignore.join('\n'));
                    $('#editIgnores').modal()
                        .one('shown.bs.modal', function () {
                            textArea.focus();
                        });
                })
                .then(function () {
                    $('#editIgnoresButton').removeAttr('disabled');
                });
        };

        $scope.editIgnoresOnAddingFolder = function () {
            if ($scope.editingExisting) {
                return;
            }

            if ($scope.currentFolder.path.endsWith($scope.system.pathSeparator)) {
                $scope.currentFolder.path = $scope.currentFolder.path.slice(0, -1);
            };
            $('#editIgnores').modal().one('shown.bs.modal', function () {
                textArea.focus();
            });
        };


        $scope.saveIgnores = function (cb) {
            $http.post(urlbase + '/db/ignores?folder=' + encodeURIComponent($scope.currentFolder.id), {
                ignore: $('#editIgnores textarea').val().split('\n')
            }).success(function () {
                if (cb) {
                    cb();
                }
            });
        };

        $scope.setAPIKey = function (cfg) {
            $http.get(urlbase + '/svc/random/string?length=32').success(function (data) {
                cfg.apiKey = data.random;
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
            $scope.failedCurrent = $scope.failed[folder];
            $scope.failedFolderPath = $scope.folders[folder].path;
            if ($scope.failedFolderPath[$scope.failedFolderPath.length - 1] !== $scope.system.pathSeparator) {
                $scope.failedFolderPath += $scope.system.pathSeparator;
            }
            $('#failed').modal().on('hidden.bs.modal', function () {
                $scope.failedCurrent = undefined;
            });
        };

        $scope.hasFailedFiles = function (folder) {
            if (!$scope.failed[folder]) {
                return false;
            }
            if ($scope.failed[folder].length === 0) {
                return false;
            }
            return true;
        };

        $scope.override = function (folder) {
            $http.post(urlbase + "/db/override?folder=" + encodeURIComponent(folder));
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

        $scope.setAllFoldersPause = function(pause) {
            var folderListCache = $scope.folderList();

            for (var i = 0; i < folderListCache.length; i++) {
                folderListCache[i].paused = pause;
            }

            $scope.config.folders = folderList(folderListCache);
            $scope.saveConfig();
        };

        $scope.isAtleastOneFolderPausedStateSetTo = function(pause) {
            var folderListCache = $scope.folderList();

            for (var i = 0; i < folderListCache.length; i++) {
                if (folderListCache[i].paused == pause) {
                    return true;
                }
            }

            return false;
        };

        $scope.bumpFile = function (folder, file) {
            var url = urlbase + "/db/prio?folder=" + encodeURIComponent(folder) + "&file=" + encodeURIComponent(file);
            // In order to get the right view of data in the response.
            url += "&page=" + $scope.neededCurrentPage;
            url += "&perpage=" + $scope.neededPageSize;
            $http.post(url).success(function (data) {
                if ($scope.neededFolder === folder) {
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
                'solaris': 'Solaris'
            }[$scope.version.os] || $scope.version.os;

            var arch ={
                '386': '32 bit',
                'amd64': '64 bit',
                'arm': 'ARM',
                'arm64': 'AArch64',
                'ppc64': 'PowerPC',
                'ppc64le': 'PowerPC (LE)'
            }[$scope.version.arch] || $scope.version.arch;

            return $scope.version.version + ', ' + os + ' (' + arch + ')';
        };

        $scope.inputTypeFor = function (key, value) {
            if (key.substr(0, 1) === '_') {
                return 'skip';
            }
            if (value === null) {
                return 'null';
            }
            if (typeof value === 'number') {
                return 'number';
            }
            if (typeof value === 'boolean') {
                return 'checkbox';
            }
            if (value instanceof Array) {
                return 'list';
            }
            if (typeof value === 'object') {
                return 'skip';
            }
            return 'text';
        };

        $scope.themeName = function (theme) {
            return theme.replace('-', ' ').replace(/(?:^|\s)\S/g, function (a) {
                return a.toUpperCase();
            });
        };

        $scope.modalLoaded = function () {
            // once all modal elements have been processed
            if ($('modal').length === 0) {

                // pseudo main. called on all definitions assigned
                initController();
            }
        }

        $scope.toggleUnits = function () {
            $scope.metricRates = !$scope.metricRates;
            try {
                window.localStorage["metricRates"] = $scope.metricRates;
            } catch (exception) { }
        }
    });
