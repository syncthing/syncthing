angular.module('syncthing.folder')
    .factory('CurrentFolder', function () {
        'use strict';

        // This singleton object is empty now but will allow multiple modules to
        // access attributes of the current folder.
        return {};
    });
