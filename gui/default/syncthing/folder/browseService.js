angular.module('syncthing.folder')
    .service('Browse', function ($http) {
        'use strict';

        var self = this;

        // public definitions

        self.data = {
            pathParts: [],
            files: [],
        };

        self.refresh = function(folderId, prefix) {
            return getBrowse(folderId, prefix).then(function(response) {
                angular.copy(response, self.data);
                return self.data;
            });
        };

        /*
         * private definitions
         */

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
                    files: browseList(response.data, cleanPrefix)
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
    });
