angular.module('syncthing.folder')
    .service('Browse', function ($http) {
        'use strict';

        var self = this;

        // public definitions

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
                pathPrefix.push(prefix.replace(/\/+$/g, ''));
            }

            var items = [];
            for (var name in data) {
                var isFile = Array.isArray(data[name]);
                var item = {
                    name: name,
                    path: pathPrefix.concat([name]).join('/'),
                    isFile: isFile
                };
                items.push(item);
            }
            return items;
        };
    });
