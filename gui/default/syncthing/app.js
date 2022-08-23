// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.


/*jslint browser: true, continue: true, plusplus: true */
/*global $: false, angular: false, console: false, validLangs: false */

var syncthing = angular.module('syncthing', [
    'angularUtils.directives.dirPagination',
    'pascalprecht.translate', 'ngSanitize',

    'syncthing.core'
]);

var urlbase = 'rest';

syncthing.config(function ($httpProvider, $translateProvider, LocaleServiceProvider) {
    var deviceIDShort = metadata.deviceID.substr(0, 5);
    $httpProvider.defaults.xsrfHeaderName = 'X-CSRF-Token-' + deviceIDShort;
    $httpProvider.defaults.xsrfCookieName = 'CSRF-Token-' + deviceIDShort;
    $httpProvider.useApplyAsync(true);

    // language and localisation

    $translateProvider.useSanitizeValueStrategy('escape');
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
    var labelA = a.id;
    if (typeof a.label !== 'undefined' && a.label !== null && a.label.length > 0) {
        labelA = a.label;
    }

    var labelB = b.id;
    if (typeof b.label !== 'undefined' && b.label !== null && b.label.length > 0) {
        labelB = b.label;
    }

    if (labelA < labelB) {
        return -1;
    }
    return labelA > labelB;
}

function deviceMap(l) {
    var m = {};
    l.forEach(function (r) {
        m[r.deviceID] = r;
    });
    return m;
}

function deviceList(m) {
    var l = [];
    for (var id in m) {
        l.push(m[id]);
    }
    l.sort(deviceCompare);
    return l;
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

function buildTree(children) {
    /* Converts
    *
    * {
    *   'foo/bar': [...],
    *   'foo/baz': [...]
    * }
    *
    * to
    *
    * [
    *   {
    *     title: 'foo',
    *     children: [
    *       {
    *         title: 'bar',
    *         versions: [...],
    *         ...
    *       },
    *       {
    *         title: 'baz',
    *         versions: [...],
    *         ...
    *       }
    *     ],
    *   }
    * ]
    */
    var root = {
        children: []
    }

    $.each(children, function (path, data) {
        var parts = path.split('/');
        var name = parts.splice(-1)[0];

        var keySoFar = [];
        var parent = root;
        while (parts.length > 0) {
            var part = parts.shift();
            keySoFar.push(part);
            var found = false;
            for (var i = 0; i < parent.children.length; i++) {
                if (parent.children[i].title == part) {
                    parent = parent.children[i];
                    found = true;
                    break;
                }
            }
            if (!found) {
                var child = {
                    title: part,
                    key: keySoFar.join('/'),
                    folder: true,
                    children: []
                };
                parent.children.push(child);
                parent = child;
            }
        }

        parent.children.push({
            title: name,
            key: path,
            folder: false,
            versions: data,
        });
    });

    return root.children;
}

// unitPrefixed converts the input such that it returns a string representation
// <1000 (<1024) with the metric unit prefix suffixed. I.e. when calling this with
// binary == true, you need to suffix an addition 'i'.  The "biggest" prefix used
// is 'T', numbers > 1000T are just returned as such big numbers. If ever deemed
// useful 'P' can be added easily.
function unitPrefixed(input, binary) {
    if (input === undefined || isNaN(input)) {
        return '0 ';
    }
    var factor = 1000;
    var i = '';
    if (binary) {
        factor = 1024;
        i = 'i';
    }
    if (input > factor * factor * factor * factor * 1000) {
        // Don't show any decimals for more than 4 digits
        input /= factor * factor * factor * factor;
        return input.toLocaleString(undefined, { maximumFractionDigits: 0 }) + ' T' + i;
    }
    // Show 3 significant digits (e.g. 123T or 2.54T)
    if (input > factor * factor * factor * factor) {
        input /= factor * factor * factor * factor;
        return input.toLocaleString(undefined, { maximumSignificantDigits: 3 }) + ' T' + i;
    }
    if (input > factor * factor * factor) {
        input /= factor * factor * factor;
        if (binary && input >= 1000) {
            return input.toLocaleString(undefined, { maximumFractionDigits: 0 }) + ' G' + i;
        }
        return input.toLocaleString(undefined, { maximumSignificantDigits: 3 }) + ' G' + i;
    }
    if (input > factor * factor) {
        input /= factor * factor;
        if (binary && input >= 1000) {
            return input.toLocaleString(undefined, { maximumFractionDigits: 0 }) + ' M' + i;
        }
        return input.toLocaleString(undefined, { maximumSignificantDigits: 3 }) + ' M' + i;
    }
    if (input > factor) {
        input /= factor;
        var prefix = ' k';
        if (binary) {
            prefix = ' K';
        }
        if (binary && input >= 1000) {
            return input.toLocaleString(undefined, { maximumFractionDigits: 0 }) + prefix + i;
        }
        return input.toLocaleString(undefined, { maximumSignificantDigits: 3 }) + prefix + i;
    }
    return Math.round(input).toLocaleString() + ' ';
};
