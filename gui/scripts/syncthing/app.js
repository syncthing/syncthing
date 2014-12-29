// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

/*jslint browser: true, continue: true, plusplus: true */
/*global $: false, angular: false, console: false, validLangs: false */

var syncthing = angular.module('syncthing', [
    'pascalprecht.translate',

    'syncthing.core'
]);

var urlbase = 'rest';
var guiVersion = null;

syncthing.config(function ($httpProvider, $translateProvider, LocaleServiceProvider) {
    $httpProvider.defaults.xsrfHeaderName = 'X-CSRF-Token';
    $httpProvider.defaults.xsrfCookieName = 'CSRF-Token';
    $httpProvider.interceptors.push(function () {
        return {
            response: function (response) {
                var responseVersion = response.headers()['x-syncthing-version'];
                if (!guiVersion) {
                    guiVersion = responseVersion;
                } else if (guiVersion != responseVersion) {
                    document.location.reload(true);
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

// @TODO: extract global level functions into seperate service(s)

function deviceCompare(a, b) {
    if (typeof a.Name !== 'undefined' && typeof b.Name !== 'undefined') {
        if (a.Name < b.Name)
            return -1;
        return a.Name > b.Name;
    }
    if (a.DeviceID < b.DeviceID) {
        return -1;
    }
    return a.DeviceID > b.DeviceID;
}

function folderCompare(a, b) {
    if (a.ID < b.ID) {
        return -1;
    }
    return a.ID > b.ID;
}

function folderMap(l) {
    var m = {};
    l.forEach(function (r) {
        m[r.ID] = r;
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

