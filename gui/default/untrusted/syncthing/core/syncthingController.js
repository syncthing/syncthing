angular.module('syncthing.core')
    .config(function ($locationProvider) {
        $locationProvider.html5Mode({ enabled: true, requireBase: false }).hashPrefix('!');
    })
    .controller('SyncthingController', function ($scope, $http, $location, LocaleService, Events, $filter, $q, $compile, $timeout, $rootScope, $translate) {
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
        $scope.devices = {};
        $scope.discoveryCache = {};
        $scope.protocolChanged = false;
        $scope.reportData = {};
        $scope.reportDataPreview = '';
        $scope.reportPreview = false;
        $scope.folders = {};
        $scope.seenError = '';
        $scope.upgradeInfo = null;
        $scope.deviceStats = {};
        $scope.folderStats = {};
        $scope.pendingDevices = {};
        $scope.pendingFolders = {};
        $scope.progress = {};
        $scope.version = {};
        $scope.needed = {}
        $scope.neededFolder = '';
        $scope.failed = {};
        $scope.localChanged = {};
        $scope.scanProgress = {};
        $scope.themes = [];
        $scope.globalChangeEvents = {};
        $scope.metricRates = false;
        $scope.folderPathErrors = {};
        $scope.currentFolder = {};
        $scope.currentDevice = {};
        $scope.ignores = {
            text: '',
            error: null,
            disabled: false,
        };
        resetRemoteNeed();

        try {
            $scope.metricRates = (window.localStorage["metricRates"] == "true");
        } catch (exception) { }

        $scope.versioningDefaults = {
            selector: "none",
            trashcanClean: 0,
            versioningCleanupIntervalS: 3600,
            simpleKeep: 5,
            staggeredMaxAge: 365,
            staggeredCleanInterval: 3600,
            staggeredVersionsPath: "",
            externalCommand: "",
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
            'rm': 'far fa-fw fa-trash-alt',
            'rmdir': 'far fa-fw fa-trash-alt',
            'sync': 'far fa-fw fa-arrow-alt-circle-down',
            'touch': 'fas fa-fw fa-asterisk'
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
                console.log("version", data);
                if ($scope.version.version && $scope.version.version !== data.version) {
                    // We already have a version response, but it differs from
                    // the new one. Reload the full GUI in case it's changed.
                    document.location.reload(true);
                }

                $scope.version = data;
            }).error($scope.emitHTTPError);

            $http.get(urlbase + '/svc/report').success(function (data) {
                $scope.reportData = data;
                if ($scope.system && $scope.config.options.urAccepted > -1 && $scope.config.options.urSeen < $scope.system.urVersionMax && $scope.config.options.urAccepted < $scope.system.urVersionMax) {
                    // Usage reporting format has changed, prompt the user to re-accept.
                    $('#ur').modal();
                }
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

                // If a folder has started scanning, then any scan progress is
                // also obsolete.
                if (data.to === 'scanning') {
                    delete $scope.scanProgress[data.folder];
                }

                // If a folder finished scanning, then refresh folder stats
                // to update last scan time.
                if (data.from === 'scanning' && data.to === 'idle') {
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
                    _total: 100,
                    _needBytes: 0,
                    _needItems: 0
                };
            }
        });

        $scope.$on(Events.DEVICE_REJECTED, function (event, arg) {
            var pendingDevice = {
                time: arg.time,
                name: arg.data.name,
                address: arg.data.address
            };
            console.log("rejected device:", arg.data.device, pendingDevice);

            $scope.pendingDevices[arg.data.device] = pendingDevice;
        });

        $scope.$on(Events.FOLDER_REJECTED, function (event, arg) {
            var offeringDevice = {
                time: arg.time,
                label: arg.data.folderLabel
            };
            console.log("rejected folder", arg.data.folder, "from device:", arg.data.device, offeringDevice);

            var pendingFolder = $scope.pendingFolders[arg.data.folder];
            if (pendingFolder === undefined) {
                pendingFolder = {
                    offeredBy: {}
                };
            }
            pendingFolder.offeredBy[arg.data.device] = offeringDevice;
            $scope.pendingFolders[arg.data.folder] = pendingFolder;
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

        $scope.$on(Events.CONFIG_SAVED, function (event, arg) {
            updateLocalConfig(arg.data);

            $http.get(urlbase + '/config/insync').success(function (data) {
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
                        $scope.refreshNeed($scope.needed.page, $scope.needed.perpage);
                    }
                } else if ($scope.neededFolder === folder) {
                    for (file in $scope.progress[folder]) {
                        if (!(file in progress[folder])) {
                            $scope.refreshNeed($scope.needed.page, $scope.needed.perpage);
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
            $scope.model[arg.data.folder].errors = arg.data.errors.length;
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
            $scope.$emit('HTTPError', { data: data, status: status, headers: headers, config: config });
        };

        var debouncedFuncs = {};

        function refreshFolder(folder) {
            if ($scope.folders[folder].paused) {
                return;
            }
            var key = "refreshFolder" + folder;
            if (!debouncedFuncs[key]) {
                debouncedFuncs[key] = debounce(function () {
                    $http.get(urlbase + '/db/status?folder=' + encodeURIComponent(folder)).success(function (data) {
                        $scope.model[folder] = data;
                        recalcLocalStateTotal();
                        console.log("refreshFolder", folder, data);
                    }).error($scope.emitHTTPError);
                }, 1000);
            }
            debouncedFuncs[key]();
        }

        function updateLocalConfig(config) {
            var hasConfig = !isEmptyObject($scope.config);

            $scope.config = config;
            $scope.config.options._listenAddressesStr = $scope.config.options.listenAddresses.join(', ');
            $scope.config.options._globalAnnounceServersStr = $scope.config.options.globalAnnounceServers.join(', ');
            $scope.config.options._urAcceptedStr = "" + $scope.config.options.urAccepted;

            $scope.devices = deviceMap($scope.config.devices);
            for (var id in $scope.devices) {
                $scope.completion[id] = {
                    _total: 100,
                    _needBytes: 0,
                    _needItems: 0
                };
            };
            $scope.folders = folderMap($scope.config.folders);
            Object.keys($scope.folders).forEach(function (folder) {
                refreshFolder(folder);
                $scope.folders[folder].devices.forEach(function (deviceCfg) {
                    refreshCompletion(deviceCfg.deviceID, folder);
                });
            });

            refreshCluster();
            refreshNoAuthWarning();
            setDefaultTheme();

            if (!hasConfig) {
                $scope.$emit('ConfigLoaded');
            }
        }

        function refreshSystem() {
            $http.get(urlbase + '/system/status').success(function (data) {
                $scope.myID = data.myID;
                $scope.system = data;

                if ($scope.reportDataPreviewVersion === '') {
                    $scope.reportDataPreviewVersion = $scope.system.urVersionMax;
                }

                var listenersFailed = [];
                for (var address in data.connectionServiceStatus) {
                    if (data.connectionServiceStatus[address].error) {
                        listenersFailed.push(address + ": " + data.connectionServiceStatus[address].error);
                    }
                }
                $scope.listenersFailed = listenersFailed;
                $scope.listenersTotal = $scope.sizeOf(data.connectionServiceStatus);

                $scope.discoveryTotal = data.discoveryMethods;
                var discoveryFailed = [];
                for (var disco in data.discoveryErrors) {
                    if (data.discoveryErrors[disco]) {
                        discoveryFailed.push(disco + ": " + data.discoveryErrors[disco]);
                    }
                }
                $scope.discoveryFailed = discoveryFailed;

                refreshNoAuthWarning();

                console.log("refreshSystem", data);
            }).error($scope.emitHTTPError);
        }

        function refreshNoAuthWarning() {
            if (!$scope.system || !$scope.config || !$scope.config.gui) {
                // We need all to be able to determine the state.
                return
            }

            // If we're not listening on localhost, and there is no
            // authentication configured, and the magic setting to silence the
            // warning isn't set, then yell at the user.
            var addr = $scope.system.guiAddressUsed;
            var guiCfg = $scope.config.gui;
            $scope.openNoAuth = addr.substr(0, 4) !== "127."
                && addr.substr(0, 6) !== "[::1]:"
                && addr.substr(0, 1) !== "/"
                && (!guiCfg.user || !guiCfg.password)
                && guiCfg.authMode !== 'ldap'
                && !guiCfg.insecureAdminAccess;

            if (guiCfg.user && guiCfg.password) {
                $scope.dismissNotification('authenticationUserAndPassword');
            }
        }

        function refreshCluster() {
            $http.get(urlbase + '/cluster/pending/devices').success(function (data) {
                $scope.pendingDevices = data;
                console.log("refreshCluster devices", data);
            }).error($scope.emitHTTPError);
            $http.get(urlbase + '/cluster/pending/folders').success(function (data) {
                $scope.pendingFolders = data;
                console.log("refreshCluster folders", data);
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

        function recalcLocalStateTotal() {
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
            var total = 0, needed = 0, deletes = 0, items = 0;
            for (var folder in $scope.completion[device]) {
                if (folder === "_total" || folder === '_needBytes' || folder === '_needItems') {
                    continue;
                }
                total += $scope.completion[device][folder].globalBytes;
                needed += $scope.completion[device][folder].needBytes;
                items += $scope.completion[device][folder].needItems;
                deletes += $scope.completion[device][folder].needDeletes;
            }
            if (total == 0) {
                $scope.completion[device]._total = 100;
                $scope.completion[device]._needBytes = 0;
                $scope.completion[device]._needItems = 0;
            } else {
                $scope.completion[device]._total = Math.floor(100 * (1 - needed / total));
                $scope.completion[device]._needBytes = needed;
                $scope.completion[device]._needItems = items + deletes;
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
            $http.get(urlbase + '/config').success(function (data) {
                updateLocalConfig(data);
                console.log("refreshConfig", data);
            }).error($scope.emitHTTPError);

            $http.get(urlbase + '/config/insync').success(function (data) {
                $scope.configInSync = data.configInSync;
            }).error($scope.emitHTTPError);
        }

        $scope.refreshNeed = function (page, perpage) {
            if (!$scope.neededFolder) {
                return;
            }
            var url = urlbase + "/db/need?folder=" + encodeURIComponent($scope.neededFolder);
            url += "&page=" + page;
            url += "&perpage=" + perpage;
            $http.get(url).success(function (data) {
                console.log("refreshNeed", $scope.neededFolder, data);
                parseNeeded(data);
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
            $scope.needed = data;
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
            $scope.needed.items = merged;
        }

        function pathJoin(base, name) {
            base = expandTilde(base);
            if (base[base.length - 1] !== $scope.system.pathSeparator) {
                return base + $scope.system.pathSeparator + name;
            }
            return base + name;
        }

        function expandTilde(path) {
            if (path && path.trim().charAt(0) === '~') {
                return $scope.system.tilde + path.trim().substring(1);
            }
            return path;
        }

        function shouldSetDefaultFolderPath() {
            return $scope.config.defaults.folder.path && !$scope.editingExisting && $scope.folderEditor.folderPath.$pristine && !$scope.editingDefaults;
        }

        function resetRemoteNeed() {
            $scope.remoteNeed = {};
            $scope.remoteNeedFolders = [];
            $scope.remoteNeedDevice = undefined;
        }


        function setDefaultTheme() {
            if (!document.getElementById("fallback-theme-css")) {

                // check if no support for prefers-color-scheme
                var colorSchemeNotSupported = typeof window.matchMedia === "undefined" || window.matchMedia('(prefers-color-scheme: dark)').media === 'not all';

                if ($scope.config.gui.theme === "default" && colorSchemeNotSupported) {
                    document.documentElement.style.display = 'none';
                    document.head.insertAdjacentHTML(
                        'beforeend',
                        '<link id="fallback-theme-css" rel="stylesheet" href="theme-assets/light/assets/css/theme.css" onload="document.documentElement.style.display = \'\'">'
                    );
                }
            }
        }

        function saveIgnores(ignores, cb) {
            $http.post(urlbase + '/db/ignores?folder=' + encodeURIComponent($scope.currentFolder.id), {
                ignore: ignores
            }).success(function () {
                if (cb) {
                    cb();
                }
            });
        };

        function initShareEditing(editing) {
            $scope.currentSharing = {};
            $scope.currentSharing.editing = editing;
            $scope.currentSharing.shared = [];
            $scope.currentSharing.unrelated = [];
            $scope.currentSharing.selected = {};
            $scope.currentSharing.encryptionPasswords = {};
            if (editing === 'folder') {
                initShareEditingFolder();
            }
        };

        function initShareEditingFolder() {
            $scope.currentFolder.devices.forEach(function (n) {
                if (n.deviceID !== $scope.myID) {
                    $scope.currentSharing.shared.push($scope.devices[n.deviceID]);
                }
                if (n.encryptionPassword !== '') {
                    $scope.currentSharing.encryptionPasswords[n.deviceID] = n.encryptionPassword;
                }
                $scope.currentSharing.selected[n.deviceID] = true;
            });
            $scope.currentSharing.unrelated = $scope.deviceList().filter(function (n) {
                return n.deviceID !== $scope.myID && !$scope.currentSharing.selected[n.deviceID];
            });
        }

        $scope.refreshFailed = function (page, perpage) {
            if (!$scope.failed || !$scope.failed.folder) {
                return;
            }
            var url = urlbase + '/folder/errors?folder=' + encodeURIComponent($scope.failed.folder);
            url += "&page=" + page + "&perpage=" + perpage;
            $http.get(url).success(function (data) {
                $scope.failed = data;
            }).error($scope.emitHTTPError);
        };

        $scope.refreshRemoteNeed = function (folder, page, perpage) {
            if (!$scope.remoteNeedDevice) {
                return;
            }
            var url = urlbase + '/db/remoteneed?device=' + $scope.remoteNeedDevice.deviceID;
            url += '&folder=' + encodeURIComponent(folder);
            url += "&page=" + page + "&perpage=" + perpage;
            $http.get(url).success(function (data) {
                $scope.remoteNeed[folder] = data;
            }).error(function (err) {
                $scope.remoteNeed[folder] = undefined;
                $scope.emitHTTPError(err);
            });
        };

        $scope.refreshLocalChanged = function (page, perpage) {
            if (!$scope.localChangedFolder) {
                return;
            }
            var url = urlbase + '/db/localchanged?folder=';
            url += encodeURIComponent($scope.localChangedFolder);
            url += "&page=" + page + "&perpage=" + perpage;
            $http.get(url).success(function (data) {
                $scope.localChanged = data;
            }).error($scope.emitHTTPError);
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
                if (!data) {
                    // For reasons unknown this is called with data being the empty
                    // string on shutdown, causing an error on .reverse().
                    return;
                }
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
            if (folderCfg.paused) {
                return 'paused';
            }

            var folderInfo = $scope.model[folderCfg.id];

            // after restart syncthing process state may be empty
            if (typeof folderInfo === 'undefined' || !folderInfo.state) {
                return 'unknown';
            }

            var state = '' + folderInfo.state;
            if (state === 'error') {
                return 'stopped'; // legacy, the state is called "stopped" in the GUI
            }

            if (state !== 'idle') {
                return state;
            }

            if (folderInfo.needTotalItems > 0) {
                return 'outofsync';
            }
            if ($scope.hasFailedFiles(folderCfg.id)) {
                return 'faileditems';
            }
            if ($scope.hasReceiveOnlyChanged(folderCfg)) {
                return 'localadditions';
            }
            if ($scope.hasReceiveEncryptedItems(folderCfg)) {
                return 'localunencrypted';
            }
            if (folderCfg.devices.length <= 1) {
                return 'unshared';
            }

            return state;
        };

        $scope.folderClass = function (folderCfg) {
            var status = $scope.folderStatus(folderCfg);

            if (status === 'idle' || status === 'localadditions') {
                return 'success';
            }
            if (status == 'paused') {
                return 'default';
            }
            if (status === 'syncing' || status === 'sync-preparing' || status === 'scanning' || status === 'cleaning') {
                return 'primary';
            }
            if (status === 'unknown') {
                return 'info';
            }
            if (status === 'stopped' || status === 'outofsync' || status === 'error' || status === 'faileditems' || status === 'localunencrypted') {
                return 'danger';
            }
            if (status === 'unshared' || status === 'scan-waiting' || status === 'sync-waiting' || status === 'clean-waiting') {
                return 'warning';
            }

            return 'info';
        };

        $scope.syncPercentage = function (folder) {
            if (typeof $scope.model[folder] === 'undefined') {
                return 100;
            }
            if ($scope.model[folder].needTotalItems === 0) {
                return 100;
            }
            if (($scope.model[folder].needBytes == 0 && $scope.model[folder].needDeletes > 0) || $scope.model[folder].globalBytes == 0) {
                // We don't need any data, but we have deletes that we need
                // to do. Drop down the completion percentage to indicate
                // that we have stuff to do.
                // Do the same thing in case we only have zero byte files to sync.
                return 95;
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
            // In case remaining scan time appears to be >31d, omit the
            // details, i.e.:
            // > 1 month

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
                if (days > 31) {
                    return '> 1 month';
                }
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
            var status = '';

            if ($scope.deviceFolders(deviceCfg).length === 0) {
                status = 'unused-';
            }

            if (typeof $scope.connections[deviceCfg.deviceID] === 'undefined') {
                return 'unknown';
            }

            if (deviceCfg.paused) {
                return status + 'paused';
            }

            if ($scope.connections[deviceCfg.deviceID].connected) {
                if ($scope.completion[deviceCfg.deviceID] && $scope.completion[deviceCfg.deviceID]._total === 100) {
                    return status + 'insync';
                } else {
                    return 'syncing';
                }
            }

            // Disconnected
            return status + 'disconnected';
        };

        $scope.deviceClass = function (deviceCfg) {
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
                    case 'sync-preparing':
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
            var deviceCount = 0;
            for (var id in $scope.devices) {
                var status = $scope.deviceStatus({
                    deviceID: id
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
                deviceCount++;
            }

            // enumerate notifications
            if ($scope.openNoAuth || !$scope.configInSync || $scope.errorList().length > 0 || !online || Object.keys($scope.pendingDevices).length > 0 || Object.keys($scope.pendingFolders).length > 0) {
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
            if (pauseCount === deviceCount - 1) {
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

        $scope.hasRemoteGUIAddress = function (deviceCfg) {
            if (!deviceCfg.remoteGUIPort)
                return false;
            var conn = $scope.connections[deviceCfg.deviceID];
            return conn && conn.connected && conn.address && conn.type.indexOf('Relay') == -1;
        };

        $scope.remoteGUIAddress = function (deviceCfg) {
            // Assume hasRemoteGUIAddress is true or we would not be here
            var conn = $scope.connections[deviceCfg.deviceID];
            return 'http://' + replaceAddressPort(conn.address, deviceCfg.remoteGUIPort);
        };

        function replaceAddressPort(address, newPort) {
            for (var index = address.length - 1; index >= 0; index--) {
                if (address[index] === ":") {
                    return address.substr(0, index) + ":" + newPort.toString();
                }
            }
            return address;
        }

        $scope.friendlyNameFromShort = function (shortID) {
            var matches = Object.keys($scope.devices).filter(function (id) {
                return id.substr(0, 7) === shortID;
            });
            if (matches.length !== 1) {
                return shortID;
            }
            return $scope.friendlyNameFromID(matches[0]);
        };

        $scope.friendlyNameFromID = function (deviceID) {
            var match = $scope.devices[deviceID];
            if (match) {
                return $scope.deviceName(match);
            }
            return deviceID.substr(0, 6);
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
            $scope.devices[device].paused = pause;
            $scope.config.devices = $scope.deviceList();
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

        $scope.logging = {
            facilities: {},
            refreshFacilities: function () {
                $http.get(urlbase + '/system/debug').success(function (data) {
                    var facilities = {};
                    data.enabled = data.enabled || [];
                    $.each(data.facilities, function (key, value) {
                        facilities[key] = {
                            description: value,
                            enabled: data.enabled.indexOf(key) > -1
                        }
                    })
                    $scope.logging.facilities = facilities;
                }).error($scope.emitHTTPError);
            },
            show: function () {
                $scope.logging.paused = false;
                $scope.logging.refreshFacilities();
                $scope.logging.timer = $timeout($scope.logging.fetch);
                var textArea = $('#logViewerText');
                textArea.on("scroll", $scope.logging.onScroll);
                $('#logViewer').modal().one('shown.bs.modal', function () {
                    // Scroll to bottom.
                    textArea.scrollTop(textArea[0].scrollHeight);
                }).one('hidden.bs.modal', function () {
                    $timeout.cancel($scope.logging.timer);
                    textArea.off("scroll", $scope.logging.onScroll);
                    $scope.logging.timer = null;
                    $scope.logging.entries = [];
                });
            },
            onFacilityChange: function (facility) {
                var enabled = $scope.logging.facilities[facility].enabled;
                // Disable checkboxes while we're in flight.
                $.each($scope.logging.facilities, function (key) {
                    $scope.logging.facilities[key].enabled = null;
                })
                $http.post(urlbase + '/system/debug?' + (enabled ? 'enable=' : 'disable=') + facility)
                    .success($scope.logging.refreshFacilities)
                    .error($scope.emitHTTPError);
            },
            onScroll: function () {
                var textArea = $('#logViewerText');
                var scrollTop = textArea.prop('scrollTop');
                var scrollHeight = textArea.prop('scrollHeight');
                $scope.logging.paused = scrollHeight > (scrollTop + textArea.outerHeight());
                // Browser events do not cause redraw, trigger manually.
                $scope.$apply();
            },
            timer: null,
            entries: [],
            paused: false,
            content: function () {
                var content = "";
                $.each($scope.logging.entries, function (idx, entry) {
                    content += entry.when.split('.')[0].replace('T', ' ') + ' ' + entry.message + "\n";
                });
                return content;
            },
            fetch: function () {
                var textArea = $('#logViewerText');
                if ($scope.logging.paused) {
                    if (!$scope.logging.timer) return;
                    $scope.logging.timer = $timeout($scope.logging.fetch, 500);
                    return;
                }

                var last = null;
                if ($scope.logging.entries.length > 0) {
                    last = $scope.logging.entries[$scope.logging.entries.length - 1].when;
                }

                $http.get(urlbase + '/system/log' + (last ? '?since=' + encodeURIComponent(last) : '')).success(function (data) {
                    if (!$scope.logging.timer) return;
                    $scope.logging.timer = $timeout($scope.logging.fetch, 2000);
                    if (!$scope.logging.paused) {
                        if (data.messages) {
                            $scope.logging.entries.push.apply($scope.logging.entries, data.messages);
                            // Wait for the text area to be redrawn, adding new lines, and then scroll to bottom.
                            $timeout(function () {
                                textArea.scrollTop(textArea[0].scrollHeight);
                            });
                        }
                    }
                });
            }
        };

        $scope.discardChangedSettings = function () {
            $("#discard-changes-confirmation").modal("hide");
            $("#settings").off("hide.bs.modal").modal("hide");
        };

        $scope.showSettings = function () {
            // Make a working copy
            $scope.tmpOptions = angular.copy($scope.config.options);
            $scope.tmpOptions.deviceName = $scope.thisDevice().name;
            $scope.tmpOptions.upgrades = "none";
            if ($scope.tmpOptions.autoUpgradeIntervalH > 0) {
                $scope.tmpOptions.upgrades = "stable";
            }
            if ($scope.tmpOptions.upgradeToPreReleases) {
                $scope.tmpOptions.upgrades = "candidate";
            }
            $scope.tmpGUI = angular.copy($scope.config.gui);
            $scope.tmpRemoteIgnoredDevices = angular.copy($scope.config.remoteIgnoredDevices);
            $scope.tmpDevices = angular.copy($scope.config.devices);
            $('#settings').modal("show");
            $("#settings a[href='#settings-general']").tab("show");
            $("#settings").on('hide.bs.modal', function (event) {
                if ($scope.settingsModified()) {
                    event.preventDefault();
                    $("#discard-changes-confirmation").modal("show");
                } else {
                    $("#settings").off("hide.bs.modal");
                }
            });
        };

        $scope.saveConfig = function (callback) {
            var cfg = JSON.stringify($scope.config);
            var opts = {
                headers: {
                    'Content-Type': 'application/json'
                }
            };
            $http.put(urlbase + '/config', cfg, opts).success(function () {
                refreshConfig();

                if (callback) {
                    callback();
                }
            }).error(function (data, status, headers, config) {
                refreshConfig();
                $scope.emitHTTPError(data, status, headers, config);
            });
        };

        $scope.urVersions = function () {
            var result = [];
            if ($scope.system) {
                for (var i = $scope.system.urVersionMax; i >= 2; i--) {
                    result.push("" + i);
                }
            }
            return result;
        };

        $scope.settingsModified = function () {
            // Options has artificial properties injected into the temp config.
            // Need to recompute them before we can check equality
            var options = angular.copy($scope.config.options);
            options.deviceName = $scope.thisDevice().name;
            options.upgrades = "none";
            if (options.autoUpgradeIntervalH > 0) {
                options.upgrades = "stable";
            }
            if (options.upgradeToPreReleases) {
                options.upgrades = "candidate";
            }
            var optionsEqual = angular.equals(options, $scope.tmpOptions);
            var guiEquals = angular.equals($scope.config.gui, $scope.tmpGUI);
            var ignoredDevicesEquals = angular.equals($scope.config.remoteIgnoredDevices, $scope.tmpRemoteIgnoredDevices);
            var ignoredFoldersEquals = angular.equals($scope.config.devices, $scope.tmpDevices);
            console.log("settings equals - options: " + optionsEqual + " gui: " + guiEquals + " ignDev: " + ignoredDevicesEquals + " ignFol: " + ignoredFoldersEquals);
            return !optionsEqual || !guiEquals || !ignoredDevicesEquals || !ignoredFoldersEquals;
        };

        $scope.saveSettings = function () {
            // Make sure something changed
            if ($scope.settingsModified()) {
                var themeChanged = $scope.config.gui.theme !== $scope.tmpGUI.theme;
                // Angular has issues with selects with numeric values, so we handle strings here.
                $scope.tmpOptions.urAccepted = parseInt($scope.tmpOptions._urAcceptedStr);
                // Check if auto-upgrade has been enabled or disabled. This
                // also has an effect on usage reporting, so do the check
                // for that later.
                if ($scope.tmpOptions.upgrades == "candidate") {
                    $scope.tmpOptions.autoUpgradeIntervalH = $scope.tmpOptions.autoUpgradeIntervalH || 12;
                    $scope.tmpOptions.upgradeToPreReleases = true;
                    $scope.tmpOptions.urAccepted = $scope.system.urVersionMax;
                    $scope.tmpOptions.urSeen = $scope.system.urVersionMax;
                } else if ($scope.tmpOptions.upgrades == "stable") {
                    $scope.tmpOptions.autoUpgradeIntervalH = $scope.tmpOptions.autoUpgradeIntervalH || 12;
                    $scope.tmpOptions.upgradeToPreReleases = false;
                } else {
                    $scope.tmpOptions.autoUpgradeIntervalH = 0;
                    $scope.tmpOptions.upgradeToPreReleases = false;
                }

                // Check if protocol will need to be changed on restart
                if ($scope.config.gui.useTLS !== $scope.tmpGUI.useTLS) {
                    $scope.protocolChanged = true;
                }

                // Parse strings to arrays before copying over
                ['listenAddresses', 'globalAnnounceServers'].forEach(function (key) {
                    $scope.tmpOptions[key] = $scope.tmpOptions["_" + key + "Str"].split(/[ ,]+/).map(function (x) {
                        return x.trim();
                    });
                });

                // Apply new settings locally
                $scope.thisDeviceIn($scope.tmpDevices).name = $scope.tmpOptions.deviceName;
                $scope.config.options = angular.copy($scope.tmpOptions);
                $scope.config.gui = angular.copy($scope.tmpGUI);
                $scope.config.remoteIgnoredDevices = angular.copy($scope.tmpRemoteIgnoredDevices);
                $scope.config.devices = angular.copy($scope.tmpDevices);
                // $scope.devices is updated by updateLocalConfig based on
                // the config changed event, but settingsModified will look
                // at it before that and conclude that the settings are
                // modified (even though we just saved) unless we update
                // here as well...
                $scope.devices = deviceMap($scope.config.devices);

                $scope.saveConfig(function () {
                    if (themeChanged) {
                        document.location.reload(true);
                    }
                });
            }

            $("#settings").off("hide.bs.modal").modal("hide");
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
            $('#upgrade').modal('hide');
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

        function editDeviceModal() {
            $scope.currentDevice._addressesStr = $scope.currentDevice.addresses.join(', ');
            $scope.deviceEditor.$setPristine();
            $('#editDevice').modal();
        }

        $scope.editDeviceModalTitle = function() {
            if ($scope.editingDefaults) {
                return $translate.instant("Edit Device Defaults");
            }
            var title = '';
            if ($scope.editingExisting) {
                title += $translate.instant("Edit Device");
            } else {
                title += $translate.instant("Add Device");
            }
            var name = $scope.deviceName($scope.currentDevice);
            if (name !== '') {
                title += ' (' + name + ')';
            }
            return title;
        };

        $scope.editDeviceModalIcon = function() {
            if ($scope.editingDefaults || $scope.editingExisting) {
                return 'fas fa-pencil-alt';
            }
            return 'fas fa-desktop';
        };

        $scope.editDeviceExisting = function (deviceCfg) {
            $scope.currentDevice = $.extend({}, deviceCfg);
            $scope.editingExisting = true;
            $scope.editingDefaults = false;
            $scope.willBeReintroducedBy = undefined;
            if (deviceCfg.introducedBy) {
                var introducerDevice = $scope.devices[deviceCfg.introducedBy];
                if (introducerDevice && introducerDevice.introducer) {
                    $scope.willBeReintroducedBy = $scope.deviceName(introducerDevice);
                }
            }
            initShareEditing('device');
            $scope.deviceFolders($scope.currentDevice).forEach(function (folderID) {
                $scope.currentSharing.shared.push($scope.folders[folderID]);
                $scope.currentSharing.selected[folderID] = true;
                var folderdevices = $scope.folders[folderID].devices;
                for (var i = 0; i < folderdevices.length; i++) {
                    if (folderdevices[i].deviceID === deviceCfg.deviceID) {
                        $scope.currentSharing.encryptionPasswords[folderID] = folderdevices[i].encryptionPassword;
                        break;
                    }
                }
            });
            $scope.currentSharing.unrelated = $scope.folderList().filter(function (n) {
                return !$scope.currentSharing.selected[n.id];
            });
            editDeviceModal();
        };

        $scope.editDeviceDefaults = function () {
            $http.get(urlbase + '/config/defaults/device').then(function (p) {
                $scope.currentDevice = p.data;
                $scope.editingDefaults = true;
                editDeviceModal();
            }, $scope.emitHTTPError);
        };

        $scope.selectAllSharedFolders = function (state) {
            var folders = $scope.currentSharing.shared;
            for (var i = 0; i < folders.length; i++) {
                $scope.currentSharing.selected[folders[i].id] = !!state;
            }
        };

        $scope.selectAllUnrelatedFolders = function (state) {
            var folders = $scope.currentSharing.unrelated;
            for (var i = 0; i < folders.length; i++) {
                $scope.currentSharing.selected[folders[i].id] = !!state;
            }
        };

        $scope.addDevice = function (deviceID, name) {
            return $http.get(urlbase + '/system/discovery')
                .success(function (registry) {
                    $scope.discovery = [];
                    for (var id in registry) {
                        if ($scope.discovery.length === 5) {
                            break;
                        }
                        if (id in $scope.devices) {
                            continue
                        }
                        $scope.discovery.push(id);
                    }
                })
                .then(function () {
                    $http.get(urlbase + '/config/defaults/device').then(function (p) {
                        $scope.currentDevice = p.data;
                        $scope.currentDevice.name = name;
                        $scope.currentDevice.deviceID = deviceID;
                        $scope.editingExisting = false;
                        $scope.editingDefaults = false;
                        initShareEditing('device');
                        $scope.currentSharing.unrelated = $scope.folderList();
                        editDeviceModal();
                    }, $scope.emitHTTPError);
                });
        };

        $scope.deleteDevice = function () {
            $('#editDevice').modal('hide');
            if (!$scope.editingExisting) {
                return;
            }

            var id = $scope.currentDevice.deviceID
            delete $scope.devices[id];
            $scope.config.devices = $scope.deviceList();

            for (var id in $scope.folders) {
                $scope.folders[id].devices = $scope.folders[id].devices.filter(function (n) {
                    return n.deviceID !== $scope.currentDevice.deviceID;
                });
            }

            $scope.saveConfig();
        };

        $scope.saveDevice = function () {
            $('#editDevice').modal('hide');
            $scope.currentDevice.addresses = $scope.currentDevice._addressesStr.split(',').map(function (x) {
                return x.trim();
            });
            delete $scope.currentDevice._addressesStr;
            if ($scope.editingDefaults) {
                $scope.config.defaults.device = $scope.currentDevice;
            } else {
                setDeviceConfig();
            }
            delete $scope.currentSharing;
            delete $scope.currentDevice;
            $scope.saveConfig();
        };

        function setDeviceConfig() {
            var currentID = $scope.currentDevice.DeviceID;
            $scope.devices[currentID] = $scope.currentDevice;
            $scope.config.devices = deviceList($scope.devices);

            for (var id in $scope.currentSharing.selected) {
                if ($scope.currentSharing.selected[id]) {
                    var found = false;
                    for (i = 0; i < $scope.folders[id].devices.length; i++) {
                        if ($scope.folders[id].devices[i].deviceID === currentID) {
                            found = true;
                            // Update encryption pw
                            $scope.folders[id].devices[i].encryptionPassword = $scope.currentSharing.encryptionPasswords[id];
                            break;
                        }
                    }

                    if (!found) {
                        // Add device to folder
                        $scope.folders[id].devices.push({
                            deviceID: currentID,
                            encryptionPassword: $scope.currentSharing.encryptionPasswords[id],
                        });
                    }
                } else {
                    // Remove device from folder
                    $scope.folders[id].devices = $scope.folders[id].devices.filter(function (n) {
                        return n.deviceID !== currentID;
                    });
                }
            }

            $scope.config.folders = folderList($scope.folders);
        };

        $scope.ignoreDevice = function (deviceID, pendingDevice) {
            var ignoredDevice = angular.copy(pendingDevice);
            ignoredDevice.deviceID = deviceID;
            // Bump time
            ignoredDevice.time = (new Date()).toISOString();
            $scope.config.remoteIgnoredDevices.push(ignoredDevice);
            $scope.saveConfig();
        };

        $scope.unignoreDeviceFromTemporaryConfig = function (ignoredDevice) {
            $scope.tmpRemoteIgnoredDevices = $scope.tmpRemoteIgnoredDevices.filter(function (existingIgnoredDevice) {
                return ignoredDevice.deviceID !== existingIgnoredDevice.deviceID;
            });
        };

        $scope.ignoredFoldersCountTmpConfig = function () {
            var count = 0;
            ($scope.tmpDevices || []).forEach(function (deviceCfg) {
                count += deviceCfg.ignoredFolders.length;
            });
            return count;
        };

        $scope.unignoreFolderFromTemporaryConfig = function (device, ignoredFolderID) {
            for (var i = 0; i < $scope.tmpDevices.length; i++) {
                if ($scope.tmpDevices[i].deviceID == device) {
                    $scope.tmpDevices[i].ignoredFolders = $scope.tmpDevices[i].ignoredFolders.filter(function (existingIgnoredFolder) {
                        return existingIgnoredFolder.id !== ignoredFolderID;
                    });
                    return;
                }
            }
        };

        $scope.otherDevices = function () {
            return $scope.deviceList().filter(function (n) {
                return n.deviceID !== $scope.myID;
            });
        };

        $scope.thisDevice = function () {
            return $scope.devices[$scope.myID];
        };

        $scope.thisDeviceIn = function (l) {
            for (var i = 0; i < l.length; i++) {
                var n = l[i];
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

        $scope.setAllDevicesPause = function (pause) {
            for (var id in $scope.devices) {
                $scope.devices[id].paused = pause;
            };
            $scope.config.devices = deviceList($scope.devices);
            $scope.saveConfig();
        }

        $scope.isAtleastOneDevicePausedStateSetTo = function (pause) {
            for (var id in $scope.devices) {
                if ($scope.devices[id].paused == pause) {
                    return true;
                }
            }

            return false
        }

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

        $scope.fsWatcherErrorMap = function () {
            var errs = {}
            $.each($scope.folders, function (id, cfg) {
                if (cfg.fsWatcherEnabled && $scope.model[cfg.id] && $scope.model[id].watchError && !cfg.paused && $scope.folderStatus(cfg) !== 'stopped') {
                    errs[id] = $scope.model[id].watchError;
                }
            });
            return errs;
        };

        $scope.friendlyDevices = function (str) {
            for (var id in $scope.devices) {
                str = str.replace(id, $scope.deviceName($scope.devices[id]));
            }
            return str;
        };

        $scope.folderList = function () {
            return folderList($scope.folders);
        };

        $scope.deviceList = function () {
            return deviceList($scope.devices);
        };

        $scope.directoryList = [];

        $scope.$watch('currentFolder.path', function (newvalue) {
            if (!newvalue) {
                return;
            }
            $scope.currentFolder.path = expandTilde(newvalue);
            $http.get(urlbase + '/system/browse', {
                params: { current: newvalue }
            }).success(function (data) {
                $scope.directoryList = data;
            }).error($scope.emitHTTPError);
        });

        $scope.$watch('currentFolder.label', function (newvalue) {
            if (!newvalue || !shouldSetDefaultFolderPath()) {
                return;
            }
            $scope.currentFolder.path = pathJoin($scope.config.defaults.folder.path, newvalue);
        });

        $scope.$watch('currentFolder.id', function (newvalue) {
            if (!newvalue || !shouldSetDefaultFolderPath() || $scope.currentFolder.label) {
                return;
            }
            $scope.currentFolder.path = pathJoin($scope.config.defaults.folder.path, newvalue);
        });

        $scope.fsWatcherToggled = function () {
            if ($scope.currentFolder.fsWatcherEnabled) {
                $scope.currentFolder.rescanIntervalS = 3600;
            } else {
                $scope.currentFolder.rescanIntervalS = 60;
            }
        };

        $scope.loadFormIntoScope = function (form) {
            console.log('loadFormIntoScope', form.$name);
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

        function editFolderModal() {
            initVersioningEditing();
            $scope.folderPathErrors = {};
            $scope.folderEditor.$setPristine();
            $('#editFolder').modal().one('shown.bs.tab', function (e) {
                if (e.target.attributes.href.value === "#folder-ignores") {
                    $('#folder-ignores textarea').focus();
                }
            }).one('hidden.bs.modal', function () {
                $('.nav-tabs a[href="#folder-general"]').tab('show');
                window.location.hash = "";
            });
        };

        $scope.editFolderModalTitle = function() {
            if ($scope.editingDefaults) {
                return $translate.instant("Edit Folder Defaults");
            }
            var title = '';
            if ($scope.editingExisting) {
                title += $translate.instant("Edit Folder");
            } else {
                title += $translate.instant("Add Folder");
            }
            if ($scope.currentFolder.id !== '') {
                title += ' (' + $scope.folderLabel($scope.currentFolder.id) + ')';
            }
            return title;
        };

        $scope.editFolderModalIcon = function() {
            if ($scope.editingDefaults || $scope.editingExisting) {
                return 'fas fa-pencil-alt';
            }
            return 'fas fa-folder';
        };

        function editFolder() {
            if ($scope.currentFolder.path.length > 1 && $scope.currentFolder.path.slice(-1) === $scope.system.pathSeparator) {
                $scope.currentFolder.path = $scope.currentFolder.path.slice(0, -1);
            } else if (!$scope.currentFolder.path) {
                // undefined path leads to invalid input field
                $scope.currentFolder.path = '';
            }
            initShareEditing('folder');
            editFolderModal();
        }

        function initVersioningEditing() {
            $scope.currentFolder._guiVersioning = angular.copy($scope.versioningDefaults);

            if (!$scope.currentFolder.versioning) {
                return;
            }

            var currentVersioning = $scope.currentFolder.versioning;

            $scope.currentFolder._guiVersioning.cleanupIntervalS = +currentVersioning.cleanupIntervalS;

            // Apply parameters currently in use
            switch (currentVersioning.type) {
            case "trashcan":
                $scope.currentFolder._guiVersioning.selector = "trashcan";
                $scope.currentFolder._guiVersioning.trashcanClean = +currentVersioning.params.cleanoutDays;
                break;
            case "simple":
                $scope.currentFolder._guiVersioning.selector = "simple";
                $scope.currentFolder._guiVersioning.simpleKeep = +currentVersioning.params.keep;
                $scope.currentFolder._guiVersioning.trashcanClean = +currentVersioning.params.cleanoutDays;
                break;
            case "staggered":
                $scope.currentFolder._guiVersioning.selector = "staggered";
                $scope.currentFolder._guiVersioning.staggeredMaxAge = Math.floor(+currentVersioning.params.maxAge / 86400);
                $scope.currentFolder._guiVersioning.staggeredCleanInterval = +currentVersioning.params.cleanInterval;
                $scope.currentFolder._guiVersioning.staggeredVersionsPath = currentVersioning.params.versionsPath;
                break;
            case "external":
                $scope.currentFolder._guiVersioning.selector = "external";
                $scope.currentFolder.externalCommand = currentVersioning.params.command;
                break;
            }
        };

        $scope.editFolderExisting = function(folderCfg) {
            $scope.editingExisting = true;
            $scope.currentFolder = angular.copy(folderCfg);

            $scope.ignores.text = 'Loading...';
            $scope.ignores.error = null;
            $scope.ignores.disabled = true;
            $http.get(urlbase + '/db/ignores?folder=' + encodeURIComponent($scope.currentFolder.id))
                .success(function (data) {
                    $scope.currentFolder.ignores = data.ignore || [];
                    $scope.ignores.text = $scope.currentFolder.ignores.join('\n');
                    $scope.ignores.error = data.error;
                    $scope.ignores.disabled = false;
                })
                .error(function (err) {
                    $scope.ignores.text = $translate.instant("Failed to load ignore patterns.");
                    $scope.emitHTTPError(err);
                });

            editFolder();
        };

        $scope.editFolderDefaults = function() {
            $http.get(urlbase + '/config/defaults/folder')
                 .success(function (data) {
                     $scope.currentFolder = data;
                     $scope.editingExisting = false;
                     $scope.editingDefaults = true;
                     editFolder();
                 })
                 .error($scope.emitHTTPError);
        };

        $scope.selectAllSharedDevices = function (state) {
            var devices = $scope.currentSharing.shared;
            for (var i = 0; i < devices.length; i++) {
                $scope.currentSharing.selected[devices[i].deviceID] = !!state;
            }
        };

        $scope.selectAllUnrelatedDevices = function (state) {
            var devices = $scope.currentSharing.unrelated;
            for (var i = 0; i < devices.length; i++) {
                $scope.currentSharing.selected[devices[i].deviceID] = !!state;
            }
        };

        $scope.addFolder = function () {
            $http.get(urlbase + '/svc/random/string?length=10').success(function (data) {
                var folderID = (data.random.substr(0, 5) + '-' + data.random.substr(5, 5)).toLowerCase();
                addFolderInit(folderID).then(function() {
                    // Triggers the watch that sets the path
                    $scope.currentFolder.label = $scope.currentFolder.label;
                    editFolderModal();
                });
            });
        };

        $scope.addFolderAndShare = function (folderID, folderLabel, device) {
            addFolderInit(folderID).then(function() {
                $scope.currentFolder.viewFlags = {
                    importFromOtherDevice: true
                };
                $scope.currentSharing.selected[device] = true;
                $scope.currentFolder.label = folderLabel;
                editFolderModal();
            });
        };

        function addFolderInit(folderID) {
            $scope.editingExisting = false;
            $scope.editingDefaults = false;
            return $http.get(urlbase + '/config/defaults/folder').then(function(p) {
                $scope.currentFolder = p.data;
                $scope.currentFolder.id = folderID;

                initShareEditing('folder');
                $scope.currentSharing.unrelated = $scope.currentSharing.unrelated.concat($scope.currentSharing.shared);
                $scope.currentSharing.shared = [];

                $scope.ignores.text = '';
                $scope.ignores.error = null;
                $scope.ignores.disabled = false;
            }, $scope.emitHTTPError);
        }

        $scope.shareFolderWithDevice = function (folder, device) {
            $scope.folders[folder].devices.push({
                deviceID: device
            });
            $scope.config.folders = folderList($scope.folders);
            $scope.saveConfig();
        };

        $scope.saveFolder = function () {
            $('#editFolder').modal('hide');
            var folderCfg = angular.copy($scope.currentFolder);
            $scope.currentSharing.selected[$scope.myID] = true;
            var newDevices = [];
            folderCfg.devices.forEach(function (dev) {
                if ($scope.currentSharing.selected[dev.deviceID] === true) {
                    dev.encryptionPassword = $scope.currentSharing.encryptionPasswords[dev.deviceID];
                    newDevices.push(dev);
                    delete $scope.currentSharing.selected[dev.deviceID];
                };
            });
            for (var deviceID in $scope.currentSharing.selected) {
                if ($scope.currentSharing.selected[deviceID] === true) {
                    newDevices.push({
                        deviceID: deviceID,
                        encryptionPassword: $scope.currentSharing.encryptionPasswords[deviceID],
                    });
                }
            }
            folderCfg.devices = newDevices;
            delete $scope.currentSharing;

            switch (folderCfg._guiVersioning.selector) {
            case "trashcan":
                folderCfg.versioning = {
                    'type': 'trashcan',
                    'params': {
                        'cleanoutDays': '' + folderCfg._guiVersioning.trashcanClean
                    },
                    'cleanupIntervalS': folderCfg._guiVersioning.cleanupIntervalS
                };
                break;
            case "simple":
                folderCfg.versioning = {
                    'type': 'simple',
                    'params': {
                        'keep': '' + folderCfg._guiVersioning.simpleKeep,
                        'cleanoutDays': '' + folderCfg._guiVersioning.trashcanClean
                    },
                    'cleanupIntervalS': folderCfg._guiVersioning.cleanupIntervalS
                };
                break;
            case "staggered":
                folderCfg.versioning = {
                    'type': 'staggered',
                    'params': {
                        'maxAge': '' + (folderCfg._guiVersioning.staggeredMaxAge * 86400),
                        'cleanInterval': '' + folderCfg._guiVersioning.staggeredCleanInterval,
                        'versionsPath': '' + folderCfg._guiVersioning.staggeredVersionsPath
                    },
                    'cleanupIntervalS': folderCfg._guiVersioning.cleanupIntervalS
                };
                break;
            case "external":
                folderCfg.versioning = {
                    'type': 'external',
                    'params': {
                        'command': '' + folderCfg._guiVersioning.externalCommand
                    },
                    'cleanupIntervalS': folderCfg._guiVersioning.cleanupIntervalS
                };
                break;
            default:
                delete folderCfg.versioning;
            }
            delete folderCfg._guiVersioning;

            if ($scope.editingDefaults) {
                $scope.config.defaults.folder = folderCfg;
                $scope.saveConfig();
            } else {
                saveFolderExisting(folderCfg);
            }
        };

        function saveFolderExisting(folderCfg) {
            var ignoresLoaded = !$scope.ignores.disabled;
            var ignores = $scope.ignores.text.split('\n');
            // Split always returns a minimum 1-length array even for no patterns
            if (ignores.length === 1 && ignores[0] === "") {
                ignores = [];
            }
            if (!$scope.editingExisting && ignores.length) {
                folderCfg.paused = true;
            };

            $scope.folders[folderCfg.id] = folderCfg;
            $scope.config.folders = folderList($scope.folders);

            if (ignoresLoaded && $scope.editingExisting && ignores !== folderCfg.ignores) {
                saveIgnores(ignores);
            };

            $scope.saveConfig(function () {
                if (!$scope.editingExisting && ignores.length) {
                    saveIgnores(ignores, function () {
                        $scope.setFolderPause(folderCfg.id, false);
                    });
                }
            });
        };

        $scope.ignoreFolder = function (device, folderID, offeringDevice) {
            var ignoredFolder = {
                id: folderID,
                label: offeringDevice.label,
                // Bump time
                time: (new Date()).toISOString()
            }

            if (device in $scope.devices) {
                $scope.devices[device].ignoredFolders.push(ignoredFolder);
                $scope.saveConfig();
            }
        };

        $scope.sharesFolder = function (folderCfg) {
            var names = [];
            folderCfg.devices.forEach(function (device) {
                if (device.deviceID !== $scope.myID) {
                    names.push($scope.deviceName($scope.devices[device.deviceID]));
                }
            });
            names.sort();
            return names.join(", ");
        };

        $scope.deviceFolders = function (deviceCfg) {
            var folders = [];
            $scope.folderList().forEach(function (folder) {
                for (var i = 0; i < folder.devices.length; i++) {
                    if (folder.devices[i].deviceID === deviceCfg.deviceID) {
                        folders.push(folder.id);
                        break;
                    }
                }
            });
            return folders;
        };

        $scope.folderLabel = function (folderID) {
            if (!$scope.folders[folderID]) {
                return folderID;
            }
            var label = $scope.folders[folderID].label;
            return label && label.length > 0 ? label : folderID;
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

        function resetRestoreVersions() {
            $scope.restoreVersions = {
                folder: null,
                selections: {},
                versions: null,
                tree: null,
                errors: null,
                filters: {},
                massAction: function (name, action) {
                    $.each($scope.restoreVersions.versions, function (key) {
                        if (key.indexOf(name + '/') == 0 && (!$scope.restoreVersions.filters.text || key.indexOf($scope.restoreVersions.filters.text) > -1)) {
                            if (action == 'unset') {
                                delete $scope.restoreVersions.selections[key];
                                return;
                            }

                            var availableVersions = [];
                            $.each($scope.restoreVersions.filterVersions($scope.restoreVersions.versions[key]), function (idx, version) {
                                availableVersions.push(version.versionTime);
                            })

                            if (availableVersions.length) {
                                availableVersions.sort(function (a, b) { return a - b; });
                                if (action == 'latest') {
                                    $scope.restoreVersions.selections[key] = availableVersions.pop();
                                } else if (action == 'oldest') {
                                    $scope.restoreVersions.selections[key] = availableVersions.shift();
                                }
                            }
                        }
                    });
                },
                filterVersions: function (versions) {
                    var filteredVersions = [];
                    $.each(versions, function (idx, version) {
                        if (moment(version.versionTime).isBetween($scope.restoreVersions.filters['start'], $scope.restoreVersions.filters['end'], null, '[]')) {
                            filteredVersions.push(version);
                        }
                    });
                    return filteredVersions;
                },
                selectionCount: function () {
                    var count = 0;
                    $.each($scope.restoreVersions.selections, function (key, value) {
                        if (value) {
                            count++;
                        }
                    });
                    return count;
                },

                restore: function () {
                    $scope.restoreVersions.tree.clear();
                    $scope.restoreVersions.tree = null;
                    $scope.restoreVersions.versions = null;
                    var selections = {};
                    $.each($scope.restoreVersions.selections, function (key, value) {
                        if (value) {
                            selections[key] = value;
                        }
                    });
                    $scope.restoreVersions.selections = {};

                    $http.post(urlbase + '/folder/versions?folder=' + encodeURIComponent($scope.restoreVersions.folder), selections).success(function (data) {
                        if (Object.keys(data).length == 0) {
                            $('#restoreVersions').modal('hide');
                        } else {
                            $scope.restoreVersions.errors = data;
                        }
                    });
                },
                show: function (folder) {
                    $scope.restoreVersions.folder = folder;

                    var closed = false;
                    var modalShown = $q.defer();
                    $('#restoreVersions').modal().one('hidden.bs.modal', function () {
                        closed = true;
                        resetRestoreVersions();
                    }).one('shown.bs.modal', function () {
                        modalShown.resolve();
                    });

                    var dataReceived = $http.get(urlbase + '/folder/versions?folder=' + encodeURIComponent($scope.restoreVersions.folder))
                        .success(function (data) {
                            $.each(data, function (key, values) {
                                $.each(values, function (idx, value) {
                                    value.modTime = new Date(value.modTime);
                                    value.versionTime = new Date(value.versionTime);
                                });
                                values.sort(function (a, b) {
                                    return b.versionTime - a.versionTime;
                                });
                            });
                            if (closed) return;
                            $scope.restoreVersions.versions = data;
                        });

                    $q.all([dataReceived, modalShown.promise]).then(function () {
                        $timeout(function () {
                            if (closed) {
                                resetRestoreVersions();
                                return;
                            }

                            $scope.restoreVersions.tree = $("#restoreTree").fancytree({
                                extensions: ["table", "filter"],
                                quicksearch: true,
                                filter: {
                                    autoApply: true,
                                    counter: true,
                                    hideExpandedCounter: true,
                                    hideExpanders: true,
                                    highlight: true,
                                    leavesOnly: false,
                                    nodata: true,
                                    mode: "hide"
                                },
                                table: {
                                    indentation: 20,
                                    nodeColumnIdx: 0,
                                },
                                debugLevel: 2,
                                source: buildTree($scope.restoreVersions.versions),
                                renderColumns: function (event, data) {
                                    var node = data.node,
                                        $tdList = $(node.tr).find(">td"),
                                        template;
                                    if (node.folder) {
                                        template = '<div ng-include="\'syncthing/folder/restoreVersionsMassActions.html\'" class="pull-right"/>';
                                    } else {
                                        template = '<div ng-include="\'syncthing/folder/restoreVersionsVersionSelector.html\'" class="pull-right"/>';
                                    }

                                    var scope = $rootScope.$new(true);
                                    scope.key = node.key;
                                    scope.restoreVersions = $scope.restoreVersions;

                                    $tdList.eq(1).html(
                                        $compile(template)(scope)
                                    );

                                    // Force angular to redraw.
                                    $timeout(function () {
                                        $scope.$apply();
                                    });
                                }
                            }).fancytree("getTree");

                            var minDate = moment(),
                                maxDate = moment(0, 'X'),
                                date;

                            // Find version window.
                            $.each($scope.restoreVersions.versions, function (key) {
                                $.each($scope.restoreVersions.versions[key], function (idx, version) {
                                    date = moment(version.versionTime);
                                    if (date.isBefore(minDate)) {
                                        minDate = date;
                                    }
                                    if (date.isAfter(maxDate)) {
                                        maxDate = date;
                                    }
                                });
                            });

                            $scope.restoreVersions.filters['start'] = minDate;
                            $scope.restoreVersions.filters['end'] = maxDate;

                            var ranges = {
                                'All time': [minDate, maxDate],
                                'Today': [moment(), moment()],
                                'Yesterday': [moment().subtract(1, 'days'), moment().subtract(1, 'days')],
                                'Last 7 Days': [moment().subtract(6, 'days'), moment()],
                                'Last 30 Days': [moment().subtract(29, 'days'), moment()],
                                'This Month': [moment().startOf('month'), moment().endOf('month')],
                                'Last Month': [moment().subtract(1, 'month').startOf('month'), moment().subtract(1, 'month').endOf('month')]
                            };

                            // Filter out invalid ranges.
                            $.each(ranges, function (key, range) {
                                if (!range[0].isBetween(minDate, maxDate, null, '[]') && !range[1].isBetween(minDate, maxDate, null, '[]')) {
                                    delete ranges[key];
                                }
                            });

                            $("#restoreVersionDateRange").daterangepicker({
                                timePicker: true,
                                timePicker24Hour: true,
                                timePickerSeconds: true,
                                autoUpdateInput: true,
                                opens: "left",
                                drops: "up",
                                startDate: minDate,
                                endDate: maxDate,
                                minDate: minDate,
                                maxDate: maxDate,
                                ranges: ranges,
                                locale: {
                                    format: 'YYYY/MM/DD HH:mm:ss',
                                }
                            }).on('apply.daterangepicker', function (ev, picker) {
                                $scope.restoreVersions.filters['start'] = picker.startDate;
                                $scope.restoreVersions.filters['end'] = picker.endDate;
                                // Events for this UI element are not managed by angular.
                                // Force angular to wake up.
                                $timeout(function () {
                                    $scope.$apply();
                                });
                            });
                        });
                    });
                }
            };
        }
        resetRestoreVersions();

        $scope.$watchCollection('restoreVersions.filters', function () {
            if (!$scope.restoreVersions.tree) return;

            $scope.restoreVersions.tree.filterNodes(function (node) {
                if (node.folder) return false;
                if ($scope.restoreVersions.filters.text && node.key.indexOf($scope.restoreVersions.filters.text) < 0) {
                    return false;
                }
                if ($scope.restoreVersions.filterVersions(node.data.versions).length == 0) {
                    return false;
                }
                return true;
            });
        });

        $scope.setAPIKey = function (cfg) {
            $http.get(urlbase + '/svc/random/string?length=32').success(function (data) {
                cfg.apiKey = data.random;
            });
        };

        $scope.acceptUR = function () {
            $scope.config.options.urAccepted = $scope.system.urVersionMax;
            $scope.config.options.urSeen = $scope.system.urVersionMax;
            $scope.saveConfig();
            $('#ur').modal('hide');
        };

        $scope.declineUR = function () {
            if ($scope.config.options.urAccepted === 0) {
                $scope.config.options.urAccepted = -1;
            }
            $scope.config.options.urSeen = $scope.system.urVersionMax;
            $scope.saveConfig();
            $('#ur').modal('hide');
        };

        $scope.showNeed = function (folder) {
            $scope.neededFolder = folder;
            $scope.refreshNeed(1, 10);
            $('#needed').modal().one('hidden.bs.modal', function () {
                $scope.needed = undefined;
                $scope.neededFolder = '';
            });
        };

        $scope.showRemoteNeed = function (device) {
            resetRemoteNeed();
            $scope.remoteNeedDevice = device;
            $scope.deviceFolders(device).forEach(function (folder) {
                var comp = $scope.completion[device.deviceID][folder];
                if (comp !== undefined && comp.needItems + comp.needDeletes === 0) {
                    return;
                }
                $scope.remoteNeedFolders.push(folder);
                $scope.refreshRemoteNeed(folder, 1, 10);
            });
            $('#remoteNeed').modal().one('hidden.bs.modal', function () {
                resetRemoteNeed();
            });
        };

        $scope.showFailed = function (folder) {
            $scope.failed.folder = folder;
            $scope.failed = $scope.refreshFailed(1, 10);
            $('#failed').modal().one('hidden.bs.modal', function () {
                $scope.failed = {};
            });
        };

        $scope.hasFailedFiles = function (folder) {
            if (!$scope.model[folder]) {
                return false;
            }
            return $scope.model[folder].errors !== 0;
        };

        $scope.override = function (folder) {
            $http.post(urlbase + "/db/override?folder=" + encodeURIComponent(folder));
        };

        $scope.showLocalChanged = function (folder, folderType) {
            $scope.localChangedFolder = folder;
            $scope.localChangedType = folderType;
            $scope.localChanged = $scope.refreshLocalChanged(1, 10);
            $('#localChanged').modal().one('hidden.bs.modal', function () {
                $scope.localChanged = {};
                $scope.localChangedFolder = undefined;
                $scope.localChangedType = undefined;
            });
        };

        $scope.hasReceiveOnlyChanged = function (folderCfg) {
            if (!folderCfg || folderCfg.type !== "receiveonly") {
                return false;
            }
            var counts = $scope.model[folderCfg.id];
            return counts && counts.receiveOnlyTotalItems > 0;
        };

        $scope.hasReceiveEncryptedItems = function (folderCfg) {
            if (!folderCfg || folderCfg.type !== "receiveencrypted") {
                return false;
            }
            return $scope.receiveEncryptedItemsCount(folderCfg) > 0;
        };

        $scope.receiveEncryptedItemsCount = function (folderCfg) {
            var counts = $scope.model[folderCfg.id];
            if (!counts) {
                return 0;
            }
            return counts.receiveOnlyTotalItems - counts.receiveOnlyChangedDeletes;
        }

        $scope.revert = function (folder) {
            $http.post(urlbase + "/db/revert?folder=" + encodeURIComponent(folder));
        };

        $scope.deleteEncryptionModal = function (folderID) {
            $scope.revertEncryptionFolder = folderID;
            $('#delete-encryption-confirmation').modal('show').one('hidden.bs.modal', function () {
                $scope.revertEncryptionFolder = undefined;
            });
        };

        $scope.advanced = function () {
            $scope.advancedConfig = angular.copy($scope.config);
            $scope.advancedConfig.devices.sort(deviceCompare);
            $scope.advancedConfig.folders.sort(folderCompare);
            $('#advanced').modal('show');
        };

        $scope.showReportPreview = function () {
            $scope.reportPreview = true;
        };

        $scope.refreshReportDataPreview = function (ver, diff) {
            $scope.reportDataPreview = '';
            if (!ver) {
                return;
            }
            var version = parseInt(ver);
            if (diff && version > 2) {
                $q.all([
                    $http.get(urlbase + '/svc/report?version=' + version),
                    $http.get(urlbase + '/svc/report?version=' + (version - 1)),
                ]).then(function (responses) {
                    var newReport = responses[0].data;
                    var oldReport = responses[1].data;
                    angular.forEach(oldReport, function (_, key) {
                        delete newReport[key];
                    });
                    $scope.reportDataPreview = newReport;
                });
            } else {
                $http.get(urlbase + '/svc/report?version=' + version).success(function (data) {
                    $scope.reportDataPreview = data;
                }).error($scope.emitHTTPError);
            }
        };

        $scope.rescanAllFolders = function () {
            $http.post(urlbase + "/db/scan");
        };

        $scope.rescanFolder = function (folder) {
            $http.post(urlbase + "/db/scan?folder=" + encodeURIComponent(folder));
        };

        $scope.setAllFoldersPause = function (pause) {
            var folderListCache = $scope.folderList();

            for (var i = 0; i < folderListCache.length; i++) {
                folderListCache[i].paused = pause;
            }

            $scope.config.folders = folderList(folderListCache);
            $scope.saveConfig();
        };

        $scope.isAtleastOneFolderPausedStateSetTo = function (pause) {
            var folderListCache = $scope.folderList();

            for (var i = 0; i < folderListCache.length; i++) {
                if (folderListCache[i].paused == pause) {
                    return true;
                }
            }

            return false;
        };

        $scope.activateAllFsWatchers = function () {
            var folders = $scope.folderList();

            $.each(folders, function (i) {
                if (folders[i].fsWatcherEnabled) {
                    return;
                }
                folders[i].fsWatcherEnabled = true;
                if (folders[i].rescanIntervalS === 0) {
                    return;
                }
                // Delay full scans, but scan at least once per day
                folders[i].rescanIntervalS *= 60;
                if (folders[i].rescanIntervalS > 86400) {
                    folders[i].rescanIntervalS = 86400;
                }
            });

            $scope.config.folders = folders;
            $scope.saveConfig();
        };

        $scope.bumpFile = function (folder, file) {
            var url = urlbase + "/db/prio?folder=" + encodeURIComponent(folder) + "&file=" + encodeURIComponent(file);
            // In order to get the right view of data in the response.
            url += "&page=" + $scope.needed.page;
            url += "&perpage=" + $scope.needed.perpage;
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
                'darwin': 'macOS',
                'dragonfly': 'DragonFly BSD',
                'freebsd': 'FreeBSD',
                'openbsd': 'OpenBSD',
                'netbsd': 'NetBSD',
                'linux': 'Linux',
                'windows': 'Windows',
                'solaris': 'Solaris'
            }[$scope.version.os] || $scope.version.os;

            var arch = {
                '386': '32-bit Intel/AMD',
                'amd64': '64-bit Intel/AMD',
                'arm': '32-bit ARM',
                'arm64': '64-bit ARM',
                'ppc64': '64-bit PowerPC',
                'ppc64le': '64-bit PowerPC (LE)',
                'mips': '32-bit MIPS',
                'mipsle': '32-bit MIPS (LE)',
                'mips64': '64-bit MIPS',
                'mips64le': '64-bit MIPS (LE)',
                'riscv64': '64-bit RISC-V',
                's390x': '64-bit z/Architecture',
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
        };

        $scope.toggleUnits = function () {
            $scope.metricRates = !$scope.metricRates;
            try {
                window.localStorage["metricRates"] = $scope.metricRates;
            } catch (exception) { }
        };

        $scope.sizeOf = function (dict) {
            if (dict === undefined) {
                return 0;
            }
            return Object.keys(dict).length;
        };

        $scope.dismissNotification = function (id) {
            var idx = $scope.config.options.unackedNotificationIDs.indexOf(id);
            if (idx > -1) {
                $scope.config.options.unackedNotificationIDs.splice(idx, 1);
                $scope.saveConfig();
            }
        };

        $scope.abbreviatedError = function (addr) {
            var status = $scope.system.lastDialStatus[addr];
            if (!status || !status.error) {
                return null;
            }
            var time = $filter('date')(status.when, "HH:mm:ss")
            var err = status.error.replace(/.+: /, '');
            return err + " (" + time + ")";
        }

        $scope.setCrashReportingEnabled = function (enabled) {
            $scope.config.options.crashReportingEnabled = enabled;
            $scope.saveConfig();
        };

        $scope.isUnixAddress = function (address) {
            return address != null &&
                (address.indexOf('/') == 0 ||
                    address.indexOf('unix://') == 0 ||
                    address.indexOf('unixs://') == 0);
        }

    })
    .directive('shareTemplate', function () {
        return {
            templateUrl: 'syncthing/core/editShareTemplate.html',
            scope: {
                selected: '=',
                encryptionPasswords: '=',
                id: '@',
                label: '@',
                folderType: '@',
                untrusted: '=',
            },
            link: function(scope, elem, attrs) {
                var plain = false;
                scope.togglePasswordVisibility = function() {
                    scope.plain = !scope.plain;
                };
            },
        }
    });
