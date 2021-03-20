angular.module('syncthing.folder')
    .service('Ignores', function ($http, System) {
        'use strict';

        var self = this;

        // public definitions

        self.data = {
            // Text representation of ignore patterns. `text` is updated when
            // patterns are added or removed, but `parseTest` must be called
            // to update `patterns` after modifying `text`.
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

            // Find the last pattern more specific than newPattern. Because
            // the first matching pattern is applied to a file, we need to
            // insert newPattern in a position that doesn't override existing
            // patterns.
            var afterIndex = findLastIndex(self.data.patterns, function(pattern) {
                return patternMoreSpecificThan(pattern, newPattern);
            });
            self.data.patterns.splice(afterIndex + 1, 0, newPattern);

            // Remove any more specific patterns so the new pattern has the intended effect.
            for (var i = afterIndex; i >= 0; i--) {
                if (patternMoreSpecificThan(self.data.patterns[i], newPattern)) {
                    self.data.patterns.splice(i, 1);
                }
            }

            // Update text to reflect pattern changes.
            self.data.text = self.data.patterns.map(function(r) { return r.text; }).join('\n');
            return newPattern;
        };

        self.removePattern = function(text) {
            // Find the pattern with the specified text
            var index = self.data.patterns.findIndex(function(pattern) {
                return pattern.text === text;
            });
            if (index >= 0) {
                var oldPattern = self.data.patterns.splice(index, 1)[0];

                for (var i = index - 1; i >= 0; i--) {
                    if (patternMoreSpecificThan(self.data.patterns[i], oldPattern)) {
                        self.data.patterns.splice(i, 1);
                    }
                }

                self.data.text = self.data.patterns.map(function(r) { return r.text; }).join('\n');
                return oldPattern;
            }
            return null;
        };

        self.matchingPattern = function(file) {
            return self.data.patterns.find(function(pattern) {
                // Only consider patterns that match a simple path
                if (!pattern.isSimple) return false;

                var absPath = System.data.pathSeparator + file.path;
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
            var matchFunc = !hasPrefix && chooseMatcher(path);

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

        // Infer a path from wildcards that might be used to match a file path
        // by prefix (for simple patterns)
        // The changes made to the path make it more specific than the original
        // pattern. It is useful for prefix matcher. Should a glob matcher be
        // added this path should not be used to match because it does not
        // represent the complete pattern. The most common effect of the changes
        // is matching patterns like `*`, `**`, or `/*` without a special case
        function toPath(line) {
            // Add a leading slash when the pattern begins with wildcards
            // because a pattern beginning with * or ** will match entries at
            // the root (as well as those in child directories)
            line = line.replace(/^\*+/, System.data.pathSeparator + '$&'); // `$&` inserts the matched substring, a series of wildcards

            // Trim trailing wildcards after separator because child paths would
            // already be prefixed by this path
            line = line.replace(new RegExp('\\' + System.data.pathSeparator + '\\*+$'), System.data.pathSeparator);
            return line;
        }

        // Inspect the pattern to determine if it is simple enough to match,
        // then return a function that can be applied to match a path.
        function chooseMatcher(line) {
            if (line.length === 0) return neverMatch;
            if (line.indexOf('//') === 0) return neverMatch; // comment
            if (line.indexOf(System.data.pathSeparator) !== 0) return null; // not a root line
            if (line.length > 1 && line.charAt(line.length - 1) === System.data.pathSeparator) return null; // trailing slash

            line = line.replaceAll(/\\[\*\?\[\]\{\}]/g, '') // remove properly escaped glob characters
            if (line.match(/[\*\?\[\]\{\}]/)) {
                // The pattern contains a special character. The pattern is a
                // glob, too complex for us to handle in the UI. Return no
                // matcher function indicating we cannot match it.
                return null;
            }

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

        // Here we abuse matchFunc by using it to match another
        // pattern's path. The comparison assumes both patterns are
        // simple with a prefixMatch type, so if support for glob
        // patterns is added this will need a different approach.
        function patternMoreSpecificThan(patternA, patternB) {
            return patternB.isSimple && patternA.isSimple && patternB.matchFunc(patternA.path)
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
            if (patternPath.charAt(patternPath.length - 1) === System.data.pathSeparator) return true;

            var suffix = filePath.slice(patternPath.length);
            // pattern is an exact match to file path
            if (suffix.length === 0) return true;
            // pattern is an exact match to a parent directory in the file path
            return suffix.charAt(0) === System.data.pathSeparator;
        }

        function neverMatch() {
            return false;
        }
    });
