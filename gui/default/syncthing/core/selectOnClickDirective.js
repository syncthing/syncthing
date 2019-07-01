angular.module('syncthing.core')
    .directive('selectOnClick', function ($window) {
        return {
            link: function (scope, element, attrs) {
                element.on('click', function () {
                    var selection = $window.getSelection();
                    var range = document.createRange();
                    range.selectNodeContents(element[0]);
                    selection.removeAllRanges();
                    selection.addRange(range);
                });
            }
        };
    });
