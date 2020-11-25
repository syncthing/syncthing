angular.module('syncthing.folder')
    .controller('BrowseController', function ($scope, CurrentFolder, Ignores, Browse) {
        'use strict';

        // Bind methods directly to the controller so we can use controllerAs in template
        var self = this;

        // public definitions

        self.folder = CurrentFolder;
        // Reference to browse data for the current folder
        self.browse = undefined;

        $scope.$watch(function() {
            return self.folder.id;
        }, function (newId) {
            if (newId) {
                self.browse = Browse.forFolder(newId);
            }
        });

        /*
         * Browse
         */

        self.navigate = function(folderId, prefix) {
            Browse.refresh(folderId, prefix);
        };

        self.matchingPatterns = function(file) {
            return Ignores.forFolder(self.folder.id).patterns.filter(function(pattern) {
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
