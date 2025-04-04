angular.module('syncthing.folder')
    .controller('FolderPickerModalController', function ($scope, $http, $rootScope) {
        'use strict';

        $scope.directories = [];
        $scope.showHiddenFiles = false;

        // Ensure path ends with a separator (e.g., "/")
        function normalizePath(path) {
            if (!path || path.endsWith($scope.pathSeparator)) return path;
            return path + $scope.pathSeparator;
        }

        function getParentPath(path) {
            path = path.endsWith($scope.pathSeparator) ? path.slice(0, -$scope.pathSeparator.length) : path;
            let parts = path.split($scope.pathSeparator);
            parts.pop();

            if (parts.length === 0) return '';
            return parts.join($scope.pathSeparator) + $scope.pathSeparator;
        }

        $scope.formatDirectoryName = function (path) {
            return path.split($scope.pathSeparator).filter(Boolean).pop() || path;
        };

        $scope.isHidden = function (path) {
            return $scope.formatDirectoryName(path).startsWith('.');
        };

        $scope.updateDirectoryList = function () {
            $http.get(urlbase + '/system/browse', {
                params: {
                    current: $scope.currentPath
                }
            }).success(function (data) {
                $scope.directories = data.map(normalizePath).filter(dir =>
                    $scope.showHiddenFiles || !$scope.isHidden(dir)
                );
            }).error($scope.emitHTTPError);
        };

        $scope.navigateTo = function (path) {
            $scope.currentPath = path;
            $scope.updateDirectoryList();
        };

        $scope.navigateUp = function () {
            $scope.currentPath = getParentPath($scope.currentPath);
            $scope.updateDirectoryList()
        };

        $scope.selectCurrentPath = function () {
            $rootScope.$emit('folderPathSelected', $scope.currentPath);
            angular.element('#folderPicker').modal('hide');
        };

        angular.element("#folderPicker").on("shown.bs.modal", function () {
            $scope.$apply(() => {
                $scope.pathSeparator = $scope.$parent.system.pathSeparator || '/';
                $scope.currentPath = $scope.$parent.currentFolder.path || $scope.pathSeparator;
                $scope.updateDirectoryList();
            });
        });

        angular.element("#folderPickerSelect").on("click", $scope.selectCurrentPath)
    });