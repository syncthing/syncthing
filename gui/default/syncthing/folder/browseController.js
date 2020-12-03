angular.module('syncthing.folder')
    .controller('BrowseController', function (
        $scope,
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
        self.browse = undefined;
        self.fileMatches = undefined;

        $scope.$watch(function() {
            return self.folder.id;
        }, function (newId) {
            if (newId) {
                self.browse = Browse.forFolder(newId);
                self.fileMatches = FileMatches.forFolder(newId);
            }
        });

        self.toggle = function(fileMatch) {
            var absPath = '/' + fileMatch.file.path;
            if (fileMatch.match) {
                var match = fileMatch.match;
                if (absPath === match.path) {
                    // match is exact match to this file, remove match from patterns
                    Ignores.removePattern(self.folder.id, match.text);
                } else {
                    // match is parent directory of file
                    // If the parent pattern is negated, add pattern ignoring this file
                    var prefix = match.isNegated ? '' : '!';
                    Ignores.addPattern(self.folder.id, prefix + absPath);
                }
            } else {
                // Add a pattern to ignore this file
                Ignores.addPattern(self.folder.id, absPath);
            }
            var folder = Ignores.forFolder(self.folder.id);
            FileMatches.update(self.folder.id, self.browse.files, folder.patterns);
        };

        self.navigate = function(folderId, prefix) {
            Browse.refresh(folderId, prefix).then(function (response) {
                FileMatches.update(folderId, response.files, Ignores.forFolder(folderId).patterns);
            });
        };
    });
