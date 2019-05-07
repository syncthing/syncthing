'use strict';

var _electron = require('electron');

var _electron2 = _interopRequireDefault(_electron);

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

var ipcRenderer = _electron2.default.ipcRenderer;


var form = document.getElementById('login-form');

form.addEventListener('submit', function (event) {
  event.preventDefault();
  var username = document.getElementById('username-input').value;
  var password = document.getElementById('password-input').value;
  ipcRenderer.send('login-message', [username, password]);
});
//# sourceMappingURL=login.js.map
