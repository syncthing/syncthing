function toggleCurrent (elem) {
    var parent_li = elem.closest('li');
    parent_li.siblings('li.current').removeClass('current');
    parent_li.siblings().find('li.current').removeClass('current');
    parent_li.find('> ul li.current').removeClass('current');
    parent_li.toggleClass('current');
}

$(document).ready(function() {
    // Shift nav in mobile when clicking the menu.
    $(document).on('click', "[data-toggle='wy-nav-top']", function() {
        $("[data-toggle='wy-nav-shift']").toggleClass("shift");
        $("[data-toggle='rst-versions']").toggleClass("shift");
    });
    // Nav menu link click operations
    $(document).on('click', ".wy-menu-vertical .current ul li a", function() {
        var target = $(this);
        // Close menu when you click a link.
        $("[data-toggle='wy-nav-shift']").removeClass("shift");
        $("[data-toggle='rst-versions']").toggleClass("shift");
        // Handle dynamic display of l3 and l4 nav lists
        toggleCurrent(target);
        if (typeof(window.SphinxRtdTheme) != 'undefined') {
            window.SphinxRtdTheme.StickyNav.hashChange();
        }
    });
    $(document).on('click', "[data-toggle='rst-current-version']", function() {
        $("[data-toggle='rst-versions']").toggleClass("shift-up");
    });
    // Make tables responsive
    $("table.docutils:not(.field-list)").wrap("<div class='wy-table-responsive'></div>");

    // Add expand links to all parents of nested ul
    $('.wy-menu-vertical ul').siblings('a').each(function () {
        var link = $(this);
            expand = $('<span class="toctree-expand"></span>');
        expand.on('click', function (ev) {
            toggleCurrent(link);
            ev.stopPropagation();
            return false;
        });
        link.prepend(expand);
    });
});

// Sphinx theme state
window.SphinxRtdTheme = (function (jquery) {
    var stickyNav = (function () {
        var navBar,
            win,
            winScroll = false,
            linkScroll = false,
            winPosition = 0,
            enable = function () {
                init();
                reset();
                win.on('hashchange', reset);

                // Set scrolling
                win.on('scroll', function () {
                    if (!linkScroll) {
                        winScroll = true;
                    }
                });
                setInterval(function () {
                    if (winScroll) {
                        winScroll = false;
                        var newWinPosition = win.scrollTop(),
                            navPosition = navBar.scrollTop(),
                            newNavPosition = navPosition + (newWinPosition - winPosition);
                        navBar.scrollTop(newNavPosition);
                        winPosition = newWinPosition;
                    }
                }, 25);
            },
            init = function () {
                navBar = jquery('nav.wy-nav-side:first');
                win = jquery(window);
            },
            reset = function () {
                // Get anchor from URL and open up nested nav
                var anchor = encodeURI(window.location.hash);
                if (anchor) {
                    try {
                        var link = $('.wy-menu-vertical')
                            .find('[href="' + anchor + '"]');
                        $('.wy-menu-vertical li.toctree-l1 li.current')
                            .removeClass('current');
                        link.closest('li.toctree-l2').addClass('current');
                        link.closest('li.toctree-l3').addClass('current');
                        link.closest('li.toctree-l4').addClass('current');
                    }
                    catch (err) {
                        console.log("Error expanding nav for anchor", err);
                    }
                }
            },
            hashChange = function () {
                linkScroll = true;
                win.one('hashchange', function () {
                    linkScroll = false;
                });
            };
        jquery(init);
        return {
            enable: enable,
            hashChange: hashChange
        };
    }());
    return {
        StickyNav: stickyNav
    };
}($));
