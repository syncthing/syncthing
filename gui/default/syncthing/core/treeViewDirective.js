angular.module('syncthing.core')
    .directive('treeElement', function () {
        return {
            restrict: 'EA',
            scope: {
                isDirectory: "=",
                name: "@",
                folderId: "@"
            },
            template: `
                <div ng-if="isDirectory">
                    <a href="#" ng-click="setCurrentFolderAndGetContents(folderId, name)">
                        <span style="margin:10px;" class="fas fa-folder"></span>
                        <span>{{name}}</span>
                    </a>
                </div>
                <div ng-if="!isDirectory">
                    <span style="margin:10px;" class="fas fa-file"></span>
                    <span>{{name}}</span>
                </div>
            `
        }
});