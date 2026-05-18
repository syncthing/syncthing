// Event long-polling service for Syncthing
// Mirrors the AngularJS eventService.js

import { api } from './api.js';

// Event types emitted by this service
export const ONLINE = 'UIOnline';
export const OFFLINE = 'UIOffline';

// Event types emitted by Syncthing
export const CONFIG_SAVED = 'ConfigSaved';
export const DEVICE_CONNECTED = 'DeviceConnected';
export const DEVICE_DISCONNECTED = 'DeviceDisconnected';
export const DEVICE_DISCOVERED = 'DeviceDiscovered';
export const PENDING_DEVICES_CHANGED = 'PendingDevicesChanged';
export const PENDING_FOLDERS_CHANGED = 'PendingFoldersChanged';
export const DEVICE_PAUSED = 'DevicePaused';
export const DEVICE_RESUMED = 'DeviceResumed';
export const DOWNLOAD_PROGRESS = 'DownloadProgress';
export const FAILURE = 'Failure';
export const UPGRADE_RESTART_SCHEDULED = 'UpgradeRestartScheduled';
export const FOLDER_COMPLETION = 'FolderCompletion';
export const FOLDER_SUMMARY = 'FolderSummary';
export const FOLDER_ERRORS = 'FolderErrors';
export const FOLDER_SCAN_PROGRESS = 'FolderScanProgress';
export const FOLDER_PAUSED = 'FolderPaused';
export const FOLDER_RESUMED = 'FolderResumed';
export const STATE_CHANGED = 'StateChanged';
export const LOCAL_INDEX_UPDATED = 'LocalIndexUpdated';
export const ITEM_FINISHED = 'ItemFinished';
export const ITEM_STARTED = 'ItemStarted';
export const LOCAL_CHANGE_DETECTED = 'LocalChangeDetected';
export const REMOTE_CHANGE_DETECTED = 'RemoteChangeDetected';
export const REMOTE_INDEX_UPDATED = 'RemoteIndexUpdated';

const listeners = new Map();
let lastID = 0;
let running = false;
let abortController = null;

export function on(eventType, callback) {
  if (!listeners.has(eventType)) {
    listeners.set(eventType, new Set());
  }
  listeners.get(eventType).add(callback);
  return () => {
    const set = listeners.get(eventType);
    if (set) {
      set.delete(callback);
      if (set.size === 0) listeners.delete(eventType);
    }
  };
}

function emit(eventType, data) {
  const set = listeners.get(eventType);
  if (set) {
    for (const cb of set) {
      try {
        cb(data);
      } catch (e) {
        console.error('Event handler error:', eventType, e);
      }
    }
  }
}

async function poll() {
  if (!running) return;

  try {
    const params = lastID > 0 ? { since: lastID } : { limit: 1 };
    const url = lastID > 0
      ? 'events?since=' + lastID
      : 'events?limit=1';

    const data = await api.get(url);

    if (!data || !running) {
      throw new Error('Empty response');
    }

    emit(ONLINE);

    if (lastID > 0) {
      // Don't emit events from the first response
      for (const event of data) {
        emit(event.type, event);
      }
    }

    const lastEvent = data[data.length - 1];
    if (lastEvent) {
      lastID = lastEvent.id;
    }

    // Immediately poll again
    if (running) {
      poll();
    }
  } catch (err) {
    if (!running) return;

    if (err.status === 403) {
      location.reload();
      return;
    }

    emit(OFFLINE);

    // Retry after 1 second
    setTimeout(() => {
      if (running) poll();
    }, 1000);
  }
}

export function start() {
  if (running) return;
  running = true;
  lastID = 0;
  poll();
}

export function stop() {
  running = false;
  if (abortController) {
    abortController.abort();
    abortController = null;
  }
}
