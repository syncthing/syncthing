# unused-filename [![Build Status](https://travis-ci.org/sindresorhus/unused-filename.svg?branch=master)](https://travis-ci.org/sindresorhus/unused-filename)

> Get an unused filename by appending a number if it exists: `file.txt` → `file (1).txt`

Useful for safely writing, copying, moving files without overwriting existing files.


## Install

```
$ npm install --save unused-filename
```


## Usage

```
.
├── rainbow (1).txt
├── rainbow.txt
└── unicorn.txt
```

```js
const unusedFilename = require('unused-filename');

unusedFilename('rainbow.txt').then(filename => {
	console.log(filename);
	//=> 'rainbow (2).txt'
});
```


## API

### unusedFilename(filepath)

Returns a `Promise<string>`.

### unusedFilename.sync(filepath)

Returns a `string`.

#### filepath

Type: `string`


## Related

- [filenamify](https://github.com/sindresorhus/filenamify) - Convert a string to a valid safe filename


## License

MIT © [Sindre Sorhus](https://sindresorhus.com)
