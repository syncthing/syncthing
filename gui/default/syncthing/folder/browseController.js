angular.module('syncthing.folder')
    .controller('BrowseController', function ($scope, $http, $q, CurrentFolder) {
        'use strict';

        // Bind methods directly to the controller so we can use controllerAs in template
        var self = this;

        // public definitions

        // Browse data for the most recently fetched folder
        self.browse = {
            /*
            pathParts: [],
            list: [],
            */
        };
        // Parsed ignore pattern objects. Order matters, first match is applied.
        self.ignorePatterns = [
            /*
            {
                text: '/Photos', // original text of the ignore pattern
                isSimple: true, // whether the pattern is unambiguous and can be displayed by browser
                isNegated: false, // begins with !, matching files are included
                path: '/Photos', // path to the file or directory, stripped of prefix and used for matching
            }
            */
        ];
        self.folder = CurrentFolder;

        $scope.$watch(function() {
            return self.folder.id;
        }, function (newId) {
            if (newId) {
                refresh(newId);
            }
        });

        function refresh(folderId) {
            $q.all([
                getIgnores(self.folder.id),
                getBrowse(self.folder.id),
            ]).then(function (responses) {
                self.ignorePatterns = responses[0];
                self.browse = responses[1];
            });
        };

        /*
         * Browse
         */

        self.navigate = function(folderId, prefix) {
            getBrowse(folderId, prefix).then(function (response) {
                self.browse = response;
            });
        };

        function getBrowse(folderId, prefix) {
            var params = { folder: folderId, levels: 0 };
            var cleanPrefix = '';
            if (prefix) {
                // Ensure functions receive a nice clean prefix to combine with paths
                cleanPrefix = prefix.replace(/\/+$/g, '');
                params.prefix = cleanPrefix;
            }

            return $http.get('rest/db/browse', { params: params }).then(function (response) {
                return {
                    pathParts: browsePath(folderId, cleanPrefix),
                    list: browseList(response.data, cleanPrefix)
                };
            });
        };

        function browsePath(folderId, prefix) {
            // Always include a part for the folder root
            var parts = [{ name: folderId, prefix: '' }];
            var prefixAcc = '';
            prefix.split('/').forEach(function (part) {
                if (part) {
                    parts.push({ name: part, prefix: prefixAcc + part });
                    prefixAcc = prefixAcc + part + '/'
                }
            });
            return parts;
        }

        function browseList(data, prefix) {
            var pathPrefix = []
            if (prefix.length > 0) {
                pathPrefix.push(prefix);
            }

            var items = [];
            for (var name in data) {
                var isFile = Array.isArray(data[name]);
                var item = {
                    name: name,
                    path: pathPrefix.concat([name]).join('/'),
                    isFile: isFile
                };
                if (isFile) {
                    item.modifiedAt = moment(data[name][0]);
                    item.size = data[name][1];
                }
                items.push(item);
            }
            return items;
        };

        /*
         * Ignore Patterns
         */

        function getIgnores(folderId) {
            return $http.get('rest/db/ignores', {
                params: { folder: folderId }
            }).then(function (response) {
                var data = response.data;
                return data.ignore.map(parsePattern);
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

        self.matchingPatterns = function(file) {
            return self.ignorePatterns.filter(function(pattern) {
                // Only consider patterns that match a simple path
                if (!pattern.isSimple) return false;

                var absPath = '/' + file.path;
                if (absPath.indexOf(pattern.path) !== 0) return false;

                // pattern ends with path separator, file is a child of the pattern path
                if (pattern.path.charAt(pattern.path.length - 1) === '/') return true;

                var suffix = absPath.slice(pattern.path.length);
                // pattern is an exact match to file path
                if (suffix.length === 0) return true;
                // pattern is an exact match to a parent directory in the file path
                return suffix.charAt(0) === '/';
            });
        }
    });
