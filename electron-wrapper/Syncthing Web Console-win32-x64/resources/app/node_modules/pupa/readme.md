# pupa [![Build Status](https://travis-ci.org/sindresorhus/pupa.svg?branch=master)](https://travis-ci.org/sindresorhus/pupa)

> Simple micro templating

Useful when all you need is to fill in some placeholders.


## Install

```
$ npm install --save pupa
```


## Usage

```js
const pupa = require('pupa');

pupa('The mobile number of {name} is {phone.mobile}', {
	name: 'Sindre',
	phone: {
		mobile: '609 24 363'
	}
});
//=> 'The mobile number of Sindre is 609 24 363'

pupa('I like {0} and {1}', ['🦄', '🐮']);
//=> 'I like 🦄 and 🐮'
```


## API

### pupa(template, data)

#### template

Type: `string`

Text with placeholders for `data` properties.

#### data

Type: `Object` `Array`

Data to interpolate into `template`.


## FAQ

### What about template literals?

Template literals expand on creation. This module expands the template on execution, which can be useful if either or both template and data are lazily created or user-supplied.


## License

MIT © [Sindre Sorhus](https://sindresorhus.com)
