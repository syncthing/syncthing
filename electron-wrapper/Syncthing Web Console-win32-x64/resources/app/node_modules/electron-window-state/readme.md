# electron-window-state [![Build Status](https://travis-ci.org/mawie81/electron-window-state.svg)](https://travis-ci.org/mawie81/electron-window-state)

> A library to store and restore window sizes and positions for your
[Electron](http://electron.atom.io) app

*Heavily influenced by the implementation in [electron-boilerplate](https://github.com/szwacz/electron-boilerplate).*

## Install

```
$ npm install --save electron-window-state
```

## Usage

```js
const windowStateKeeper = require('electron-window-state');
let win;

app.on('ready', function () {
  // Load the previous state with fallback to defaults
  let mainWindowState = windowStateKeeper({
    defaultWidth: 1000,
    defaultHeight: 800
  });

  // Create the window using the state information
  win = new BrowserWindow({
    'x': mainWindowState.x,
    'y': mainWindowState.y,
    'width': mainWindowState.width,
    'height': mainWindowState.height
  });

  // Let us register listeners on the window, so we can update the state
  // automatically (the listeners will be removed when the window is closed)
  // and restore the maximized or full screen state
  mainWindowState.manage(win);
});
```

## API

#### windowStateKeeper(opts)

Note: Don't call this function before the `ready` event is fired.

##### opts

`defaultWidth` - *Number*

  The width that should be returned if no file exists yet. Defaults to `800`.

`defaultHeight` - *Number*

  The height that should be returned if no file exists yet. Defaults to `600`.

`path` - *String*

  The path where the state file should be written to. Defaults to
  `app.getPath('userData')`

`file` - *String*

  The name of file. Defaults to `window-state.json`

`maximize` - *Boolean*

  Should we automatically maximize the window, if it was last closed
  maximized. Defaults to `true`

`fullScreen` - *Boolean*

  Should we automatically restore the window to full screen, if it was last
  closed full screen. Defaults to `true`

### state object

```js
const windowState = windowStateKeeper({
  defaultWidth: 1000,
  defaultHeight: 800
});
```

`x` - *Number*

  The saved `x` coordinate of the loaded state. `undefined` if the state has not
  been saved yet.

`y` - *Number*

  The saved `y` coordinate of the loaded state. `undefined` if the state has not
  been saved yet.

`width` - *Number*

  The saved `width` of loaded state. `defaultWidth` if the state has not been
  saved yet.

`height` - *Number*

  The saved `heigth` of loaded state. `defaultHeight` if the state has not been
  saved yet.

`isMaximized` - *Boolean*

  `true` if the window state was saved while the the window was maximized.
  `undefined` if the state has not been saved yet.

`isFullScreen` - *Boolean*

  `true` if the window state was saved while the the window was in full screen
  mode. `undefined` if the state has not been saved yet.

`manage(window)` - *Function*

  Register listeners on the given `BrowserWindow` for events that are
  related to size or position changes (`resize`, `move`). It will also restore
  the window's maximized or full screen state.
  When the window is closed we automatically remove the listeners and save the
  state.

`unmanage` - *Function*

  Removes all listeners of the managed `BrowserWindow` in case it does not
  need to be managed anymore.

`saveState(window)` - *Function*

  Saves the current state of the given `BrowserWindow`. This exists mostly for
  legacy purposes, and in most cases it's better to just use `manage`.

## License

MIT © [Marcel Wiehle](http://marcel.wiehle.me)
