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
        var authenticated = !!window.metadata;

        function initController() {
            LocaleService.autoConfigLocale();

            if (!authenticated) {
                // Can't proceed yet - wait for the page reload after successful login.
                return;
            }

            setInterval($scope.refresh, 10000);
            Events.start();
        }

        // public/scope definitions

        $scope.authenticated = authenticated;
        $scope.login = {
            username: '',
            password: '',
            errors: {},
        };
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
        $scope.needed = {};
        $scope.neededFolder = '';
        $scope.failed = {};
        $scope.localChanged = {};
        $scope.scanProgress = {};
        $scope.themes = [];
        $scope.globalChangeEvents = {};
        $scope.metricRates = false;
        $scope.folderPathErrors = {};
        $scope.currentSharing = {};
        $scope.currentFolder = {};
        $scope.currentDevice = {};
        $scope.ignores = {
            text: '',
            error: null,
            disabled: false,
            originalLines: [],
            defaultLines: [],
            saved: false,
        };
        $scope.webauthn = {
            errors: {},
            request: false,
        };
        resetRemoteNeed();

        try {
            $scope.metricRates = (window.localStorage["metricRates"] == "true");
        } catch (exception) { }

        $scope.versioningDefaults = {
            selector: "none",
            trashcanClean: 0,
            cleanupIntervalS: 3600,
            simpleKeep: 5,
            staggeredMaxAge: 365,
            externalCommand: "",
        };

        $scope.localStateTotal = {
            bytes: 0,
            directories: 0,
            files: 0
        };

        $scope.authenticatePassword = function () {
            $scope.login.inProgress = true;
            $scope.login.errors = {};
            $http.post(authnUrlbase + '/password', {
              username: $scope.login.username,
              password: $scope.login.password,
            }).success(function () {
                location.reload();
            }).catch(function (response) {
                if (response.status === 403) {
                    $scope.login.errors.badLogin = true;
                } else {
                    $scope.login.errors.failed = true;
                    console.log('Password authentication failed:', response);
                }
            }).finally(function () {
                $scope.login.inProgress = false;
            });
        };

        $scope.getLocationHost = function () {
            return $location.host();
        };

        $scope.isLocationInsecure = function() {
            return $location.protocol() !== 'https';
        };

        var ipv4Pattern = /^([0-9]{1,3}\.){3}[0-9]{1,3}(:.*)?$/;
        var ipv6Pattern = /^\[[0-9a-fA-F:]+\](:.*)?/;
        $scope.isRawIpAddress = function (host) {
            var h = host || $location.host();
            return h.match(ipv4Pattern) !== null || h.match(ipv6Pattern) !== null;
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

            refreshDeviceStats();
            refreshFolderStats();
            refreshGlobalChanges();
            refreshThemes();

            $q.all([
                refreshSystem(),
                refreshDiscoveryCache(),
                refreshConfig(),
                refreshCluster(),
                refreshConnectionStats(),
            ]).then(function() {
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
            }).catch($scope.emitHTTPError);
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
            // We sometimes get arg == null from angularjs - no idea why
            if (!restarting && arg) {
                if (arg.status === 0) {
                    // A network error, not an HTTP error
                    $scope.$emit(Events.OFFLINE);
                } else if (arg.status === 403) {
                    // Auth error - reload login page
                    location.reload();
                } else if (arg.status >= 400 && arg.status <= 599 && arg.status != 501) {
                    // A genuine HTTP error. 501/NotImplemented is considered intentional
                    // and not an error which we need to act upon.
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
            if (!$scope.connections[arg.data.id]) {
                return;
            }
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

        $scope.$on(Events.PENDING_DEVICES_CHANGED, function (event, arg) {
            if (!(arg.data.added || arg.data.removed)) {
                // Not enough information to update in place, just refresh it completely
                refreshCluster();
                return;
            }

            if (arg.data.added) {
                arg.data.added.forEach(function (rejected) {
                    var pendingDevice = {
                        time: arg.time,
                        name: rejected.name,
                        address: rejected.address
                    };
                    console.log("rejected device:", rejected.deviceID, pendingDevice);
                    $scope.pendingDevices[rejected.deviceID] = pendingDevice;
                });
            }

            if (arg.data.removed) {
                arg.data.removed.forEach(function (dev) {
                    console.log("no longer pending device:", dev.deviceID);
                    delete $scope.pendingDevices[dev.deviceID];
                });
            }
        });

        $scope.$on(Events.PENDING_FOLDERS_CHANGED, function (event, arg) {
            if (!(arg.data.added || arg.data.removed)) {
                // Not enough information to update in place, just refresh it completely
                refreshCluster();
                return;
            }

            if (arg.data.added) {
                arg.data.added.forEach(function (rejected) {
                    var offeringDevice = {
                        time: arg.time,
                        label: rejected.folderLabel,
                        receiveEncrypted: rejected.receiveEncrypted,
                    };
                    console.log("rejected folder", rejected.folderID, "from device:", rejected.deviceID, offeringDevice);

                    var pendingFolder = $scope.pendingFolders[rejected.folderID];
                    if (pendingFolder === undefined) {
                        pendingFolder = {
                            offeredBy: {}
                        };
                    }
                    pendingFolder.offeredBy[rejected.deviceID] = offeringDevice;
                    $scope.pendingFolders[rejected.folderID] = pendingFolder;
                });
            }

            if (arg.data.removed) {
                arg.data.removed.forEach(function (folderDev) {
                    console.log("no longer pending folder", folderDev.folderID, "from device:", folderDev.deviceID);
                    if (folderDev.deviceID === undefined) {
                        delete $scope.pendingFolders[folderDev.folderID];
                    } else if ($scope.pendingFolders[folderDev.folderID]) {
                        delete $scope.pendingFolders[folderDev.folderID].offeredBy[folderDev.deviceID];
                    }
                });
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
            if (!$scope.model[arg.data.folder]) {
                console.log("Dropping folder errors event for unknown folder", arg.data.folder)
                return;
            }
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

        // May be called through .error with the presented arguments, or through
        // .catch with the http response object containing the same arguments.
        $scope.emitHTTPError = function (data, status, headers, config) {
            var out = data;
            if (data && !data.data) {
                out = { data: data, status: status, headers: headers, config: config };
            }
            $scope.$emit('HTTPError', out);
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

            refreshNoAuthWarning();
            setDefaultTheme();

            if (!hasConfig) {
                $scope.$emit('ConfigLoaded');
            }
        }

        function refreshSystem() {
            return $http.get(urlbase + '/system/status').success(function (data) {
                $scope.myID = data.myID;
                $scope.system = data;

                if ($scope.reportDataPreviewVersion === '') {
                    $scope.reportDataPreviewVersion = $scope.system.urVersionMax;
                }

                var listenersFailed = [];
                var listenersRunning = [];
                for (var address in data.connectionServiceStatus) {
                    if (data.connectionServiceStatus[address].error) {
                        listenersFailed.push(address + ": " + data.connectionServiceStatus[address].error);
                    } else {
                        listenersRunning.push(address);
                    }
                }
                $scope.listenersFailed = listenersFailed;
                $scope.listenersRunning = listenersRunning;
                $scope.listenersTotal = $scope.sizeOf(data.connectionServiceStatus);

                var discoveryFailed = [];
                var discoveryRunning = [];
                for (var disco in data.discoveryStatus) {
                    if (data.discoveryStatus[disco] && data.discoveryStatus[disco].error) {
                        discoveryFailed.push(disco + ": " + data.discoveryStatus[disco].error);
                    } else {
                        discoveryRunning.push(disco);
                    }
                }
                $scope.discoveryFailed = discoveryFailed;
                $scope.discoveryRunning = discoveryRunning;
                $scope.discoveryTotal = $scope.sizeOf(data.discoveryStatus);

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

            if ((guiCfg.user && guiCfg.password) || guiCfg.authMode === 'ldap') {
                $scope.dismissNotification('authenticationUserAndPassword');
            }
        }

        function refreshCluster() {
            return $q.all([
                $http.get(urlbase + '/cluster/pending/devices').success(function (data) {
                    $scope.pendingDevices = data;
                    console.log("refreshCluster devices", data);
                }).error($scope.emitHTTPError),
                $http.get(urlbase + '/cluster/pending/folders').success(function (data) {
                    $scope.pendingFolders = data;
                    console.log("refreshCluster folders", data);
                }).error($scope.emitHTTPError),
            ]);
        }

        function refreshDiscoveryCache() {
            return $http.get(urlbase + '/system/discovery').success(function (data) {
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

            if (needed == 0 && deletes + items > 0 ) {
                // We don't need any data, but we have deletes or
                // dirs/links/empty files that we need to do. Drop down the
                // completion percentage to indicate that we have stuff to do.
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
            }).error(function (data, status, headers, config) {
                if (status === 404) {
                    console.log("refreshCompletion:", data);
                } else {
                    $scope.emitHTTPError(data, status, headers, config);
                }
            });
        }

        function refreshConnectionStats() {
            return $http.get(urlbase + '/system/connections').success(function (data) {
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
            return $q.all([
                $http.get(urlbase + '/config').success(function (data) {
                    updateLocalConfig(data);
                    console.log("refreshConfig", data);
                }),
                $http.get(urlbase + '/config/insync').success(function (data) {
                    $scope.configInSync = data.configInSync;
                }),
            ]);
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
        };

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
            return $scope.config.defaults.folder.path && $scope.folderEditor.folderPath.$pristine && $scope.editingFolderNew();
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

        function saveIgnores(ignores) {
            return $http.post(urlbase + '/db/ignores?folder=' + encodeURIComponent($scope.currentFolder.id), {
                ignore: ignores
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
            $scope.currentSharing.shared.sort(deviceCompare);
            $scope.currentSharing.unrelated = $scope.deviceList().filter(function (n) {
                return n.deviceID !== $scope.myID && !$scope.currentSharing.selected[n.deviceID];
            });
        }

        $scope.pendingIsRemoteEncrypted = function (folderID, deviceID) {
            var pending = $scope.pendingFolders[folderID];
            if (!pending || !pending.offeredBy || !pending.offeredBy[deviceID]) {
                return false;
            }
            return pending.offeredBy[deviceID].remoteEncrypted;
        };

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
                    if ($scope.deviceStats[device].lastSeen.toISOString() !== '1970-01-01T00:00:00.000Z') {
                        $scope.deviceStats[device].lastSeenDays = (new Date() - $scope.deviceStats[device].lastSeen) / 1000 / 86400;
                    }
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
                if (folderCfg.type === "receiveonly") {
                    return 'localadditions';
                }
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
                res.push('' + days + 'd');
                seconds = seconds % 86400;
            }

            // Separate out the number of hours.
            var hours = 0;
            if (seconds > 3600) {
                hours = Math.floor(seconds / 3600);
                res.push('' + hours + 'h');
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
            var unused = $scope.deviceFolders(deviceCfg).length === 0;

            if (unused) {
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
            if (!unused && $scope.deviceStats[deviceCfg.deviceID].lastSeenDays && $scope.deviceStats[deviceCfg.deviceID].lastSeenDays >= 7) {
                return status + 'disconnected-inactive';
            } else {
                return status + 'disconnected';
            }
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

        $scope.rdConnType = function (deviceID) {
            var conn = $scope.connections[deviceID];
            if (!conn) return "-1";
            var type = "disconnected";
            if (conn.type.indexOf('relay') === 0) type = "relay";
            else if (conn.type.indexOf('quic') === 0) type = "quic";
            else if (conn.type.indexOf('tcp') === 0) type = "tcp";
            else return type;

            if (conn.isLocal) type += "lan";
            else type += "wan";
            return type;
        }

        $scope.rdConnTypeString = function (type) {
            switch (type) {
                case "relaywan":
                    return $translate.instant('Relay WAN');
                case "relaylan":
                    return $translate.instant('Relay LAN');
                case "quicwan":
                    return $translate.instant('QUIC WAN');
                case "quiclan":
                    return $translate.instant('QUIC LAN');
                case "tcpwan":
                    return $translate.instant('TCP WAN');
                case "tcplan":
                    return $translate.instant('TCP LAN');
                default:
                    return $translate.instant('Disconnected');
            }
        }

        $scope.rdConnTypeIcon = function (type) {
            switch (type) {
            case "tcplan":
            case "quiclan":
                return "reception-4";
            case "tcpwan":
            case "quicwan":
                return "reception-3";
            case "relaylan":
                return "reception-2";
            case "relaywan":
                return "reception-1";
            case "disconnected":
                return "reception-0";
            }
        }

        $scope.rdConnDetails = function (type) {
            switch (type) {
                case "relaylan":
                case "relaywan":
                    return $translate.instant('Connections via relays might be rate limited by the relay');
                case "quiclan":
                case "quicwan":
                    return $translate.instant('QUIC connections are in most cases considered suboptimal');
                case "tcpwan":
                    return $translate.instant('Using a direct TCP connection over WAN');
                case "tcplan":
                    return $translate.instant('Using a direct TCP connection over LAN');
                default:
                    return $translate.instant('Unknown');
            }
        }

        $scope.hasRemoteGUIAddress = function (deviceCfg) {
            if (!deviceCfg.remoteGUIPort)
                return false;
            var conn = $scope.connections[deviceCfg.deviceID];
            return conn && conn.connected && conn.address && conn.type.indexOf('Relay') == -1;
        };

        $scope.remoteGUIAddress = function (deviceCfg) {
            // Assume hasRemoteGUIAddress is true or we would not be here
            var conn = $scope.connections[deviceCfg.deviceID];
            // Use regex to filter out scope ID from IPv6 addresses.
            return 'http://' + replaceAddressPort(conn.address, deviceCfg.remoteGUIPort).replace('%.*?\]:', ']:');
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
            if (typeof deviceCfg === 'undefined') {
                return "";
            }
            if (deviceCfg.name) {
                return deviceCfg.name;
            }
            return $scope.deviceShortID(deviceCfg.deviceID);
        };

        $scope.deviceShortID = function (deviceID) {
            if (typeof deviceID === 'undefined') {
                return "";
            }
            return deviceID.substr(0, 6);
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

        $scope.showDeviceIdentification = function (deviceCfg) {
            $scope.currentDevice = deviceCfg;
            $('#idqr').modal();
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
                return $scope.saveConfig();
            }
            return $q.when();
        };

        $scope.showListenerStatus = function () {
            var params = {
                type: 'listeners',
            };
            if ($scope.listenersFailed.length > 0) {
                params.status = 'danger';
                params.heading = $translate.instant("Listener Failures");
            } else {
                params.status = 'default';
                params.heading = $translate.instant("Listener Status");
            }
            $scope.connectivityStatusParams = params;
            $('#connectivity-status').modal();
        };

        $scope.showDiscoveryStatus = function () {
            var params = {
                type: 'discovery',
            };
            if ($scope.discoveryFailed.length > 0) {
                params.status = 'danger';
                params.heading = $translate.instant("Discovery Failures");
            } else {
                params.status = 'default';
                params.heading = $translate.instant("Discovery Status");
            }
            $scope.connectivityStatusParams = params;
            $('#connectivity-status').modal();
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

        $scope.about = {
            paths: {},
            refreshPaths: function () {
                $http.get(urlbase + '/system/paths').success(function (data) {
                    $scope.about.paths = data;
                }).error($scope.emitHTTPError);
            },
            show: function () {
                $scope.about.refreshPaths();
                $('#about').modal("show");
            },
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

          $scope.webauthn.errors = {};
        };

        $scope.webauthnAvailable = function () {
            return window['PublicKeyCredential'] !== undefined;
        };

        if ($scope.webauthnAvailable()) {
            if (authenticated) {
                // Functions for use in the settings dialog

                $scope.registerWebauthnCredential = function () {
                    $scope.webauthn.errors = {};
                    $http.post(urlbase + '/config/webauthn/register-start')
                        .then(function (resp) {
                            // Set excludeCredentials in frontend instead of backend so we can be consistent with UI state
                            resp.data.publicKey.excludeCredentials = $scope.tmpGUI.webauthnCredentials.map(function (cred) {
                              return { type: "public-key", id: cred.id };
                            });
                          return webauthnJSON.create(resp.data);
                        })
                        .then(function (pkc) {
                            return $http.post(urlbase + '/config/webauthn/register-finish', pkc);
                        })
                        .then(function (resp) {
                            $scope.tmpGUI.webauthnCredentials.push(resp.data);
                        })
                        .catch(function (e) {
                            if (e instanceof DOMException && e.code === DOMException.INVALID_STATE_ERR) {
                                $scope.webauthn.errors.alreadyRegistered = true;
                            } else if (e instanceof DOMException && e.code === DOMException.ABORT_ERR) {
                              $scope.webauthn.errors.aborted = true;
                            } else if (e instanceof DOMException && e.name === "NotAllowedError") {
                              $scope.webauthn.errors.notAllowed = true;
                            } else {
                                $scope.webauthn.errors.registrationFailed = true;
                                console.log('Credential creation failed:', e);
                            }
                        });
                };

                $scope.deleteWebauthnCredential = function (cfg, cred) {
                    // Array.filter is fine since WebAuthn only works in modern browsers
                    cfg.webauthnCredentials = cfg.webauthnCredentials.filter(function (cr) {
                        return cr.id !== cred.id;
                    });
                };

                $scope.webauthnRpIdChanged = function (fromCfg, toCfg) {
                    return fromCfg && toCfg
                        && fromCfg.webauthnRpId !== toCfg.webauthnRpId
                        && !(
                            (fromCfg.webauthnRpId === 'localhost' && toCfg.webauthnRpId === '')
                            || (fromCfg.webauthnRpId === '' && toCfg.webauthnRpId === 'localhost')
                        );
                };

                $scope.webauthnRpIdMatchesLocation = function (cfg) {
                    return cfg && ($location.host() === (cfg.webauthnRpId || 'localhost') || $location.host().endsWith('.' + cfg.webauthnRpId));
                };

                $scope.webauthnOriginChanged = function (fromCfg, toCfg) {
                    return fromCfg && toCfg
                        && fromCfg.webauthnOrigin !== toCfg.webauthnOrigin
                        && !(
                            (fromCfg.webauthnOrigin === $scope.getDefaultWebauthnOrigin(fromCfg) && toCfg.webauthnOrigin === '')
                            || (fromCfg.webauthnOrigin === '' && toCfg.webauthnOrigin === $scope.getDefaultWebauthnOrigin(toCfg))
                        );
                };

                $scope.getDefaultWebauthnOrigin = function (cfg) {
                    if (cfg) {
                        var splits = (cfg.address || '').split(':');
                        var port = '';
                        if (splits.length > 0) {
                            port = splits[splits.length - 1];
                        }
                        return 'https://' + (cfg.webauthnRpId || 'localhost') + (port ? ':' : '') + port;
                    } else {
                        return '';
                    }
                };

                $scope.locationMatchesWebauthnOrigin = function (cfg) {
                    return cfg && $location.absUrl().startsWith(cfg.webauthnOrigin || $scope.getDefaultWebauthnOrigin(cfg));
                };

                $scope.reloadAtWebauthnAddress = function (save, cfg) {
                    (save
                        ? $scope.saveSettings()
                        : Promise.resolve()
                    ).then(function () {
                        location.assign(cfg.webauthnOrigin || $scope.getDefaultWebauthnOrigin(cfg));
                    });
                };

            } else {
                // Functions for use on the login page

                $scope.authenticateWebauthnStart = function () {
                    $scope.webauthn.errors = {};
                    return $http.post(authnUrlbase + '/webauthn-start')
                        .then(function (resp) {
                            if (resp && resp.data && resp.data.publicKey) {
                                $scope.webauthn.request = resp.data;
                                return resp.data;
                            } else {
                                return Promise.reject('noCredentials');
                            }
                        })
                        .catch(function (e) {
                            if (e === 'noCredentials') {
                                $scope.webauthn.errors.noCredentials = true;
                            } else {
                                $scope.webauthn.errors.initFailed = true;
                                console.log('WebAuthn initialization failed:', e);
                            }

                            // Re-reject the Promise, otherwise consumers of the Promise will consider it succeeded
                            return Promise.reject(e);
                        });
                };

                $scope.authenticateWebauthnFinish = function () {
                    var finish = function (request) {
                        return webauthnJSON.get(request)
                            .then(function (pkc) {
                                return $http.post(authnUrlbase + '/webauthn-finish', pkc);
                            })
                            .then(function () {
                                location.reload();
                            })
                            .catch(function (e) {
                                if (e instanceof DOMException && e.code === DOMException.INVALID_STATE_ERR) {
                                    $scope.webauthn.errors.notRegistered = true;
                                } else if (e instanceof DOMException && e.code === DOMException.ABORT_ERR) {
                                    $scope.webauthn.errors.aborted = true;
                                } else if (e instanceof DOMException && e.name === "NotAllowedError") {
                                    $scope.webauthn.errors.notAllowed = true;
                                } else {
                                    $scope.webauthn.errors.authenticationFailed = true;
                                    console.log('WebAuthn authentication failed:', e);
                                }

                                $scope.webauthn.request = false;
                            });
                    };

                    $scope.webauthn.errors = {};
                    if ($scope.webauthn.request) {
                        finish($scope.webauthn.request);
                    } else {
                        $scope.authenticateWebauthnStart().then(finish);
                    }
                };

                $scope.locationDoesNotMatchWebauthnRpId = function () {
                    if ($scope.webauthn.request && $scope.webauthn.request.publicKey.rpId) {
                        var exactMatch = $location.host() === $scope.webauthn.request.publicKey.rpId;
                        var subdomainMatch = $location.host().endsWith('.' + $scope.webauthn.request.publicKey.rpId);
                        return !(exactMatch || subdomainMatch);
                    }
                    // If we don't know, don't show an error message.
                    return false;
                };

                $scope.inferWebauthnAddress = function () {
                    // This isn't guaranteed to match the "WebAuthn Origin" config setting,
                    // but it's the best we can do with the public information (only the rpId property in the WebAuthn parameter object).
                    // The exact WebAuthn Origin setting is a private security property, so we should not disclose it without authentication.

                    if (!($scope.webauthn.request && $scope.webauthn.request.publicKey.rpId)) {
                        return false;
                    }

                    var portPart = $location.port() ? ':' + $location.port() : '';
                    return 'https://' + $scope.webauthn.request.publicKey.rpId + portPart;
                };

                $scope.reloadAtWebauthnAddress = function () {
                    var address = $scope.inferWebauthnAddress();
                    if (address) {
                        location.assign(address);
                    }
                };

                $scope.authenticateWebauthnStart();
            }
        }

        $scope.saveConfig = function () {
            var cfg = JSON.stringify($scope.config);
            var opts = {
                headers: {
                    'Content-Type': 'application/json'
                }
            };
            return $http.put(urlbase + '/config', cfg, opts).finally(refreshConfig).catch($scope.emitHTTPError);
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
            if ($scope.tmpGUI) {
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
            } else {
                return false;
            }
        };

        $scope.saveSettings = function () {
            var savePromise = null;

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

                savePromise = $scope.saveConfig().then(function () {
                    if (themeChanged) {
                        document.location.reload(true);
                    }
                });
            }

            $("#settings").off("hide.bs.modal").modal("hide");

          // Return a Promise so callers can wait for save completion
          return savePromise || Promise.resolve();
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
            if ($scope.editingDeviceDefaults()) {
                return $translate.instant("Edit Device Defaults");
            }
            var title = '';
            if ($scope.editingDeviceExisting()) {
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
            if ($scope.has(["existing", "defaults"], $scope.currentDevice._editing)) {
                return 'fas fa-pencil-alt';
            }
            return 'fas fa-desktop';
        };

        $scope.editingDeviceDefaults = function() {
            return $scope.currentDevice._editing == 'defaults';
        }

        $scope.editingDeviceExisting = function() {
            return $scope.currentDevice._editing == 'existing';
        }

        $scope.editingDeviceNew = function() {
            // The "new-pending" value is intentionally disregarded here.
            return $scope.currentDevice._editing == 'new';
        }

        $scope.editDeviceExisting = function (deviceCfg) {
            $scope.currentDevice = $.extend({}, deviceCfg);
            $scope.currentDevice._editing = "existing";
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
                $scope.currentDevice._editing = "defaults";
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
            $scope.discoveryUnknown = [];
            for (var id in $scope.discoveryCache) {
                if ($scope.discoveryUnknown.length === 100) {
                    break;
                }
                if (id in $scope.devices) {
                    continue
                }
                $scope.discoveryUnknown.push(id);
            }
            return $http.get(urlbase + '/config/defaults/device').then(function (p) {
                $scope.currentDevice = p.data;
                $scope.currentDevice.name = name;
                $scope.currentDevice.deviceID = deviceID;
                if (deviceID) {
                    $scope.currentDevice._editing = "new-pending";
                } else {
                    $scope.currentDevice._editing = "new";
                }
                initShareEditing('device');
                $scope.currentSharing.unrelated = $scope.folderList();
                editDeviceModal();
            }, $scope.emitHTTPError);
        };

        $scope.deleteDevice = function () {
            $('#editDevice').modal('hide');
            if ($scope.currentDevice._editing != "existing") {
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
            if ($scope.currentDevice._editing == "defaults") {
                $scope.config.defaults.device = $scope.currentDevice;
            } else {
                setDeviceConfig();
            }
            delete $scope.currentSharing;
            $scope.currentDevice = {};
            $scope.saveConfig();
        };

        function setDeviceConfig() {
            var currentID = $scope.currentDevice.deviceID;
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

        $scope.dismissPendingDevice = function (deviceID) {
            $http.delete(urlbase + '/cluster/pending/devices?device=' + encodeURIComponent(deviceID));
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
        };

        $scope.isAtleastOneDevicePausedStateSetTo = function (pause) {
            for (var id in $scope.devices) {
                if ($scope.devices[id].paused == pause) {
                    return true;
                }
            }

            return false;
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

        $scope.setFSWatcherIntervalDefault = function () {
            var defaultRescanIntervals = [60, 3600, 3600*24];
            if (defaultRescanIntervals.indexOf($scope.currentFolder.rescanIntervalS) === -1) {
                return;
            }
            var idx;
            if ($scope.currentFolder.type === 'receiveencrypted') {
                idx = 2;
            } else if ($scope.currentFolder.fsWatcherEnabled) {
                idx = 1;
            } else {
                idx = 0;
            }
            $scope.currentFolder.rescanIntervalS = defaultRescanIntervals[idx];
        };

        $scope.setDefaultsForFolderType = function () {
            if ($scope.currentFolder.type === 'receiveencrypted') {
                $scope.currentFolder.fsWatcherEnabled = false;
                $scope.currentFolder.ignorePerms = true;
                delete $scope.currentFolder.versioning;
            } else {
                $scope.currentFolder.fsWatcherEnabled = true;
            }
            $scope.setFSWatcherIntervalDefault();
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

        function editFolderModal(initialTab) {
            initVersioningEditing();
            $scope.currentFolder._recvEnc = $scope.currentFolder.type === 'receiveencrypted';
            $scope.folderPathErrors = {};
            $scope.folderEditor.$setPristine();
            if (!initialTab) {
                initialTab = "#folder-general";
            }
            $('.nav-tabs a[href="' + initialTab + '"]').tab('show');
            $('#editFolder').modal().one('shown.bs.tab', function (e) {
                if (e.target.attributes.href.value === "#folder-ignores") {
                    $('#folder-ignores textarea').focus();
                }
            }).one('hidden.bs.modal', function () {
                var p = $q.when();
                // If the modal was closed default patterns should still apply
                if ($scope.currentFolder._editing == "new-ignores" && !$scope.ignores.saved && $scope.ignores.defaultLines) {
                    p = saveFolderAddIgnores($scope.currentFolder.id, true);
                }
                p.then(function () {
                    window.location.hash = "";
                    $scope.currentFolder = {};
                    $scope.ignores = {};
                });
            });
        };

        $scope.editFolderModalTitle = function() {
            if ($scope.editingFolderDefaults()) {
                return $translate.instant("Edit Folder Defaults");
            }
            var title = '';
            switch ($scope.currentFolder._editing) {
            case "existing":
                title = $translate.instant("Edit Folder");
                break;
            case "new":
            case "new-pending":
                title = $translate.instant("Add Folder");
                break;
            case "new-ignores":
                title = $translate.instant("Set Ignores on Added Folder");
                break;
            }
            if ($scope.currentFolder.id !== '') {
                title += ' (' + $scope.folderLabel($scope.currentFolder.id) + ')';
            }
            return title;
        };

        $scope.editFolderModalIcon = function() {
            if ($scope.has(["existing", "defaults"], $scope.currentFolder._editing)) {
                return 'fas fa-pencil-alt';
            }
            return 'fas fa-folder';
        };

        $scope.editingFolderDefaults = function() {
            return $scope.currentFolder._editing == 'defaults';
        }

        $scope.editingFolderExisting = function() {
            return $scope.currentFolder._editing == 'existing';
        }

        $scope.editingFolderNew = function() {
            return $scope.has(['new', 'new-pending'], $scope.currentFolder._editing);
        }

        function editFolder(initialTab) {
            if ($scope.currentFolder.path.length > 1 && $scope.currentFolder.path.slice(-1) === $scope.system.pathSeparator) {
                $scope.currentFolder.path = $scope.currentFolder.path.slice(0, -1);
            } else if (!$scope.currentFolder.path) {
                // undefined path leads to invalid input field
                $scope.currentFolder.path = '';
            }
            initShareEditing('folder');
            editFolderModal(initialTab);
        }

        $scope.internalVersioningEnabled = function (guiVersioning) {
            if (!$scope.currentFolder._guiVersioning) {
                return false;
            }
            return ['none', 'external'].indexOf($scope.currentFolder._guiVersioning.selector) === -1;
        };

        function initVersioningEditing() {
            $scope.currentFolder._guiVersioning = angular.copy($scope.versioningDefaults);

            var currentVersioning = $scope.currentFolder.versioning;

            if (!currentVersioning || !currentVersioning.type || currentVersioning.type === 'none') {
                return;
            }

            $scope.currentFolder._guiVersioning.cleanupIntervalS = +currentVersioning.cleanupIntervalS;
            $scope.currentFolder._guiVersioning.selector = currentVersioning.type;

            // Apply parameters currently in use
            switch (currentVersioning.type) {
            case "trashcan":
                $scope.currentFolder._guiVersioning.trashcanClean = +currentVersioning.params.cleanoutDays;
                break;
            case "simple":
                $scope.currentFolder._guiVersioning.simpleKeep = +currentVersioning.params.keep;
                $scope.currentFolder._guiVersioning.trashcanClean = +currentVersioning.params.cleanoutDays;
                break;
            case "staggered":
                $scope.currentFolder._guiVersioning.staggeredMaxAge = Math.floor(+currentVersioning.params.maxAge / 86400);
                break;
            case "external":
                $scope.currentFolder._guiVersioning.externalCommand = currentVersioning.params.command;
                break;
            }
        };

        $scope.editFolderExisting = function (folderCfg, initialTab) {
            $scope.currentFolder = angular.copy(folderCfg);
            $scope.currentFolder._editing = "existing";
            editFolderLoadIgnores();
            editFolder(initialTab);
        };

        function editFolderLoadingIgnores() {
            $scope.ignores.text = 'Loading...';
            $scope.ignores.error = null;
            $scope.ignores.disabled = true;
        }

        function editFolderGetIgnores() {
            return $http.get(urlbase + '/db/ignores?folder=' + encodeURIComponent($scope.currentFolder.id))
                .then(function (r) {
                    return r.data;
                }, function (response) {
                    $scope.ignores.text = $translate.instant("Failed to load ignore patterns.");
                    return $q.reject(response);
            });
        };

        function editFolderLoadIgnores() {
            editFolderLoadingIgnores();
            return editFolderGetIgnores().then(function (data) {
                if (!data) {
                    return;
                }
                editFolderInitIgnores(data.ignore, data.error);
            }, $scope.emitHTTPError);
        }

        $scope.editFolderDefaults = function() {
            $q.all([
                $http.get(urlbase + '/config/defaults/folder').then(function (response) {
                    $scope.currentFolder = response.data;
                    $scope.currentFolder._editing = "defaults";
                }),
                getDefaultIgnores().then(editFolderInitIgnores),
            ]).then(editFolder, $scope.emitHTTPError);
        };

        function getDefaultIgnores() {
            return $http.get(urlbase + '/config/defaults/ignores').then(function (r) {
                return r.data.lines;
            });
        }

        function editFolderInitIgnores(lines, error) {
            $scope.ignores.originalLines = lines || [];
            setIgnoresText(lines);
            $scope.ignores.error = error;
            $scope.ignores.disabled = false;
        }

        function setIgnoresText(lines) {
            $scope.ignores.text = lines ? lines.join('\n') : "";
        }

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
                    $scope.currentFolder._editing = "new";
                    $scope.currentFolder.label = $scope.currentFolder.label;
                    editFolderModal();
                });
            });
        };

        $scope.addFolderAndShare = function (folderID, pendingFolder, device) {
            addFolderInit(folderID).then(function() {
                $scope.currentSharing.selected[device] = true;
                $scope.currentFolder.label = pendingFolder.offeredBy[device].label;
                for (var k in pendingFolder.offeredBy) {
                    if (pendingFolder.offeredBy[k].receiveEncrypted) {
                        $scope.currentFolder.type = "receiveencrypted";
                        $scope.setDefaultsForFolderType();
                        break;
                    }
                }
                $scope.currentFolder._editing = "new-pending";
                editFolderModal();
            });
        };

        function addFolderInit(folderID) {
            return $http.get(urlbase + '/config/defaults/folder').then(function (response) {
                $scope.currentFolder = response.data;
                $scope.currentFolder.id = folderID;
                initShareEditing('folder');
                $scope.currentSharing.unrelated = $scope.currentSharing.unrelated.concat($scope.currentSharing.shared);
                $scope.currentSharing.shared = [];
                // Ignores don't need to be initialized here, as that happens in
                // a second step if the user indicates in the creation modal
                // that they want to set ignores
            }, $scope.emitHTTPError);
        }

        $scope.shareFolderWithDevice = function (folder, device) {
            var folderCfg = $scope.folders[folder];
            if (folderCfg.type == "receiveencrypted" || !$scope.pendingIsRemoteEncrypted(folder, device)) {
                $scope.folders[folder].devices.push({
                    deviceID: device
                });
                $scope.config.folders = folderList($scope.folders);
                $scope.saveConfig();
            } else {
                // Open edit folder dialog to enter encryption password
                $scope.editFolderExisting(folderCfg, "#folder-sharing");
                $scope.currentSharing.selected[device] = true;
            }
        };

        $scope.saveFolder = function () {
            if ($scope.currentFolder._editing == "new-ignores") {
                // On modal being hidden without clicking save, the defaults will be saved.
                $scope.ignores.saved = true;
                saveFolderAddIgnores($scope.currentFolder.id);
                hideFolderModal();
                return;
            }

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

            if (!folderCfg.versioning) {
                folderCfg.versioning = {params: {}};
            }
            folderCfg.versioning.type = folderCfg._guiVersioning.selector;
            if ($scope.internalVersioningEnabled()) {
                folderCfg.versioning.cleanupIntervalS = folderCfg._guiVersioning.cleanupIntervalS;
            }
            switch (folderCfg._guiVersioning.selector) {
            case "trashcan":
                folderCfg.versioning.params.cleanoutDays = '' + folderCfg._guiVersioning.trashcanClean;
                break;
            case "simple":
                folderCfg.versioning.params.keep = '' + folderCfg._guiVersioning.simpleKeep,
                folderCfg.versioning.params.cleanoutDays = '' + folderCfg._guiVersioning.trashcanClean;
                break;
            case "staggered":
                folderCfg.versioning.params.maxAge = '' + (folderCfg._guiVersioning.staggeredMaxAge * 86400);
                break;
            case "external":
                folderCfg.versioning.params.command = '' + folderCfg._guiVersioning.externalCommand;
                break;
            default:
                folderCfg.versioning = {type: ''};
            }
            delete folderCfg._guiVersioning;

            if ($scope.currentFolder._editing == "defaults") {
                hideFolderModal();
                $scope.config.defaults.ignores.lines = ignoresArray();
                $scope.config.defaults.folder = folderCfg;
                $scope.saveConfig();
                return;
            }

            // This is a new folder where ignores should apply before it first starts.
            if ($scope.currentFolder._addIgnores) {
                folderCfg.paused = true;
            }
            $scope.folders[folderCfg.id] = folderCfg;
            $scope.config.folders = folderList($scope.folders);

            if ($scope.currentFolder._editing == "existing") {
                hideFolderModal();
                saveFolderIgnoresExisting();
                $scope.saveConfig();
                return;
            }

            // No ignores to be set on the new folder, save directly.
            if (!$scope.currentFolder._addIgnores) {
                hideFolderModal();
                $scope.saveConfig();
                return;
            }

            // Add folder (paused), load existing ignores and if there are none,
            // load default ignores, then let the user edit them.
            $scope.saveConfig().then(function() {
                editFolderLoadingIgnores();
                $scope.currentFolder._editing = "new-ignores";
                $('.nav-tabs a[href="#folder-ignores"]').tab('show');
                return editFolderGetIgnores();
            }).then(function (data) {
                // Error getting ignores -> leave error message.
                if (!data) {
                    return;
                }
                if ((data.ignore && data.ignore.length > 0) || data.error) {
                    editFolderInitIgnores(data.ignore, data.error);
                } else {
                    getDefaultIgnores().then(function (lines) {
                        setIgnoresText(lines);
                        $scope.ignores.defaultLines = lines;
                        $scope.ignores.disabled = false;
                    });
                }
            }, $scope.emitHTTPError);
        };

        function saveFolderIgnoresExisting() {
            if ($scope.ignores.disabled) {
                return;
            }
            var ignores = ignoresArray();

            function arrayDiffers(a, b) {
                return !a !== !b || a.length !== b.length || a.some(function (v, i) { return v !== b[i]; });
            }
            if (arrayDiffers(ignores, $scope.ignores.originalLines)) {
                return saveIgnores(ignores);
            };
        }

        function saveFolderAddIgnores(folderID, useDefault) {
            var ignores = useDefault ? $scope.ignores.defaultLines : ignoresArray();
            return saveIgnores(ignores).then(function () {
                return $scope.setFolderPause(folderID, $scope.currentFolder.paused);
            });
        };

        function ignoresArray() {
            var ignores = $scope.ignores.text.split('\n');
            // Split always returns a minimum 1-length array even for no patterns
            if (ignores.length === 1 && ignores[0] === "") {
                ignores = [];
            }
            return ignores;
        }

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

        $scope.dismissPendingFolder = function (folderID, deviceID) {
            $http.delete(urlbase + '/cluster/pending/folders?folder=' + encodeURIComponent(folderID)
                         + '&device=' + encodeURIComponent(deviceID));
        };

        $scope.deviceNameMarkRemoteState = function (deviceID, folderID) {
            var name = $scope.deviceName($scope.devices[deviceID]);
            // Add footnote if sharing was not accepted on the remote device
            if (deviceID in $scope.completion && folderID in $scope.completion[deviceID]) {
                if ($scope.completion[deviceID][folderID].remoteState == 'notSharing') {
                    name += '<sup>1</sup>';
                } else if ($scope.completion[deviceID][folderID].remoteState == 'paused') {
                    name += '<sup>2</sup>';
                }
            }
            return name;
        };

        $scope.sharesFolder = function (folderCfg) {
            var names = [];
            folderCfg.devices.forEach(function (device) {
                if (device.deviceID !== $scope.myID) {
                    names.push($scope.deviceNameMarkRemoteState(device.deviceID, folderCfg.id));
                }
            });
            names.sort();
            return names.join(", ");
        };

        $scope.folderHasUnacceptedDevices = function (folderCfg) {
            for (var deviceID in $scope.completion) {
                if (deviceID in $scope.devices
                    && folderCfg.id in $scope.completion[deviceID]
                    && $scope.completion[deviceID][folderCfg.id].remoteState == 'notSharing') {
                    return true;
                }
            }
            return false;
        };

        $scope.folderHasPausedDevices = function (folderCfg) {
            for (var deviceID in $scope.completion) {
                if (deviceID in $scope.devices
                    && folderCfg.id in $scope.completion[deviceID]
                    && $scope.completion[deviceID][folderCfg.id].remoteState == 'paused') {
                    return true;
                }
            }
            return false;
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

        $scope.folderLabelMarkRemoteState = function (folderID, deviceID) {
            var label = $scope.folderLabel(folderID);
            // Add footnote if sharing was not accepted on the remote device
            if (deviceID in $scope.completion && folderID in $scope.completion[deviceID]) {
                if ($scope.completion[deviceID][folderID].remoteState == 'notSharing') {
                    label += '<sup>1</sup>';
                } else if ($scope.completion[deviceID][folderID].remoteState == 'paused') {
                    label += '<sup>2</sup>';
                }
            }
            return label;
        };

        $scope.sharedFolders = function (deviceCfg) {
            var labels = [];
            $scope.deviceFolders(deviceCfg).forEach(function (folderID) {
                labels.push($scope.folderLabelMarkRemoteState(folderID, deviceCfg.deviceID));
            });
            return labels.join(', ');
        };

        $scope.deviceHasUnacceptedFolders = function (deviceCfg) {
            if (!(deviceCfg.deviceID in $scope.completion)) {
                return false;
            }
            for (var folderID in $scope.completion[deviceCfg.deviceID]) {
                if (folderID in $scope.folders
                    && $scope.completion[deviceCfg.deviceID][folderID].remoteState == 'notSharing') {
                    return true;
                }
            }
            return false;
        };

        $scope.deviceHasPausedFolders = function (deviceCfg) {
            if (!(deviceCfg.deviceID in $scope.completion)) {
                return false;
            }
            for (var folderID in $scope.completion[deviceCfg.deviceID]) {
                if (folderID in $scope.folders
                    && $scope.completion[deviceCfg.deviceID][folderID].remoteState == 'paused') {
                    return true;
                }
            }
            return false;
        };

        $scope.deleteFolder = function (id) {
            hideFolderModal();
            if ($scope.currentFolder._editing != "existing") {
                return;
            }

            delete $scope.folders[id];
            delete $scope.model[id];
            $scope.config.folders = folderList($scope.folders);
            recalcLocalStateTotal();

            $scope.saveConfig();
        };

        function hideFolderModal() {
            $('#editFolder').modal('hide');
        }

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
                                extensions: ["table", "filter", "glyph"],
                                quicksearch: true,
                                filter: {
                                    hideExpanders: true,
                                    mode: "hide"
                                },
                                glyph: {
                                    preset: "awesome5",
                                },
                                table: {
                                    indentation: 24,
                                },
                                strings: {
                                    loading: $translate.instant("Loading..."),
                                    loadError: $translate.instant("Failed to load file versions."),
                                    noData: $translate.instant("There are no file versions to restore.")
                                },
                                // Set to '1' to silence errors after pressing arrow keys on file nodes.
                                // Happens on the official option configuration from the developer's site
                                // too, so probably a bug?
                                debugLevel: 1,
                                source: buildTree($scope.restoreVersions.versions),
                                renderColumns: function (event, data) {
                                    // Case insensitive sort with folders on top.
                                    var cmp = function (a, b) {
                                        var x = (a.isFolder() ? "0" : "1") + a.title.toLowerCase(),
                                            y = (b.isFolder() ? "0" : "1") + b.title.toLowerCase();
                                        return x === y ? 0 : x > y ? 1 : -1;
                                    };
                                    data.tree.getRootNode().sortChildren(cmp, true);

                                    var node = data.node,
                                        $tdList = $(node.tr).find(">td"),
                                        template;
                                    if (node.folder) {
                                        template = '<div ng-include="\'syncthing/folder/restoreVersionsMassActions.html\'"/>';
                                    } else {
                                        template = '<div ng-include="\'syncthing/folder/restoreVersionsVersionSelector.html\'"/>';
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

                            var ranges = {};
                            ranges[$translate.instant("All Time")] = [minDate, maxDate];
                            ranges[$translate.instant("Today")] = [moment().startOf('day'), moment()];
                            ranges[$translate.instant("Yesterday")] = [moment().subtract(1, 'days').startOf('day'), moment().startOf('day')];
                            ranges[$translate.instant("Last 7 Days")] = [moment().subtract(6, 'days').startOf('day'), moment()];
                            ranges[$translate.instant("Last 30 Days")] = [moment().subtract(29, 'days').startOf('day'), moment()];
                            ranges[$translate.instant("This Month")] = [moment().startOf('month'), moment()];
                            ranges[$translate.instant("Last Month")] = [moment().subtract(1, 'month').startOf('month'), moment().startOf('month')];

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
                                opens: "left",
                                drops: "up",
                                startDate: minDate,
                                endDate: maxDate,
                                minDate: minDate,
                                maxDate: maxDate,
                                ranges: ranges,
                                locale: {
                                    applyLabel: $translate.instant("Apply"),
                                    cancelLabel: $translate.instant("Cancel"),
                                    customRangeLabel: $translate.instant("Custom Range"),
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
            if (!folderCfg || folderCfg.type !== ["receiveonly",  "receiveencrypted"].indexOf(folderCfg.type) === -1) {
                return false;
            }
            var counts = $scope.model[folderCfg.id];
            return counts && counts.receiveOnlyTotalItems > 0;
        };

        $scope.revertOverride = function () {
            $http.post(
                urlbase + "/db/" + $scope.revertOverrideParams.operation +"?folder="
                +encodeURIComponent($scope.revertOverrideParams.folderID));
        };

        $scope.revertOverrideConfirmationModal = function (type, folderID) {
            var params = {
                type: type,
                folderID: folderID,
            };
            switch (type) {
                case "override":
                    params.heading = $translate.instant("Override Changes");
                    params.icon = "fas fa-arrow-circle-up"
                    params.operation = "override";
                    break;
                case "revert":
                    params.heading = $translate.instant("Revert Local Changes");
                    params.icon = "fas fa-arrow-circle-down"
                    params.operation = "revert";
                    break;
                case "deleteEnc":
                    params.heading = $translate.instant("Delete Unexpected Items");
                    params.icon = "fas fa-minus-circle"
                    params.operation = "revert";
                    break;
            }
            $scope.revertOverrideParams = params;
            $('#revert-override-confirmation').modal('show');
        };

        $scope.advanced = function () {
            $scope.advancedConfig = angular.copy($scope.config);
            $scope.advancedConfig.devices.sort(deviceCompare);
            $scope.advancedConfig.folders.sort(folderCompare);
            $scope.advancedConfig.defaults.ignores._lines = function (newValue) {
                if (arguments.length) {
                    $scope.advancedConfig.defaults.ignores.lines = newValue.split('\n');
                }
                return $scope.advancedConfig.defaults.ignores.lines.join('\n');
            };
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

            if ($scope.version.container) {
                arch += " Container";
            }

            return $scope.version.version + ', ' + os + ' (' + arch + ')';
        };

        $scope.versionBase = function () {
            if (!$scope.version.version) {
                return '';
            }
            var version = $scope.version.version;
            var pos = version.indexOf('-');
            if (pos > 0) {
                version = version.slice(0, pos);
            }
            return version;
        };

        $scope.docsURL = function (path) {
            var url = 'https://docs.syncthing.net';
            if (!path) {
                // Undefined or null should become a valid string.
                path = '';
            }
            var hash = path.indexOf('#');
            if (hash != -1) {
                url += '/' + path.slice(0, hash);
                url += '?version=' + $scope.versionBase();
                url += path.slice(hash);
            } else {
                url += '/' + path;
                url += '?version=' + $scope.versionBase();
            }
            return url;
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
            var translation = $translate.instant("theme-name-" + theme);
            if (translation.indexOf("theme-name-") == 0) {
                // Fall back to simple Title Casing on missing translation
                translation = theme.toLowerCase().replace(/(?:^|\s)\S/g, function (a) {
                    return a.toUpperCase();
                });
            }
            return translation;
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

        $scope.has = function (array, element) {
            return array.indexOf(element) >= 0;
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
        };

        $scope.setCrashReportingEnabled = function (enabled) {
            $scope.config.options.crashReportingEnabled = enabled;
            $scope.saveConfig();
        };

        $scope.isUnixAddress = function (address) {
            return address != null &&
                (address.indexOf('/') == 0 ||
                    address.indexOf('unix://') == 0 ||
                    address.indexOf('unixs://') == 0);
        };

        $scope.shareDeviceIdDialog = function (method) {
            // This function can be used to share both user's own and remote
            // device IDs. Three sharing methods are used - copy to clipboard,
            // send by email, and send by SMS.
            var params = {
                method: method,
            };
            var deviceID = $scope.currentDevice.deviceID;
            var deviceName = $scope.deviceName($scope.currentDevice);

            // Title and footer can be reused between different sharing
            // methods, hence we define them separately before the body.
            var title = $translate.instant('Syncthing device ID for "{%devicename%}"', {devicename: deviceName});
            var footer = $translate.instant("Learn more at {%url%}", {url: "https://syncthing.net"});

            switch (method) {
                case "email":
                    params.heading = $translate.instant("Share by Email");
                    params.icon = "fa fa-envelope-o";
                    // Email message format requires using CRLF for line breaks.
                    // Ref: https://datatracker.ietf.org/doc/html/rfc5322
                    params.subject = title;
                    params.body = [
                        $translate.instant('To connect with the Syncthing device named "{%devicename%}", add a new remote device on your end with this ID:', {devicename: deviceName}),
                        deviceID,
                        $translate.instant("Syncthing is a continuous file synchronization program. It synchronizes files between two or more computers in real time, safely protected from prying eyes. Your data is your data alone and you deserve to choose where it is stored, whether it is shared with some third party, and how it's transmitted over the internet."),
                        footer
                    ].join('\r\n\r\n');
                    break;
                case "sms":
                    params.heading = $translate.instant("Share by SMS");
                    params.icon = "fa fa-comments-o";
                    // SMS is limited to 160 characters (non-Unicode), so we keep
                    // it as short as possible, e.g. by stripping hyphens from
                    // device ID. The current minimum length is around 140 chars,
                    // but some room is required for longer sharing device names.
                    params.body = [
                        title,
                        deviceID.replace(/-/g, ''),
                        footer
                    ].join('\n');
                    break;
            }

            $scope.shareDeviceIdParams = params;
            $('#share-device-id-dialog').modal('show');
        };

        $scope.shareDeviceId = function () {
            switch ($scope.shareDeviceIdParams.method) {
                case 'email':
                    location.href = 'mailto:?subject=' + encodeURIComponent($scope.shareDeviceIdParams.subject) + '&body=' + encodeURIComponent($scope.shareDeviceIdParams.body);
                    break;
                case 'sms':
                    // Ref1: https://rfc-editor.org/rfc/rfc5724
                    // Ref2: https://stackoverflow.com/questions/6480462/how-to-pre-populate-the-sms-body-text-via-an-html-link
                    location.href = 'sms:?&body=' + encodeURIComponent($scope.shareDeviceIdParams.body);
                    break;
            }
        }

        $scope.showTemporaryTooltip = function (event, tooltip) {
            // This function can be used to display a temporary tooltip above
            // the current element. This way, we can dynamically add a tooltip
            // with explanatory text after the user performs an interactive
            // operation, e.g. clicks a button. If the element already has a
            // tooltip, it will be saved first and then restored once the user
            // moves focus to a different element.
            var e = event.currentTarget;
            var oldTooltip = e.getAttribute('data-original-title');

            e.setAttribute('data-original-title', tooltip);
            $(e).tooltip('show');

            if (oldTooltip) {
                e.setAttribute('data-original-title', oldTooltip);
            } else {
                e.removeAttribute('data-original-title');
            }
        };

        $scope.copyToClipboard = function (event, content) {
            var success = $translate.instant("Copied!");
            var failure = $translate.instant("Copy failed! Try to select and copy manually.");
            var message = success;

            if (navigator.clipboard && navigator.clipboard.writeText) {
                // Default for modern browsers on localhost or HTTPS. Doesn't
                // work on unencrypted HTTP for security reasons.
                navigator.clipboard.writeText(content);
            } else if (window.clipboardData && window.clipboardData.setData) {
                // Fallback for Internet Explorer. Needs to go second before
                // "document.queryCommandSupported", as the browser supports the
                // other method too, yet it can often be disabled for security
                // reasons, causing the copy to fail. The IE-specific method is
                // more reliable.
                window.clipboardData.setData('Text', content);
            } else if (document.queryCommandSupported) {
                // Fallback for modern browsers on HTTP and non-IE old browsers.
                // Check for document.queryCommandSupported("copy") support is
                // omitted on purpose, as old Chrome versions reported "false"
                // despite supporting the feature. The position and opacity
                // hacks are needed to work inside Bootstrap modals.
                var e = event.currentTarget;
                var textarea = document.createElement("textarea");

                e.appendChild(textarea);
                textarea.style.position = "fixed";
                textarea.style.opacity = "0";
                textarea.textContent = content;
                textarea.select();

                try {
                    document.execCommand("copy");
                } catch (ex) {
                    message = failure;
                } finally {
                    e.removeChild(textarea);
                }
            } else {
                message = failure;
            }

            $scope.showTemporaryTooltip(event, message);
        };
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
            link: function (scope, elem, attrs) {
                var plain = false;
                scope.togglePasswordVisibility = function() {
                    scope.plain = !scope.plain;
                };
            },
        }
    });
