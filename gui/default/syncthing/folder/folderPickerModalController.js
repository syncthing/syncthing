angular.module('syncthing.folder')
    .controller('FolderPickerModalController', function ($scope, $http, $rootScope) {
        'use strict';

        $scope.directories = [];

        function addTrailingSeparator(path) {
            if (path.length > 0 && !path.endsWith($scope.pathSeparator)) {
                return path + $scope.pathSeparator;
            }
            return path;
        }

        function stripTrailingSeparator(path) {
            if (path.length > $scope.pathSeparator.length && path.endsWith($scope.pathSeparator)) {
                return path.slice(0, -$scope.pathSeparator.length);
            }
            return path;
        }

        function getParentPath(path) {
            let parts = stripTrailingSeparator(path).split($scope.pathSeparator);
            parts.pop();

            if (parts.length === 0) return '';
            return parts.join($scope.pathSeparator) + $scope.pathSeparator;
        }

        $scope.formatDirectoryName = function (path) {
            return path.split($scope.pathSeparator).filter(Boolean).pop() || path;
        };

        $scope.updateDirectoryList = function () {
            $http.get(urlbase + '/system/browse', {
                params: {
                    current: $scope.currentPath
                }
            }).success(function (data) {
                $scope.directories = data.map(addTrailingSeparator);
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
            $rootScope.$emit('folderPathSelected', stripTrailingSeparator($scope.currentPath));
            angular.element('#folderPicker').modal('hide');
        };

        angular.element("#folderPicker").on("shown.bs.modal", function () {
            $scope.$apply(() => {
                $scope.pathSeparator = $scope.$parent.system.pathSeparator || '/';
                $scope.currentPath = $scope.$parent.currentFolder.path || '';
                $scope.updateDirectoryList();
            });
        });

        angular.element("#folderPickerSelect").on("click", $scope.selectCurrentPath)
    });