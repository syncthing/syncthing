angular.module('syncthing.core')
    .directive('treeElement', function () {
        return {
            restrict: 'EA',
            scope: {
                fileType: "=",
                name: "@",
                show: "&",
            },
            template: `
                <div ng-if="fileType === 0">
                    <span style="margin:10px;" class="fas fa-file"></span>
                    <span>{{name}}</span>
                </div>
                <div ng-if="fileType === 1">
                    <a href="#" ng-click="show()">
                        <span style="margin:10px;" class="fas fa-folder"></span>
                        <span>{{name}}</span>
                    </a>
                </div>
                <div ng-if="fileType === 4">
                    <span style="margin:10px;" class="fa fa-link"></span>
                    <span>{{name}}</span>
                </div>
            `
        }
});