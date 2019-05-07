export interface Options {
	/**
	 * Show a `Save As…` dialog instead of downloading immediately.
	 *
	 * Note: Only use this option when strictly necessary. Downloading directly without a prompt is a much better user experience.
	 *
	 * @default false
	 */
	saveAs?: boolean;

	/**
	 * Directory to save the file in.
	 *
	 * Default: [User's downloads directory](https://electronjs.org/docs/api/app/#appgetpathname)
	 */
	directory?: string;

	/**
	 * Name of the saved file.
	 * This option only makes sense for `electronDl.download()`.
	 *
	 * Default: [`downloadItem.getFilename()`](https://electronjs.org/docs/api/download-item/#downloaditemgetfilename)
	 */
	filename?: string;

	/**
	 * Title of the error dialog. Can be customized for localization.
	 *
	 * @default 'Download Error'
	 */
	errorTitle?: string;

	/**
	 * Message of the error dialog. `{filename}` is replaced with the name of the actual file. Can be customized for localization.
	 *
	 * @default 'The download of {filename} was interrupted'
	 */
	errorMessage?: string;

	/**
	 * Optional callback that receives the [download item](https://electronjs.org/docs/api/download-item).
	 * You can use this for advanced handling such as canceling the item like `item.cancel()`.
	 */
	onStarted?: (item: Electron.DownloadItem) => void;

	/**
	 * Optional callback that receives a number between `0` and `1` representing the progress of the current download.
	 */
	onProgress?: (percent: number) => void;

	/**
	 * Optional callback that receives the [download item](https://electronjs.org/docs/api/download-item) for which the download has been cancelled.
	 */
	onCancel?: (item: Electron.DownloadItem) => void;

	/**
	 * Reveal the downloaded file in the system file manager, and if possible, select the file.
	 *
	 * @default false
	 */
	openFolderWhenDone?: boolean;

	/**
	 * Shows the file count badge on macOS/Linux dock icons when download is in progress.
	 *
	 * @default true
	 */
	showBadge?: boolean;
}

/**
 * Register the helper for all windows.
 *
 * @example
 *
 * import {app, BrowserWindow} from 'electron';
 * import electronDl from 'electron-dl';
 *
 * electronDl();
 *
 * let win;
 * app.on('ready', () => {
 * 	win = new BrowserWindow();
 * });
 */
export default function electronDl(options?: Options): void;

/**
 * This can be useful if you need download functionality in a reusable module.
 *
 * @param window - Window to register the behavior on.
 * @param url - URL to download.
 * @param options
 * @returns A promise for the downloaded file.
 *
 * @example
 *
 * import {BrowserWindow, ipcMain} from 'electron';
 * import {download} from 'electron-dl';
 *
 * ipcMain.on('download-button', async (event, {url}) => {
 * 	const win = BrowserWindow.getFocusedWindow();
 * 	console.log(await download(win, url));
 * });
 */
export function download(
	window: Electron.BrowserWindow,
	url: string,
	options?: Options
): Promise<Electron.DownloadItem>;
