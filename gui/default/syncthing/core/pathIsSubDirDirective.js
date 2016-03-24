angular.module('syncthing.core')
    .directive('pathIsSubDir', function () {
        return {
            require: 'ngModel',
            link: function (scope, elm, attrs, ctrl) {
                ctrl.$parsers.unshift(function (viewValue) {
                    // This function checks whether xdir is a subdirectory of ydir,
                    // e.g. it would return true if xdir = "/home/a", ydir = "/home/a/b".
                    function isSubDir(xdir, ydir) {
                        var xdirArr = xdir.split(scope.system.pathSeparator);
                        var ydirArr = ydir.split(scope.system.pathSeparator);
                        if (xdirArr.slice(-1).pop() === "") {
                            xdirArr = xdirArr.slice(0, -1);
                        }
                        if (xdirArr.length > ydirArr.length) {
                            return false;
                        }
                        return xdirArr.map(function(e, i) {
                            return xdirArr[i] === ydirArr[i];
                        }).every(function(e) { return e });
                    }

                    scope.pathIsSubFolder = false;
                    scope.otherFolder = "";
                    for (var folderID in scope.folders) {
                        if (isSubDir(scope.folders[folderID].path, viewValue)) {
                            scope.otherFolder = folderID;
                            scope.pathIsSubFolder = true;
                            break;
                        }
                    }
                    return viewValue;
                });
            }
        };
});
