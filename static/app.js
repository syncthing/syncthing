// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

/*jslint browser: true, continue: true, plusplus: true */
/*global $: false, angular: false */

'use strict';

var reports = angular.module('reports', []);

reports.controller('ReportsCtrl', function ($scope, $http) {
    $scope.report = {};
    $scope.failure = false;

    $http.get('/report').success(function (data) {
        $scope.report = data;
    }).error(function () {
        $scope.failure = true;
    });
});
