angular.module('syncthing.folder')
    .service('FileMatches', function () {
        'use strict';

        var self = this;

        // public definitions
        self.matches = {};

        self.forFolder = function(folderId) {
            var folder = self.matches[folderId];
            if (!folder) {
                folder = [];
                self.matches[folderId] = folder;
            }
            return folder;
        };

        self.update = function(folderId, files, patterns) {
            var matches = files.map(function(file) {
                return {
                    file: file,
                    matches: matchingPatterns(file, patterns),
                };
            });
            var folder = self.forFolder(folderId);
            Array.prototype.splice.apply(folder, [0, folder.length].concat(matches));
            return folder;
        };

        /*
         * private definitions
         */

        function matchingPatterns(file, patterns) {
            return patterns.filter(function(pattern) {
                // Only consider patterns that match a simple path
                if (!pattern.isSimple) return false;

                var absPath = '/' + file.path;
                return pattern.matchFunc(absPath);
            });
        }
    });
