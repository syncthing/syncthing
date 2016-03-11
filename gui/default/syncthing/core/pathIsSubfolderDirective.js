angular.module('syncthing.core')
    .directive('pathIsSubfolder', function () {
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

                    // check whether the directory in question is a subdirectory of any other
                    var flag = false;
                    var oldPath = "";
                    for (var folderID in scope.folders) {
                        oldPath = scope.folders[folderID].path;
                        if (isSubDir(viewValue, oldPath) || isSubDir(oldPath, viewValue)) {
                            flag = true;
                            break;
                        }
                    }

                    if (flag) {
                        scope.otherPath = oldPath;
                        ctrl.$setValidity('pathIsSubfolder', false);
                    } else {
                        ctrl.$setValidity('pathIsSubfolder', true);
                    }
                    return viewValue;
                });
            }
        };
});
