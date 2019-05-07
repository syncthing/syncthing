'use strict';
const electron = require('electron');
const {download} = require('electron-dl');
const isDev = require('electron-is-dev');

const webContents = win => win.webContents || win.getWebContents();

function create(win, options) {
	webContents(win).on('context-menu', (event, props) => {
		if (typeof options.shouldShowMenu === 'function' && options.shouldShowMenu(event, props) === false) {
			return;
		}

		const {editFlags} = props;
		const hasText = props.selectionText.trim().length > 0;
		const can = type => editFlags[`can${type}`] && hasText;

		let menuTpl = [{
			type: 'separator'
		}, {
			id: 'cut',
			label: 'Cut',
			// Needed because of macOS limitation:
			// https://github.com/electron/electron/issues/5860
			role: can('Cut') ? 'cut' : '',
			enabled: can('Cut'),
			visible: props.isEditable
		}, {
			id: 'copy',
			label: 'Copy',
			role: can('Copy') ? 'copy' : '',
			enabled: can('Copy'),
			visible: props.isEditable || hasText
		}, {
			id: 'paste',
			label: 'Paste',
			role: editFlags.canPaste ? 'paste' : '',
			enabled: editFlags.canPaste,
			visible: props.isEditable
		}, {
			type: 'separator'
		}];

		if (props.mediaType === 'image') {
			menuTpl = [{
				type: 'separator'
			}, {
				id: 'save',
				label: 'Save Image',
				click(item, win) {
					download(win, props.srcURL);
				}
			}];

			if (options.showSaveImageAs) {
				menuTpl.push({
					id: 'saveImageAs',
					label: 'Save Image As…',
					click(item, win) {
						download(win, props.srcURL, {saveAs: true});
					}
				});
			}

			menuTpl.push({
				type: 'separator'
			});
		}

		if (props.linkURL && props.mediaType === 'none') {
			menuTpl = [{
				type: 'separator'
			}, {
				id: 'copyLink',
				label: 'Copy Link',
				click() {
					electron.clipboard.write({
						bookmark: props.linkText,
						text: props.linkURL
					});
				}
			}, {
				type: 'separator'
			}];
		}

		if (options.showCopyImageAddress && props.mediaType === 'image') {
			menuTpl.push({
				type: 'separator'
			}, {
				id: 'copyImageAddress',
				label: 'Copy Image Address',
				click() {
					electron.clipboard.write({
						bookmark: props.srcURL,
						text: props.srcURL
					});
				}
			}, {
				type: 'separator'
			});
		}

		if (options.prepend) {
			const result = options.prepend(props, win);

			if (Array.isArray(result)) {
				menuTpl.unshift(...result);
			}
		}

		if (options.append) {
			const result = options.append(props, win);

			if (Array.isArray(result)) {
				menuTpl.push(...result);
			}
		}

		if (options.showInspectElement || (options.showInspectElement !== false && isDev)) {
			menuTpl.push({
				type: 'separator'
			}, {
				id: 'inspect',
				label: 'Inspect Element',
				click() {
					win.inspectElement(props.x, props.y);

					if (webContents(win).isDevToolsOpened()) {
						webContents(win).devToolsWebContents.focus();
					}
				}
			}, {
				type: 'separator'
			});
		}

		// Apply custom labels for default menu items
		if (options.labels) {
			for (const menuItem of menuTpl) {
				if (options.labels[menuItem.id]) {
					menuItem.label = options.labels[menuItem.id];
				}
			}
		}

		// Filter out leading/trailing separators
		// TODO: https://github.com/electron/electron/issues/5869
		menuTpl = delUnusedElements(menuTpl);

		if (menuTpl.length > 0) {
			const menu = (electron.remote ? electron.remote.Menu : electron.Menu).buildFromTemplate(menuTpl);

			/*
			 * When electron.remote is not available this runs in the browser process.
			 * We can safely use win in this case as it refers to the window the
			 * context-menu should open in.
			 * When this is being called from a webView, we can't use win as this
			 * would refere to the webView which is not allowed to render a popup menu.
			 */
			menu.popup(electron.remote ? electron.remote.getCurrentWindow() : win);
		}
	});
}

function delUnusedElements(menuTpl) {
	let notDeletedPrevEl;
	return menuTpl.filter(el => el.visible !== false).filter((el, i, array) => {
		const toDelete = el.type === 'separator' && (!notDeletedPrevEl || i === array.length - 1 || array[i + 1].type === 'separator');
		notDeletedPrevEl = toDelete ? notDeletedPrevEl : el;
		return !toDelete;
	});
}

module.exports = (options = {}) => {
	if (options.window) {
		const win = options.window;
		const wc = webContents(win);

		// When window is a webview that has not yet finished loading webContents is not available
		if (wc === undefined) {
			win.addEventListener('dom-ready', () => {
				create(win, options);
			}, {once: true});
			return;
		}

		return create(win, options);
	}

	for (const win of (electron.BrowserWindow || electron.remote.BrowserWindow).getAllWindows()) {
		create(win, options);
	}

	(electron.app || electron.remote.app).on('browser-window-created', (event, win) => {
		create(win, options);
	});
};
