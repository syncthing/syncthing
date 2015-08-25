// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.


/*jslint browser: true, continue: true, plusplus: true */
/*global $: false, angular: false, console: false, validLangs: false */

var syncthing = angular.module('syncthing', [
    'angularUtils.directives.dirPagination',
    'pascalprecht.translate',

    'syncthing.core',
    'syncthing.device',
    'syncthing.folder',
    'syncthing.settings',
    'syncthing.transfer',
    'syncthing.usagereport'
]);

var urlbase = 'rest';

syncthing.config(function ($httpProvider, $translateProvider, LocaleServiceProvider) {
    $httpProvider.interceptors.push(function xHeadersResponseInterceptor() {
        var deviceId = null;

        return {
            response: function onResponse(response) {
                var headers = response.headers();
                var responseVersion;
                var deviceIdShort;

                // angular template cache sends no headers
                if(Object.keys(headers).length === 0) {
                    return response;
                }

                if (!deviceId) {
                    deviceId = headers['x-syncthing-id'];
                    if (deviceId) {
                        deviceIdShort = deviceId.substring(0, 5);
                        $httpProvider.defaults.xsrfHeaderName = 'X-CSRF-Token-' + deviceIdShort;
                        $httpProvider.defaults.xsrfCookieName = 'CSRF-Token-' + deviceIdShort;
                    }
                }

                return response;
            }
        };
    });

    // language and localisation

    $translateProvider.useStaticFilesLoader({
        prefix: 'assets/lang/lang-',
        suffix: '.json'
    });

    LocaleServiceProvider.setAvailableLocales(validLangs);
    LocaleServiceProvider.setDefaultLocale('en');

});

// @TODO: extract global level functions into separate service(s)

function deviceCompare(a, b) {
    if (typeof a.name !== 'undefined' && typeof b.name !== 'undefined') {
        if (a.name < b.name)
            return -1;
        return a.name > b.name;
    }
    if (a.deviceID < b.deviceID) {
        return -1;
    }
    return a.deviceID > b.deviceID;
}

function folderCompare(a, b) {
    if (a.id < b.id) {
        return -1;
    }
    return a.id > b.id;
}

function folderMap(l) {
    var m = {};
    l.forEach(function (r) {
        m[r.id] = r;
    });
    return m;
}

function folderList(m) {
    var l = [];
    for (var id in m) {
        l.push(m[id]);
    }
    l.sort(folderCompare);
    return l;
}

function decimals(val, num) {
    var digits, decs;

    if (val === 0) {
        return 0;
    }

    digits = Math.floor(Math.log(Math.abs(val)) / Math.log(10));
    decs = Math.max(0, num - digits);
    return decs;
}

function randomString(len) {
    var i, result = '', chars = '01234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-';
    for (i = 0; i < len; i++) {
        result += chars[Math.round(Math.random() * (chars.length - 1))];
    }
    return result;
}

function isEmptyObject(obj) {
    var name;
    for (name in obj) {
        return false;
    }
    return true;
}

function debounce(func, wait) {
    var timeout, args, context, timestamp, result, again;

    var later = function () {
        var last = Date.now() - timestamp;
        if (last < wait) {
            timeout = setTimeout(later, wait - last);
        } else {
            timeout = null;
            if (again) {
                again = false;
                result = func.apply(context, args);
                context = args = null;
            }
        }
    };

    return function () {
        context = this;
        args = arguments;
        timestamp = Date.now();
        var callNow = !timeout;
        if (!timeout) {
            timeout = setTimeout(later, wait);
            result = func.apply(context, args);
            context = args = null;
        } else {
            again = true;
        }

        return result;
    };
}
