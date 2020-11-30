angular.module('syncthing.folder')
    .service('Ignores', function ($http) {
        'use strict';

        var self = this;

        // public definitions

        // map of folder id to ignores state
        self.ignores = {
            /*
            ['folderId']: {
                // Text representation of ignore patterns. Updated when patterns
                // are added or removed, but modifying `text`` does not update
                // `patterns`.
                text: '',
                error: null,
                disabled: false,
                // Parsed ignore pattern objects. Order matters, first match is applied.
                patterns: [
                    {
                        text: '/Photos', // original text of the ignore pattern
                        isSimple: true, // whether the pattern is unambiguous and can be displayed by browser
                        isNegated: false, // begins with !, matching files are included
                        path: '/Photos', // path to the file or directory, stripped of prefix and used for matching
                    }
                ],
            }
            */
        };

        self.forFolder = function(folderId) {
            var folder = self.ignores[folderId];
            if (!folder) {
                folder = {
                    text: '',
                    error: null,
                    disabled: false,
                    patterns: [],
                };
                self.ignores[folderId] = folder;
            }
            return folder;
        };

        // Temp folder is shaped like a folder, but is not persisted and not
        // shared with other service consumers
        self.tempFolder = function() {
            return {
                text: '',
                error: null,
                disabled: false,
                patterns: [],
            };
        };

        self.refresh = function(folderId) {
            var folder = self.forFolder(folderId);
            folder.text = 'Loading...';
            folder.error = null;
            folder.disabled = true;
            return getIgnores(folderId).then(function (response) {
                folder.text = response.map(function(r) { return r.text; }).join('\n');
                folder.patterns = response;
                folder.disabled = false;
                return folder;
            }).catch(function (err) {
                folder.text = '';
                throw err;
            });
        };

        self.save = function(folderId, ignores) {
            return $http.post('rest/db/ignores',
                { ignore: ignores },
                { params: { folder: folderId } }
            );
        };

        self.addPattern = function(folderId, text) {
            var folder = self.forFolder(folderId);
            folder.patterns.unshift(parsePattern(text));
            folder.text = folder.patterns.map(function(r) { return r.text; }).join('\n');
        };

        self.removePattern = function(folderId, text) {
            var folder = self.forFolder(folderId);
            var index = folder.patterns.findIndex(function(pattern) {
                return pattern.text === text;
            });
            if (index >= 0) {
                folder.patterns.splice(index, 1);
                folder.text = folder.patterns.map(function(r) { return r.text; }).join('\n');
            }
        };

        /*
         * private definitions
         */

        function getIgnores(folderId) {
            return $http.get('rest/db/ignores', {
                params: { folder: folderId }
            }).then(function (response) {
                var data = response.data;
                return (data.ignore || []).map(parsePattern);
            });
        }

        function parsePattern(line) {
            var stripResult = stripPrefix(line.trim());
            var prefixes = stripResult[0];
            var hasPrefix = prefixes['(?i)'] || prefixes['(?d)'];
            var path = toPath(stripResult[1]);

            return {
                text: line,
                isSimple: !hasPrefix && isSimple(path),
                isNegated: prefixes['!'],
                path: path,
            };
        };

        // Adapted from lib/ignore/ignore.go@parseLine
        function stripPrefix(line) {
            var seenPrefix = { '!': false, '(?i)': false, '(?d)': false };

            var found;
            while (found !== null) {
                found = null;
                for (var prefix in seenPrefix) {
                    if (line.indexOf(prefix) === 0 && !seenPrefix[prefix]) {
                        seenPrefix[prefix] = true;
                        line = line.slice(prefix.length);
                        found = prefix;
                        break;
                    }
                }
            }

            return [seenPrefix, line];
        }

        // Infer a path from wildcards that can be used to match a file by
        // prefix (for simple patterns)
        function toPath(line) {
            line = line.replace(/^\*+/, '/$&');
            // Trim wildcards after final separator for simpler path match
            line = line.replace(/\/\*+$/, '/');
            return line;
        }

        // A "simple" pattern is one that is anchored at folder root and can be
        // displayed by our browser.
        function isSimple(line) {
            if (line.indexOf('/') !== 0) return false; // not a root line
            if (line.indexOf('//') === 0) return false; // comment
            if (line.length > 1 && line.charAt(line.length - 1) === '/') return false; // trailing slash

            line = line.replaceAll(/\\[\*\?\[\]\{\}]/g, '') // remove properly escaped characters for this evaluation
            if (line.match(/[\*\?\[\]\{\}]/)) return false; // contains special character

            return true;
        }
    });
