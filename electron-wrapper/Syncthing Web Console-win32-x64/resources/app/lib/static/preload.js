'use strict';

var _electron = require('electron');

var _path = require('path');

var _path2 = _interopRequireDefault(_path);

var _fs = require('fs');

var _fs2 = _interopRequireDefault(_fs);

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

/**
 Preload file that will be executed in the renderer process
 */

/**
 * Note: This needs to be attached prior to the imports, as the they will delay
 * the attachment till after the event has been raised.
 */
document.addEventListener('DOMContentLoaded', function () {
  // Due to the early attachment, this triggers a linter error
  // because it's not yet been defined.
  // eslint-disable-next-line no-use-before-define
  injectScripts();
});

// Disable imports being first due to the above event attachment
// eslint-disable-line import/first
// eslint-disable-line import/first
// eslint-disable-line import/first

var INJECT_JS_PATH = _path2.default.join(__dirname, '../../', 'inject/inject.js');
var log = require('loglevel');
/**
 * Patches window.Notification to:
 * - set a callback on a new Notification
 * - set a callback for clicks on notifications
 * @param createCallback
 * @param clickCallback
 */
function setNotificationCallback(createCallback, clickCallback) {
  var OldNotify = window.Notification;
  var newNotify = function newNotify(title, opt) {
    createCallback(title, opt);
    var instance = new OldNotify(title, opt);
    instance.addEventListener('click', clickCallback);
    return instance;
  };
  newNotify.requestPermission = OldNotify.requestPermission.bind(OldNotify);
  Object.defineProperty(newNotify, 'permission', {
    get: function get() {
      return OldNotify.permission;
    }
  });

  window.Notification = newNotify;
}

function injectScripts() {
  var needToInject = _fs2.default.existsSync(INJECT_JS_PATH);
  if (!needToInject) {
    return;
  }
  // Dynamically require scripts
  // eslint-disable-next-line global-require, import/no-dynamic-require
  require(INJECT_JS_PATH);
}

function notifyNotificationCreate(title, opt) {
  _electron.ipcRenderer.send('notification', title, opt);
}
function notifyNotificationClick() {
  _electron.ipcRenderer.send('notification-click');
}

setNotificationCallback(notifyNotificationCreate, notifyNotificationClick);

_electron.ipcRenderer.on('params', function (event, message) {
  var appArgs = JSON.parse(message);
  log.info('nativefier.json', appArgs);
});

_electron.ipcRenderer.on('debug', function (event, message) {
  // eslint-disable-next-line no-console
  log.info('debug:', message);
});
//# sourceMappingURL=preload.js.map
