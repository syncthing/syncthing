angular.module('syncthing.core')
    .service('PushNotifications', function() {
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

        angular.extend(self, {
            enabled: null,
            isEnabled: function () {
                return self.isSupported() && self.enabled;
            },
            setEnabled: function (newEnabled) {
                if(!self.isSupported()) return;

                console.log('setNotifications', newEnabled);

                if (newEnabled && self.enabled === false)
                    self.notify('Notifications will be shown like this');

                self.enabled = newEnabled;
            },
            isSupported: function () {
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
    });