angular.module('syncthing.core')
    .directive('modal', function () {
        return {
            // If you ever change any of the petroglyphs below, please search for $parent.$parent,
            // as some templates rely on the way scope is composed in this case.
            restrict: 'E',
            templateUrl: 'modal.html',
            replace: true,
            transclude: true,
            scope: {
                heading: '@',
                status: '@',
                icon: '@',
                closeable: '@',
                large: '@'
            },
            link: function (scope, element, attrs) {

                // before modal show animation
                $(element).on('show.bs.modal', function () {

                    // cycle through open modals, acertain modal with highest z-index
                    var largestZ = 1040;
                    $('.modal:visible').each(function (i) {
                        var thisZ = parseInt($(this).css('zIndex'));
                        if (thisZ > largestZ) {
                            largestZ = thisZ;
                        }
                    });

                    // set this modal's z-index to be 10 above the highest z
                    var aboveLargestZ = largestZ + 10;
                    $(element).css('zIndex', aboveLargestZ);

                    // set backdrop z-index. timeout used because element does not exist immediately
                    setTimeout(function () {
                        $('.modal-backdrop:not(:last)').removeClass('in').addClass('out');
                        $('.modal-backdrop:last').attr('for-modal-id', $(element).attr('id')).css('zIndex', aboveLargestZ - 5);
                    }, 0);

                });

                // BEFORE modal hide animation
                $(element).on('hide.bs.modal', function () {

                    // find and unhide the next backdrop down in z order
                    var sel = false, largestZ = 0;
                    $('.modal-backdrop').each(function (i) {
                        var thisZ = parseInt($(this).css('zIndex'));
                        if (thisZ > largestZ && $(this).attr('for-modal-id') !== $(element).attr('id')) {
                            largestZ = thisZ;
                            sel = i;
                        }
                    });
                    if (sel !== false) {
                        $('.modal-backdrop:eq(' + sel + ')').removeClass('out').addClass('in');
                    }
                });

                // AFTER modal hide animation
                $(element).on('hidden.bs.modal', function () {

                    // reset z-index of modal to normal
                    $(element).css('zIndex', 1040);

                    // fix scrolling by re-adding .modal-open to body
                    if ($('.modal:visible').length > 0) {
                        $('body').addClass('modal-open');
                    }

                });

                // inform syncthingContoller that a modal is ready
                scope.$parent.modalLoaded();
            }
        };
    });
