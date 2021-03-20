angular.module('syncthing.folder')
    .service('Browse', function ($http, System) {
        'use strict';

        var self = this;

        // public definitions

        self.TYPE_DIRECTORY = 'FILE_INFO_TYPE_DIRECTORY'
        self.TYPE_FILE = 'FILE_INFO_TYPE_FILE'
        self.data = {
            files: [],
        };

        self.refresh = function(folderId, prefix) {
            return getBrowse(folderId, prefix).then(function(response) {
                self.data.files = response;
                return self.data;
            });
        };

        /*
         * private definitions
         */

        function getBrowse(folderId, prefix) {
            var params = { folder: folderId, levels: 0, prefix: prefix };
            return $http.get('rest/db/browse', { params: params }).then(function (response) {
                return browseList(response.data, prefix);
            });
        };

        function browseList(data, prefix) {
            var pathPrefix = []
            if (prefix) {
                // Strip trailing slash from prefix to combine with paths
                pathPrefix.push(prefix.replace(new RegExp('\\' + System.data.pathSeparator + '+$'), ''));
            }

            if (!Array.isArray(data)) throw 'Expected rest/db/browse response to be array';
            return data.map(function (entry) {
                return {
                    name: entry.name,
                    path: pathPrefix.concat([entry.name]).join(System.data.pathSeparator),
                    isFile: entry.type !== self.TYPE_DIRECTORY
                };
            });
        };
    });
