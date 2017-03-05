// See https://github.com/zenorocha/clipboard.js/issues/155#issuecomment-217372642
$.fn.modal.Constructor.prototype.enforceFocus = function() {};

// Enable click to clipboard
new Clipboard('.copy-to-clipboard');
