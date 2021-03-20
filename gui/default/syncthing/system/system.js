angular.module('syncthing.system')
    .service('System', function ($http) {
        'use strict';

        var self = this;

        // public definitions

        self.data = {
            pathSeparator: '/',
        };

        self.refresh = function(folderId, prefix) {
            return $http.get('rest/system/status').success(function (data) {
                self.data = data;
                return self.data;
            });
        };
    });
