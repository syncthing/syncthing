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
        self.folder = CurrentFolder;

        $scope.$watch(function() {
            return self.folder.id;
        }, function (newId) {
            if (newId) {
                refresh(newId);
            }
        });

        function refresh(folderId) {
            getBrowse(self.folder.id).then(function (response) {
                self.browse = response;
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
                var path = pathPrefix.concat([name]);
                if (!isFile) {
                    // Add a trailing slash to directory path to easily match pattern path
                    path.push('');
                }
                var item = {
                    name: name,
                    path: path.join('/'),
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
