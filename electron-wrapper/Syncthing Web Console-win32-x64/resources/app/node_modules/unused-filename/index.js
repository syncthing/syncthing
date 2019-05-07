'use strict';
const pathExists = require('path-exists');
const modifyFilename = require('modify-filename');

const incrementer = fp => {
	let i = 0;
	return () => modifyFilename(fp, (filename, ext) => `${filename} (${++i})${ext}`);
};

module.exports = fp => {
	const getFp = incrementer(fp);
	const find = newFp => pathExists(newFp).then(x => x ? find(getFp()) : newFp);
	return find(fp);
};

module.exports.sync = fp => {
	const getFp = incrementer(fp);
	const find = newFp => pathExists.sync(newFp) ? find(getFp()) : newFp;
	return find(fp);
};
