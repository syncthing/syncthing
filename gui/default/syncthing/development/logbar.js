'use strict';

function intercept(method, handler) {
    var console = window.console;
    var original = console[method];
    console[method] = function () {
        handler(method);
        // do sneaky stuff
        if (original.apply) {
            // Do this for normal browsers
            original.apply(console, arguments);
        } else {
            // Do this for IE
            var message = Array.prototype.slice.apply(arguments).join(' ');
            original(message);
        }
    };
}

function handleConsoleCall(type) {
    var element = document.querySelector('#log_' + type);
    if (element) {
        if (!element.classList.contains("hasCount")) {
            element.classList.add("hasCount");
        }

        var devTopBar = document.querySelector('#dev-top-bar');
        devTopBar.style.display = 'block';

        element.innerHTML = parseInt(element.innerHTML) + 1;
    }
}

if (window.console) {
    var methods = ['error', 'warn'];
    for (var i = 0; i < methods.length; i++) {
        intercept(methods[i], handleConsoleCall);
    }
}