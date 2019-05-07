# electron-context-menu [![Build Status](https://travis-ci.org/sindresorhus/electron-context-menu.svg?branch=master)](https://travis-ci.org/sindresorhus/electron-context-menu)

> Context menu for your [Electron](https://electronjs.org) app

<img src="screenshot.png" width="125" align="right">

Electron doesn't have a built-in context menu. You're supposed to handle that yourself. But it's both tedious and hard to get right. This module gives you a nice extensible context menu with items like `Cut`/`Copy`/`Paste` for text, `Save Image` for images, and `Copy Link` for links. It also adds an `Inspect Element` menu item when in development to quickly view items in the inspector like in Chrome.

You can use this module directly in both the main and renderer process.


## Install

```
$ npm install electron-context-menu
```

*Requires Electron 2.0.0 or later.*

<a href="https://www.patreon.com/sindresorhus">
	<img src="https://c5.patreon.com/external/logo/become_a_patron_button@2x.png" width="160">
</a>


## Usage

```js
const {app, BrowserWindow} = require('electron');

require('electron-context-menu')({
	prepend: (params, browserWindow) => [{
		label: 'Rainbow',
		// Only show it when right-clicking images
		visible: params.mediaType === 'image'
	}]
});

let mainWindow;
app.on('ready', () => {
	mainWindow = new BrowserWindow();
});
```


## API

### contextMenu([options])

### options

Type: `Object`

#### window

Type: `BrowserWindow` `WebView`<br>

Window or WebView to add the context menu to.

When not specified, the context menu will be added to all existing and new windows.

#### prepend

Type: `Function`

Should return an array of [MenuItem](https://electronjs.org/docs/api/menu-item)'s to be prepended to the context menu. The first argument is [this `params` object](https://electronjs.org/docs/api/web-contents#event-context-menu). The second argument is the [BrowserWindow](https://electronjs.org/docs/api/browser-window) the context menu was requested for.

#### append

Type: `Function`

Should return an array of [MenuItem](https://electronjs.org/docs/api/browser-window)'s to be appended to the context menu. The first argument is [this `params` object](https://electronjs.org/docs/api/browser-window). The second argument is the [BrowserWindow](https://electronjs.org/docs/api/browser-window) the context menu was requested for.

#### showCopyImageAddress

Type: `boolean`<br>
Default: `false`

Show the `Copy Image Address` menu item when right-clicking on an image.

#### showSaveImageAs

Type: `boolean`<br>
Default: `false`

Show the `Save Image As…` menu item when right-clicking on an image.

#### showInspectElement

Type: `boolean`<br>
Default: [Only in development](https://github.com/sindresorhus/electron-is-dev)

Force enable or disable the `Inspect Element` menu item.

#### labels

Type: `Object`<br>
Default: `{}`

Overwrite labels for the default menu items. Useful for i18n.

Format:

```js
labels: {
	cut: 'Configured Cut',
	copy: 'Configured Copy',
	paste: 'Configured Paste',
	save: 'Configured Save Image',
	saveImageAs: 'Configured Save Image As…'
	copyLink: 'Configured Copy Link',
	copyImageAddress: 'Configured Copy Image Address',
	inspect: 'Configured Inspect'
}
```

#### shouldShowMenu

Type: `Function`

Determines whether or not to show the menu. Can be useful if you for example have other code presenting a context menu in some contexts. The second argument is [this `params` object](https://electronjs.org/docs/api/web-contents#event-context-menu).

Example:

```js
// Doesn't show the menu if the element is editable
shouldShowMenu: (event, params) => !params.isEditable
```

## Related

- [electron-util](https://github.com/sindresorhus/electron-util) - Useful utilities for developing Electron apps and modules
- [electron-debug](https://github.com/sindresorhus/electron-debug) - Adds useful debug features to your Electron app
- [electron-store](https://github.com/sindresorhus/electron-store) - Save and load data like user preferences, app state, cache, etc
- [electron-reloader](https://github.com/sindresorhus/electron-reloader) - Simple auto-reloading for Electron apps during development
- [electron-serve](https://github.com/sindresorhus/electron-serve) - Static file serving for Electron apps
- [electron-unhandled](https://github.com/sindresorhus/electron-unhandled) - Catch unhandled errors and promise rejections in your Electron app


## License

MIT © [Sindre Sorhus](https://sindresorhus.com)
