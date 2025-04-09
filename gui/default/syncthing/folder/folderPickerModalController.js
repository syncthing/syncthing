angular.module('syncthing.folder')
    .controller('FolderPickerModalController', function ($scope, $http, $rootScope, $timeout, $translate) {
        'use strict';

        $scope.tree = null;
        $scope.tempNodes = [];

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

        function expandTilde(path) {
            if (path.startsWith('~')) {
                return path.replace('~', $scope.$parent.system.tilde);
            }
            return path;
        }

        function splitPath(path) {
            const trimmedPath = expandTilde(stripTrailingSeparator(path));
            // Keep the leading separator if it exists
            let parts = trimmedPath.startsWith($scope.pathSeparator)
                ? [$scope.pathSeparator, ...trimmedPath.slice($scope.pathSeparator.length).split($scope.pathSeparator)]
                : trimmedPath.split($scope.pathSeparator);

            return parts.filter(Boolean);
        }

        function joinPath(...parts) {
            // Don't add a second separator if the first part is already a separator
            if (parts.length > 0 && parts[0] === $scope.pathSeparator) {
                return $scope.pathSeparator + parts.slice(1).join($scope.pathSeparator);
            }
            return parts.join($scope.pathSeparator);
        }

        function formatDirectoryName(path) {
            return splitPath(path).pop() || path;
        }

        async function fetchSubdirectories(path) {
            let res = await $http.get(urlbase + '/system/browse', {
                params: {current: addTrailingSeparator(path)}
            });
            return res.data.map(dir => ({
                title: formatDirectoryName(dir),
                key: stripTrailingSeparator(dir),
                folder: true,
                lazy: true
            }));
        }

        function clearTemporaryNodes(currentNode) {
            // Delete temporary nodes, children first
            $scope.tempNodes.sort((a, b) => b.getLevel() - a.getLevel());
            $scope.tempNodes = $scope.tempNodes.filter(node => {
                if (!node.children && node.key !== currentNode.key) {
                    node.remove();
                    return false;
                }
                return true;
            });
        }

        async function findOrCreateNode(currentNode, key) {
            if (currentNode.isLazy()) {
                await currentNode.load();
            }
            if (!currentNode.children) {
                currentNode.children = [];
            }

            let nextNode = currentNode.children.find(child => child.title === key);
            // Prevent creating temp nodes at the root of the tree
            if (!nextNode && currentNode !== $scope.tree.getRootNode()) {
                nextNode = currentNode.addChildren({
                    title: key,
                    key: joinPath(currentNode.key, key),
                    extraClasses: "folderTree-new-folder",
                    folder: true,
                });
                currentNode.sortChildren();
                $scope.tempNodes.push(nextNode);
            }
            return nextNode;
        }

        function handleNodeActivation(node) {
            clearTemporaryNodes(node);
            if (node.key === expandTilde(stripTrailingSeparator($scope.currentPath))) return;

            $scope.$apply(() => {
                $scope.currentPath = node.key;
            });
        }

        async function initFolderTree() {
            const rootDirs = await fetchSubdirectories('');

            $("#folderTree").fancytree({
                extensions: ["table", "glyph"],
                quicksearch: true,
                glyph: {
                    preset: "awesome5",
                    map: {
                        expanderLazy: "fa fa-caret-right",
                    }
                },
                table: {
                    indentation: 24,
                },
                strings: {
                    loading: $translate.instant("Loading data..."),
                    loadError: $translate.instant("Failed to load data"),
                },
                debugLevel: 1,
                selectMode: 1,
                source: rootDirs,
                lazyLoad: function (event, data) {
                    data.result = fetchSubdirectories(data.node.key);
                },
                activate: (event, data) => handleNodeActivation(data.node),
                enhanceTitle: function (event, data) {
                    if (data.node.extraClasses?.includes("folderTree-new-folder")) {
                        data.$title.attr("data-original-title", $translate.instant("Folder will be created"));
                        data.$title.tooltip();
                    }
                }
            });

            $scope.tree = $.ui.fancytree.getTree("#folderTree");
        }

        $scope.selectNodeByCurrentPath = async function () {
            if (!$scope.tree || !$scope.currentPath) return;

            const parts = splitPath($scope.currentPath);

            let currentNode = $scope.tree.getRootNode();
            for (let part of parts) {
                currentNode = await findOrCreateNode(currentNode, part);
                if (!currentNode) return;
            }

            await currentNode.makeVisible();
            await currentNode.setExpanded(true);
            await currentNode.setActive(true);
        }

        angular.element("#folderPicker").on("shown.bs.modal", function () {
            $scope.pathSeparator = $scope.$parent.system.pathSeparator || '/';
            $scope.currentPath = $scope.$parent.currentFolder.path || '';

            $timeout(async () => {
                if (!$scope.tree) {
                    await initFolderTree();
                }
                await $scope.selectNodeByCurrentPath();
            })
        });

        angular.element("#folderPickerSelect").on("click", () => {
            $rootScope.$emit('folderPathSelected', stripTrailingSeparator($scope.currentPath));
            angular.element('#folderPicker').modal('hide');
        });
    });
