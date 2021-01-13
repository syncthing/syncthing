angular.module('syncthing.folder')
    .service('Ignores', function ($http) {
        'use strict';

        var self = this;

        // public definitions

        self.data = {
            // Text representation of ignore patterns. Updated when patterns
            // are added or removed, but modifying `text`` does not update
            // `patterns`.
            text: '',
            error: null,
            disabled: false,
            // Parsed ignore pattern objects. Order matters, first match is applied.
            patterns: [
                /*
                {
                    text: '/Photos', // original text of the ignore pattern
                    isSimple: true, // whether the pattern is unambiguous and can be displayed by browser
                    isNegated: false, // begins with !, matching files are included
                    path: '/Photos', // path to the file or directory, stripped of prefix and used for matching
                    matchFunc: function (filePath),
                }
                */
            ],
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
            self.data.text = 'Loading...';
            self.data.error = null;
            self.data.disabled = true;
            return getIgnores(folderId).then(function (response) {
                self.data.text = response.map(function(r) { return r.text; }).join('\n');
                self.data.patterns = response;
                self.data.disabled = false;
                return self.data;
            }).catch(function (err) {
                self.data.error = err;
                self.data.text = '';
                return Promise.reject(err);
            });
        };

        self.save = function(folderId, ignores) {
            return $http.post('rest/db/ignores',
                { ignore: ignores },
                { params: { folder: folderId } }
            );
        };

        self.parseText = function(text) {
            if (text.length === 0) {
                self.data.patterns = [];
            } else {
                self.data.patterns = text
                    .split('\n')
                    .map(parsePattern);
            }
            return self.data.patterns;
        };

        self.addPattern = function(text) {
            var newPattern = parsePattern(text);
            var afterIndex = findLastIndex(self.data.patterns, function(pattern) {
                return pattern.isSimple && newPattern.isSimple && newPattern.matchFunc(pattern.path);
            });
            self.data.patterns.splice(afterIndex + 1, 0, newPattern);
            self.data.text = self.data.patterns.map(function(r) { return r.text; }).join('\n');
            return newPattern;
        };

        self.removePattern = function(text) {
            var index = self.data.patterns.findIndex(function(pattern) {
                return pattern.text === text;
            });
            if (index >= 0) {
                var oldPattern = self.data.patterns.splice(index, 1)[0];
                self.data.text = self.data.patterns.map(function(r) { return r.text; }).join('\n');
                return oldPattern;
            }
            return null;
        };

        self.matchingPattern = function(file) {
            return self.data.patterns.find(function(pattern) {
                // Only consider patterns that match a simple path
                if (!pattern.isSimple) return false;

                var absPath = '/' + file.path;
                return pattern.matchFunc(absPath);
            });
        }

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
            var stripResult = splitPrefix(line.trim());
            var prefixes = stripResult[0];
            var hasPrefix = prefixes['(?i)'] || prefixes['(?d)'];
            var path = toPath(stripResult[1]);
            var matchFunc = !hasPrefix && matcher(path);

            return {
                text: line,
                isSimple: !!matchFunc,
                isNegated: prefixes['!'],
                path: path,
                matchFunc: matchFunc,
            };
        };

        // Adapted from lib/ignore/ignore.go@parseLine
        function splitPrefix(line) {
            var seenPrefix = { '!': false, '(?i)': false, '(?d)': false };

            var found = true;
            while (found === true) {
                found = false;
                for (var prefix in seenPrefix) {
                    if (!seenPrefix[prefix] && line.indexOf(prefix) === 0) {
                        seenPrefix[prefix] = true;
                        line = line.slice(prefix.length);
                        found = true;
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

        // Return a function that can be applied to match the pattern
        function matcher(line) {
            if (line.length === 0) return neverMatch;
            if (line.indexOf('//') === 0) return neverMatch; // comment
            if (line.indexOf('/') !== 0) return null; // not a root line
            if (line.length > 1 && line.charAt(line.length - 1) === '/') return null; // trailing slash

            line = line.replaceAll(/\\[\*\?\[\]\{\}]/g, '') // remove properly escaped characters for this evaluation
            if (line.match(/[\*\?\[\]\{\}]/)) return null; // contains special character

            return prefixMatch;
        }

        function findLastIndex(array, predicate) {
            var i = array.length;
            while (i--) {
                if (predicate(array[i], i, array)) {
                    return i;
                }
            }
            return -1;
        }

        /*
         * pattern matching functions
         */

        // A prefix match is anchored at folder root and is an exact match to
        // one file or directory
        function prefixMatch(filePath) {
            var patternPath = this.path
            if (filePath.indexOf(patternPath) !== 0) return false;

            // pattern ends with path separator, file is a child of the pattern path
            if (patternPath.charAt(patternPath.length - 1) === '/') return true;

            var suffix = filePath.slice(patternPath.length);
            // pattern is an exact match to file path
            if (suffix.length === 0) return true;
            // pattern is an exact match to a parent directory in the file path
            return suffix.charAt(0) === '/';
        }

        function neverMatch() {
            return false;
        }
    });
