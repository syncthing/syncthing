angular.module('syncthing.folder')
    .controller('BrowseController', function (
        CurrentFolder,
        Ignores,
        Browse,
        FileMatches,
    ) {
        'use strict';

        // Bind methods directly to the controller so we can use controllerAs in template
        var self = this;

        // public definitions

        self.folder = CurrentFolder;
        // Reference to browse data for the current folder
        self.browse = Browse.data;
        self.fileMatches = FileMatches.data;

        self.toggle = function(fileMatch) {
            var absPath = '/' + fileMatch.file.path;
            if (fileMatch.match) {
                var match = fileMatch.match;
                if (absPath === match.path) {
                    // match is exact match to this file, remove match from patterns
                    Ignores.removePattern(match.text);
                } else {
                    // match is parent directory of file
                    // If the parent pattern is negated, add pattern ignoring this file
                    var prefix = match.isNegated ? '' : '!';
                    Ignores.addPattern(prefix + absPath);
                }
            } else {
                // Add a pattern to ignore this file
                Ignores.addPattern(absPath);
            }
            var folder = Ignores.data;
            FileMatches.update(self.browse.files, folder.patterns);
        };

        self.navigate = function(folderId, prefix) {
            Browse.refresh(folderId, prefix).then(function (response) {
                FileMatches.update(response.files, Ignores.data.patterns);
            });
        };
    });
