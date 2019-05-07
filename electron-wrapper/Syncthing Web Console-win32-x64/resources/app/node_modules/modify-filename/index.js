'use strict';
var path = require('path');

module.exports = function modifyFilename(pth, modifier) {
	if (arguments.length !== 2) {
		throw new Error('`path` and `modifier` required');
	}

	if (Array.isArray(pth)) {
		return pth.map(function (el) {
			return modifyFilename(el, modifier);
		});
	}

	var ext = path.extname(pth);
	return path.join(path.dirname(pth), modifier(path.basename(pth, ext), ext));
};
