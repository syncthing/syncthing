angular.module('syncthing.core')
    .service('PushNotifications', ['Events', function (Events) {
        'use strict';

        var self = this;

        function checkNotificationPromise() {
            try {
                Notification.requestPermission().then();
            } catch (e) {
                return false;
            }

            return true;
        }

        function detectLocalStorage() {
            // Feature detect localStorage; https://mathiasbynens.be/notes/localstorage-pattern
            try {
                var uid = new Date();
                var storage = window.localStorage;
                storage.setItem(uid, uid);
                storage.removeItem(uid);
                return storage;
            } catch (exception) {
                return undefined;
            }
        }

        var _localStorage = detectLocalStorage();
        var _SYNPN = "SYN_PN"; // const key for localStorage

        angular.extend(self, {
            isEnabled: function () {
                return  self.isSupported() &&_localStorage && _localStorage[_SYNPN] === 'true'
            },
            setEnabled: function (enabled) {
                if (!_localStorage || !self.isSupported()) return;

                var showWelcomeNotification = false;
                if (enabled && !self.isEnabled())
                    showWelcomeNotification = true;

                _localStorage[_SYNPN] = enabled;

                if (showWelcomeNotification) self.notify('Notifications will be shown like this')
            },
            isSupported: function() {
              return "Notification" in window && Notification.permission !== 'denied';
            },
            checkPermission: function () {
                function handlePermission(permission) {
                    // Whatever the user answers, we make sure Chrome stores the information
                    if (!('permission' in Notification)) {
                        Notification.permission = permission;
                    }
                }

                return new Promise(function (resolve) {
                    // Let's check if the browser supports notifications and user has enabled them
                   if (!self.isEnabled() || !self.isSupported()) {
                        resolve(false)
                    } else {
                        // Check if browser uses Promises or callbacks (Safari)
                        if (checkNotificationPromise()) {
                            Notification.requestPermission()
                                .then((permission) => {
                                    handlePermission(permission);
                                    resolve(permission === 'granted');
                                })
                        } else {
                            Notification.requestPermission(function (permission) {
                                handlePermission(permission);
                                resolve(permission === 'granted');
                            });
                        }
                    }
                })
            },
            notify: function (message) {
                self.checkPermission().then(function (canNotify) {
                    if (canNotify) new Notification('Syncthing', {body: message});
                })
            }
        })
    }]);