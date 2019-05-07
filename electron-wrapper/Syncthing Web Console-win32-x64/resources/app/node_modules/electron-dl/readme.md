# electron-dl [![Build Status](https://travis-ci.org/sindresorhus/electron-dl.svg?branch=master)](https://travis-ci.org/sindresorhus/electron-dl)

> Simplified file downloads for your [Electron](https://electronjs.org) app


## Why?

- One function call instead of having to manually implement a lot of [boilerplate](index.js).
- Saves the file to the users Downloads directory instead of prompting.
- Bounces the Downloads directory in the dock when done. *(macOS)*
- Handles multiple downloads.
- Shows badge count *(macOS & Linux only)* and download progress. Example on macOS:

<img src="screenshot.png" width="82">


## Install

```
$ npm install electron-dl
```


## Usage

### Register it for all windows

This is probably what you want for your app.

```js
const {app, BrowserWindow} = require('electron');
const electronDl = require('electron-dl');

electronDl();

let win;
(async () => {
	await app.whenReady();
	win = new BrowserWindow();
})();
```

### Use it manually

This can be useful if you need download functionality in a reusable module.

```js
const {BrowserWindow, ipcMain} = require('electron');
const {download} = require('electron-dl');

ipcMain.on('download-button', async (event, {url}) => {
 	const win = BrowserWindow.getFocusedWindow();
 	console.log(await download(win, url));
});
```

## API

It can only be used in the [main](https://electronjs.org/docs/glossary/#main-process) process.

### electronDl([options])

### electronDl.download(window, url, [options]): Promise<[DownloadItem](https://electronjs.org/docs/api/download-item)>

### window

Type: `BrowserWindow`

Window to register the behavior on.

### url

Type: `string`

URL to download.

### options

Type: `Object`

#### saveAs

Type: `boolean`<br>
Default: `false`

Show a `Save As…` dialog instead of downloading immediately.

Note: Only use this option when strictly necessary. Downloading directly without a prompt is a much better user experience.

#### directory

Type: `string`<br>
Default: [User's downloads directory](https://electronjs.org/docs/api/app/#appgetpathname)

Directory to save the file in.

#### filename

Type: `string`<br>
Default: [`downloadItem.getFilename()`](https://electronjs.org/docs/api/download-item/#downloaditemgetfilename)

Name of the saved file.

This option only makes sense for `electronDl.download()`.

#### errorTitle

Type: `string`<br>
Default: `Download Error`

Title of the error dialog. Can be customized for localization.

#### errorMessage

Type: `string`<br>
Default: `The download of {filename} was interrupted`

Message of the error dialog. `{filename}` is replaced with the name of the actual file. Can be customized for localization.

#### onStarted

Type: `Function`

Optional callback that receives the [download item](https://electronjs.org/docs/api/download-item).
You can use this for advanced handling such as canceling the item like `item.cancel()`.

#### onProgress

Type: `Function`

Optional callback that receives a number between `0` and `1` representing the progress of the current download.

#### onCancel

Type: `Function`

Optional callback that receives the [download item](https://electronjs.org/docs/api/download-item) for which the download has been cancelled.

#### openFolderWhenDone

Type: `boolean`<br>
Default: `false`

Reveal the downloaded file in the system file manager, and if possible, select the file.

#### showBadge

Type: `boolean`<br>
Default: `true`

Shows the file count badge on macOS/Linux dock icons when download is in progress.


## Development

After making changes, run the automated tests:

```
$ npm test
```

And before submitting a pull request, run the manual tests to manually verify that everything works:

```
npm start
```


## Related

- [electron-debug](https://github.com/sindresorhus/electron-debug) - Adds useful debug features to your Electron app
- [electron-context-menu](https://github.com/sindresorhus/electron-context-menu) - Context menu for your Electron app
- [electron-store](https://github.com/sindresorhus/electron-store) - Save and load data like user preferences, app state, cache, etc
- [electron-unhandled](https://github.com/sindresorhus/electron-unhandled) - Catch unhandled errors and promise rejections in your Electron app


## License

MIT © [Sindre Sorhus](https://sindresorhus.com)
