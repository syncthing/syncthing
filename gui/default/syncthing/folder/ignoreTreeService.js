angular.module('syncthing.folder')
    .service('IgnoreTree', function (
        Ignores,
        Browse,
    ) {
        'use strict';

        // Bind methods directly to the controller so we can use controllerAs in template
        var self = this;

        // public definitions
        self.tree = null;

        // TODO: there is a bug with refresh
        // Sometimes clear throws `Fancytree assertion failed: only init
        // supported`. Always when calling from textarea blur, sometimes when
        // opening modal for a different folder
        self.refresh = function(folderId) {
            if (self.tree) self.tree.clear();
            self.tree = $("#ignore-tree").fancytree({
                extensions: ["table"],
                checkbox: true,
                selectMode: 2,
                table: {
                    indentation: 20,
                    nodeColumnIdx: 1,
                    checkboxColumnIdx: 0,
                },
                debugLevel: 2,
                source: Browse.refresh(folderId).then(function(response) {
                    return response.files.map(buildNode);
                }),
                lazyLoad: function (event, data) {
                    var prefix = data.node.data.file.path;
                    data.result = Browse.refresh(folderId, prefix).then(function(response) {
                        return response.files.map(buildNode);
                    });
                },
                select: function (event, data) {
                    toggle(data.node);
                },
            }).fancytree("getTree");
        }

        function buildNode(file) {
            var match = Ignores.matchingPattern(file);

            return {
                // Fancytree keys
                title: file.name,
                selected: (!match || match.isNegated),
                key: file.path,
                lazy: !file.isFile,
                folder: !file.isFile,
                // Data keys
                file: file,
                match: match,
            };
        };

        function toggle(node) {
            var absPath = '/' + node.data.file.path;
            if (node.data.match) {
                var match = node.data.match;
                if (absPath === match.path) {
                    // match is exact match to this file, remove match from patterns
                    Ignores.removePattern(match.text);
                } else {
                    // match is parent directory of file
                    // If the parent pattern is negated, add pattern ignoring this file
                    var prefix = match.isNegated ? '' : '!';
                    Ignores.addPattern(prefix + absPath);
                }
            } else {
                // Add a pattern to ignore this file
                Ignores.addPattern(absPath);
            }

            updateNodes([node]);
        };

        function updateNodes(nodes) {
            if (!Array.isArray(nodes)) return;

            nodes.forEach(function(node) {
                var match = Ignores.matchingPattern(node.data.file);
                node.data.match = match;
                node.selected = (!match || match.isNegated);


                node.render(true);
                updateNodes(node.children);
            });
        };
    });
