angular.module('syncthing.core')
    .directive('modal', function () {
        return {
            restrict: 'E',
            templateUrl: 'modal.html',
            replace: true,
            transclude: true,
            scope: {
                heading: '@',
                status: '@',
                icon: '@',
                close: '@',
                large: '@'
            },
            link: function (scope, element, attrs, tabsCtrl) {

                // before modal show animation
                $(element).on('show.bs.modal', function () {

                    // cycle through open modals, acertain modal with highest z-index
                    var largestZ = 1040;
                    $('.modal:visible').each(function (i) {
                        var thisZ = parseInt($(this).css('zIndex'));
                        largestZ = thisZ > largestZ ? thisZ : largestZ;
                    });

                    // set this modal's z-index to be 10 above the highest z
                    var aboveLargestZ = largestZ + 10;
                    $(element).css('zIndex', aboveLargestZ);

                    // set backdrop z-index. timeout used because element does not exist immediatly
                    setTimeout(function () {
                        $('.modal-backdrop:last').css('zIndex', aboveLargestZ-5);
                    },0);

                });

                // after modal hide animation
                $(element).on('hidden.bs.modal', function () {

                    // reset z-index of modal to normal (backdrop element gets deleted, so no need to reset its z-index)
                    $(element).css('zIndex', 1040);

                });
            }
        };
    });
