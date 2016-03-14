angular.module('syncthing.core')
    .directive('pathIsSubDir', function () {
        return {
            require: 'ngModel',
            priority: 1000,
            link: function (scope, elm, attrs, ctrl) {
                ctrl.$parsers.unshift(function (viewValue) {
                    // This function checks whether xdir is a subdirectory of ydir,
                    // e.g. it would return true if xdir = "/home/a", ydir = "/home/a/b".
                    function isSubDir(xdir, ydir) {
                        var xdirArr = xdir.split("/");
                        var ydirArr = ydir.split("/");
                        if (xdirArr.slice(-1).pop() === "") {
                            xdirArr = xdirArr.slice(0, -1);
                        }
                        if (xdirArr.length > ydirArr.length) {
                            return false;
                        }
                        return xdirArr.map(function(e, i) {
                            return xdirArr[i] === ydirArr[i];
                        }).every(e => e == true);
                    }


                    scope.pathIsSubFolder = false;
                    scope.otherPath = "";
                    for (var folderID in scope.folders) {
                        scope.otherPath = scope.folders[folderID].path;
                        if (isSubDir(scope.otherPath, viewValue)) {
                            scope.pathIsSubFolder = true;
                            break;
                        }
                    }
                    return viewValue;
                });
            }
        };
});
