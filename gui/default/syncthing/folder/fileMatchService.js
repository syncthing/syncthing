angular.module('syncthing.folder')
    .service('FileMatches', function () {
        'use strict';

        var self = this;

        // public definitions
        self.data = [];

        self.update = function(files, patterns) {
            var matches = files.map(function(file) {
                return {
                    file: file,
                    match: matchingPattern(file, patterns),
                };
            });
            Array.prototype.splice.apply(self.data, [0, self.data.length].concat(matches));
            return self.data;
        };

        /*
         * private definitions
         */

        function matchingPattern(file, patterns) {
            return patterns.find(function(pattern) {
                // Only consider patterns that match a simple path
                if (!pattern.isSimple) return false;

                var absPath = '/' + file.path;
                return pattern.matchFunc(absPath);
            });
        }
    });
