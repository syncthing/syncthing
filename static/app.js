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
	$scope.categories = [
	{key: 'totFiles', descr: 'Files Managed per Node', unit: ''},
	{key: 'maxFiles', descr: 'Files in Largest Repo', unit: ''},
	{key: 'totMiB', descr: 'Data Managed per Node', unit: 'MiB'},
	{key: 'maxMiB', descr: 'Data in Largest Repo', unit: 'MiB'},
	{key: 'numNodes', descr: 'Number of Nodes in Cluster', unit: ''},
	{key: 'numRepos', descr: 'Number of Repositories Configured', unit: ''},
	{key: 'memoryUsage', descr: 'Memory Usage', unit: 'MiB'},
	{key: 'memorySize', descr: 'System Memory', unit: 'MiB'},
	{key: 'sha256Perf', descr: 'SHA-256 Hashing Performance', unit: 'MiB/s'},
	];

	$http.get('/report').success(function (data) {
		$scope.report = data;

		var versions = [];
		for (var ver in data.versions) {
			versions.push([ver, data.versions[ver]]);
		}
		$scope.versions = sortedList(data.versions);
		$scope.platforms = sortedList(data.platforms);

		var os = aggregate(data.platforms, function (x) {return x.replace(/-.*/, '');})
		$scope.os = sortedList(os);

	}).error(function () {
		$scope.failure = true;
	});
});

function aggregate(d, f) {
	var r = {};
	for (var o in d) {
		var k = f(o);
		if (k in r) {
			r[k] += d[o];
		} else {
			r[k] = d[o];
		}
	}
	return r;
}

function sortedList(d) {
	var l = [];
	var tot = 0;
	for (var o in d) {
		tot += d[o];
	}

	for (var o in d) {
		l.push([o, d[o], 100 * d[o] / tot]);
	}

	l.sort(function (a, b) {
		if (b[1] < a[1])
			return -1;
		return b[1] > a[1];
	});
	return l;
}
