angular.module('syncthing.core')
    .directive('pathIsSubDir', function () {
        return {
            require: 'ngModel',
            link: function (scope, elm, attrs, ctrl) {
                ctrl.$parsers.unshift(function (viewValue) {
                    // This function checks whether ydir is a subdirectory of xdir,
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
                    scope.pathIsParentFolder = false;
                    scope.otherFolder = "";
                    scope.otherFolderLabel = "";
                    for (var folderID in scope.folders) {
                        if (isSubDir(scope.folders[folderID].path, viewValue)) {
                            scope.otherFolder = folderID;
                            scope.otherFolderLabel = scope.folders[folderID].label;
                            scope.pathIsSubFolder = true;
                            break;
                        }
                        if (viewValue !== "" &&
                            isSubDir(viewValue, scope.folders[folderID].path)) {
                            scope.otherFolder = folderID;
                            scope.otherFolderLabel = scope.folders[folderID].label;
                            scope.pathIsParentFolder = true;
                            break;
                        }
                    }
                    return viewValue;
                });
            }
        };
});
