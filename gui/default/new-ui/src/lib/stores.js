// Svelte stores for Syncthing shared state
// Uses Svelte 5 runes ($state) via writable stores for cross-component sharing

import { writable, derived, get } from 'svelte/store';
import { api, initCSRF } from './api.js';
import * as events from './events.js';
import { autoConfigLocale } from './i18n.js';

// ===== Core State Stores =====

export const authenticated = writable(false);
export const config = writable({});
export const configInSync = writable(true);
export const myID = writable('');
export const system = writable(null);
export const version = writable({});
export const connections = writable({});
export const connectionsTotal = writable({ inbps: 0, outbps: 0, inBytesTotal: 0, outBytesTotal: 0 });
export const completion = writable({});
export const devices = writable({});
export const devicesGrouped = writable({});
export const folders = writable({});
export const foldersGrouped = writable({});
export const model = writable({});
export const errors = writable([]);
export const seenError = writable('');
export const pendingDevices = writable({});
export const pendingFolders = writable({});
export const deviceStats = writable({});
export const folderStats = writable({});
export const progress = writable({});
export const scanProgress = writable({});
export const discoveryCache = writable({});
export const upgradeInfo = writable(null);
export const themes = writable([]);
export const globalChangeEvents = writable([]);
export const metricRates = writable(false);
export const online = writable(false);
export const restarting = writable(false);

// Listeners/Discovery status
export const listenersFailed = writable([]);
export const listenersRunning = writable([]);
export const listenersTotal = writable(0);
export const discoveryFailed = writable([]);
export const discoveryRunning = writable([]);
export const discoveryTotal = writable(0);

// ===== Derived Stores =====

export const localStateTotal = derived(model, ($model) => {
  const total = { bytes: 0, directories: 0, files: 0 };
  for (const f in $model) {
    if ($model[f]) {
      total.bytes += $model[f].localBytes || 0;
      total.files += $model[f].localFiles || 0;
      total.directories += $model[f].localDirectories || 0;
    }
  }
  return total;
});

export const openNoAuth = derived([system, config], ([$system, $config]) => {
  if (!$system || !$config || !$config.gui) return false;
  const addr = $system.guiAddressUsed || '';
  const guiCfg = $config.gui;
  const isAuthEnabled = guiCfg.authMode === 'ldap' || (guiCfg.user && guiCfg.password);
  return addr.substr(0, 4) !== '127.'
    && addr.substr(0, 6) !== '[::1]:'
    && addr.substr(0, 1) !== '/'
    && !isAuthEnabled
    && !guiCfg.insecureAdminAccess;
});

export const errorList = derived([errors, seenError], ([$errors, $seenError]) => {
  if (!$errors) return [];
  return $errors.filter(e => e.when > $seenError);
});

export const otherDevicesList = derived([devices, myID], ([$devices, $myID]) => {
  return Object.values($devices)
    .filter(d => d.deviceID !== $myID)
    .sort(deviceCompare);
});

export const folderList = derived(folders, ($folders) => {
  return Object.values($folders).sort(folderCompare);
});

export const deviceList = derived(devices, ($devices) => {
  return Object.values($devices).sort(deviceCompare);
});

// ===== Helper functions =====

const shortIDStringLength = 7;

export function deviceCompare(a, b) {
  if (a.name && b.name) {
    if (a.name < b.name) return -1;
    if (a.name > b.name) return 1;
    return 0;
  }
  if (a.deviceID < b.deviceID) return -1;
  if (a.deviceID > b.deviceID) return 1;
  return 0;
}

export function folderCompare(a, b) {
  const labelA = (a.label && a.label.length > 0) ? a.label : a.id;
  const labelB = (b.label && b.label.length > 0) ? b.label : b.id;
  if (labelA < labelB) return -1;
  if (labelA > labelB) return 1;
  return 0;
}

function deviceMap(list) {
  const m = {};
  if (list) list.forEach(r => { m[r.deviceID] = r; });
  return m;
}

function folderMap(list) {
  const m = {};
  if (list) list.forEach(r => { m[r.id] = r; });
  return m;
}

function sortByKeyThenProperty(obj, prop, fallbackProp) {
  const sorted = {};
  Object.keys(obj).sort().forEach(key => {
    sorted[key] = obj[key].sort((a, b) => {
      const aProp = a[prop] ? prop : fallbackProp;
      const bProp = b[prop] ? prop : fallbackProp;
      return (a[aProp] || '').localeCompare(b[bProp] || '');
    });
  });
  return sorted;
}

// ===== Debounce utility =====

function debounce(func, wait) {
  let timeout, args, context, timestamp, result, again;
  const later = () => {
    const last = Date.now() - timestamp;
    if (last < wait) {
      timeout = setTimeout(later, wait - last);
    } else {
      timeout = null;
      if (again) {
        again = false;
        result = func.apply(context, args);
        context = args = null;
      }
    }
  };
  return function (...callArgs) {
    context = this;
    args = callArgs;
    timestamp = Date.now();
    if (!timeout) {
      timeout = setTimeout(later, wait);
      result = func.apply(context, args);
      context = args = null;
    } else {
      again = true;
    }
    return result;
  };
}

// ===== State updaters =====

let prevDate = 0;

export function updateLocalConfig(cfg) {
  config.set(cfg);

  const devMap = deviceMap(cfg.devices);
  devices.set(devMap);

  // Init completion for all devices
  const comp = get(completion);
  for (const id in devMap) {
    if (!comp[id]) {
      comp[id] = { _total: 100, _needBytes: 0, _needItems: 0 };
    }
  }
  completion.set(comp);

  const fMap = folderMap(cfg.folders);
  folders.set(fMap);

  // Refresh folder status for each folder
  const $myID = get(myID);
  for (const fid in fMap) {
    refreshFolderStatus(fid);
    if (fMap[fid].devices) {
      fMap[fid].devices.forEach(devCfg => {
        if (devCfg.deviceID !== $myID) {
          refreshCompletion(devCfg.deviceID, fid);
        }
      });
    }
  }

  // Build grouped folders
  const fGrouped = {};
  Object.values(fMap).forEach(f => {
    const group = f.group || '';
    if (!fGrouped[group]) fGrouped[group] = [];
    fGrouped[group].push(f);
  });
  foldersGrouped.set(sortByKeyThenProperty(fGrouped, 'label', 'id'));

  // Build grouped devices (need myID)
  if ($myID) {
    const dGrouped = {};
    Object.values(devMap)
      .filter(d => d.deviceID !== $myID)
      .forEach(d => {
        const group = d.group || '';
        if (!dGrouped[group]) dGrouped[group] = [];
        dGrouped[group].push(d);
      });
    devicesGrouped.set(sortByKeyThenProperty(dGrouped, 'name', 'deviceID'));
  }
}

const debouncedFolderRefreshes = {};

function refreshFolderStatus(folder) {
  const $folders = get(folders);
  if ($folders[folder] && $folders[folder].paused) return;

  if (!debouncedFolderRefreshes[folder]) {
    debouncedFolderRefreshes[folder] = debounce(async () => {
      try {
        const data = await api.getFolderStatus(folder);
        model.update(m => ({ ...m, [folder]: data }));
      } catch (e) {
        console.error('refreshFolderStatus error:', folder, e);
      }
    }, 1000);
  }
  debouncedFolderRefreshes[folder]();
}

export function refreshCompletion(device, folder) {
  const $myID = get(myID);
  if (device === $myID) return;

  api.getCompletion(device, folder).then(data => {
    completion.update(comp => {
      if (!comp[device]) comp[device] = {};
      comp[device][folder] = data;
      recalcCompletion(comp, device);
      return { ...comp };
    });
  }).catch(err => {
    if (err.status !== 404) {
      console.error('refreshCompletion error:', err);
    }
  });
}

function recalcCompletion(comp, device) {
  let total = 0, needed = 0, deletes = 0, items = 0;
  for (const folder in comp[device]) {
    if (folder === '_total' || folder === '_needBytes' || folder === '_needItems') continue;
    const c = comp[device][folder];
    total += c.globalBytes || 0;
    needed += c.needBytes || 0;
    items += c.needItems || 0;
    deletes += c.needDeletes || 0;
  }
  if (total === 0) {
    comp[device]._total = 100;
    comp[device]._needBytes = 0;
    comp[device]._needItems = 0;
  } else {
    comp[device]._total = Math.floor(100 * (1 - needed / total));
    comp[device]._needBytes = needed;
    comp[device]._needItems = items + deletes;
  }
  if (needed === 0 && deletes + items > 0) {
    comp[device]._total = 95;
  }
}

export const refreshSystem = async () => {
  try {
    const data = await api.getSystemStatus();
    myID.set(data.myID);
    system.set(data);

    const lFailed = [];
    const lRunning = [];
    for (const address in data.connectionServiceStatus) {
      if (data.connectionServiceStatus[address].error) {
        lFailed.push(address + ': ' + data.connectionServiceStatus[address].error);
      } else {
        lRunning.push(address);
      }
    }
    listenersFailed.set(lFailed);
    listenersRunning.set(lRunning);
    listenersTotal.set(Object.keys(data.connectionServiceStatus || {}).length);

    const dFailed = [];
    const dRunning = [];
    for (const disco in data.discoveryStatus) {
      if (data.discoveryStatus[disco] && data.discoveryStatus[disco].error) {
        dFailed.push(disco + ': ' + data.discoveryStatus[disco].error);
      } else {
        dRunning.push(disco);
      }
    }
    discoveryFailed.set(dFailed);
    discoveryRunning.set(dRunning);
    discoveryTotal.set(Object.keys(data.discoveryStatus || {}).length);
  } catch (e) {
    console.error('refreshSystem error:', e);
    throw e;
  }
};

export const refreshConfig = async () => {
  try {
    const [cfg, insync] = await Promise.all([
      api.getConfig(),
      api.getConfigInsync(),
    ]);
    updateLocalConfig(cfg);
    configInSync.set(insync.configInSync);
  } catch (e) {
    console.error('refreshConfig error:', e);
    throw e;
  }
};

export const refreshConnectionStats = async () => {
  try {
    const data = await api.getSystemConnections();
    const now = Date.now();
    const td = (now - prevDate) / 1000;
    prevDate = now;

    const $connectionsTotal = get(connectionsTotal);
    try {
      data.total.inbps = Math.max(0, (data.total.inBytesTotal - $connectionsTotal.inBytesTotal) / td);
      data.total.outbps = Math.max(0, (data.total.outBytesTotal - $connectionsTotal.outBytesTotal) / td);
    } catch (e) {
      data.total.inbps = 0;
      data.total.outbps = 0;
    }
    connectionsTotal.set(data.total);

    const conns = data.connections;
    const $connections = get(connections);
    for (const id in conns) {
      try {
        conns[id].inbps = Math.max(0, (conns[id].inBytesTotal - ($connections[id] ? $connections[id].inBytesTotal : conns[id].inBytesTotal)) / td);
        conns[id].outbps = Math.max(0, (conns[id].outBytesTotal - ($connections[id] ? $connections[id].outBytesTotal : conns[id].outBytesTotal)) / td);
      } catch (e) {
        conns[id].inbps = 0;
        conns[id].outbps = 0;
      }
    }
    connections.set(conns);
  } catch (e) {
    console.error('refreshConnectionStats error:', e);
    throw e;
  }
};

export const refreshDiscoveryCache = async () => {
  try {
    const data = await api.getSystemDiscovery();
    for (const device in data) {
      for (let i = 0; i < data[device].addresses.length; i++) {
        data[device].addresses[i] = data[device].addresses[i].replace(/\/\?.*/, '');
      }
    }
    discoveryCache.set(data);
  } catch (e) {
    console.error('refreshDiscoveryCache error:', e);
  }
};

export const refreshErrors = async () => {
  try {
    const data = await api.getSystemError();
    errors.set(data.errors || []);
  } catch (e) {
    console.error('refreshErrors error:', e);
  }
};

export const refreshDeviceStats = debounce(async () => {
  try {
    const data = await api.getDeviceStats();
    for (const device in data) {
      data[device].lastSeen = new Date(data[device].lastSeen);
      if (data[device].lastSeen.toISOString() !== '1970-01-01T00:00:00.000Z') {
        data[device].lastSeenDays = (new Date() - data[device].lastSeen) / 1000 / 86400;
      }
    }
    deviceStats.set(data);
  } catch (e) {
    console.error('refreshDeviceStats error:', e);
  }
}, 2500);

export const refreshFolderStats = debounce(async () => {
  try {
    const data = await api.getFolderStats();
    for (const folder in data) {
      if (data[folder].lastFile) {
        data[folder].lastFile.at = new Date(data[folder].lastFile.at);
      }
      data[folder].lastScan = new Date(data[folder].lastScan);
      data[folder].lastScanDays = (new Date() - data[folder].lastScan) / 1000 / 86400;
    }
    folderStats.set(data);
  } catch (e) {
    console.error('refreshFolderStats error:', e);
  }
}, 2500);

export const refreshGlobalChanges = debounce(async () => {
  try {
    const data = await api.getDiskEvents(25);
    if (data) {
      globalChangeEvents.set(data.reverse());
    }
  } catch (e) {
    console.error('refreshGlobalChanges error:', e);
  }
}, 2500);

export const refreshThemes = debounce(async () => {
  try {
    const data = await api.getThemes();
    themes.set(data.themes || []);
  } catch (e) {
    console.error('refreshThemes error:', e);
  }
}, 2500);

export const refreshCluster = async () => {
  try {
    const [devs, folds] = await Promise.all([
      api.getPendingDevices(),
      api.getPendingFolders(),
    ]);
    pendingDevices.set(devs);
    pendingFolders.set(folds);
  } catch (e) {
    console.error('refreshCluster error:', e);
  }
};

export const refresh = () => {
  refreshSystem();
  refreshDiscoveryCache();
  refreshConnectionStats();
  refreshErrors();
};

// ===== Save Config =====

export async function saveConfig() {
  const cfg = get(config);
  try {
    await api.putConfig(cfg);
    await refreshConfig();
  } catch (e) {
    console.error('saveConfig error:', e);
    throw e;
  }
}

// ===== Event handlers setup =====

let restartExpectedFrom = 0;
let restartExpectedUntil = 0;

function clearRestartExpectation() {
  restartExpectedFrom = 0;
  restartExpectedUntil = 0;
}

function setRestartExpectation(delayS) {
  const delay = delayS > 0 ? delayS : 60;
  const delayMs = delay * 1000;
  const earlyMs = 5 * 1000;
  const graceMs = 60 * 1000;
  const now = Date.now();
  restartExpectedFrom = now + Math.max(0, delayMs - earlyMs);
  restartExpectedUntil = now + delayMs + graceMs;
}

function restartExpectedNow() {
  if (!restartExpectedUntil) return false;
  const now = Date.now();
  if (now > restartExpectedUntil) {
    clearRestartExpectation();
    return false;
  }
  return now >= restartExpectedFrom;
}

let navigatingAway = false;

export function setupEventHandlers() {
  window.addEventListener('beforeunload', () => { navigatingAway = true; });

  events.on(events.ONLINE, () => {
    const isOnline = get(online);
    const isRestarting = get(restarting);
    if (isOnline && !isRestarting) return;

    console.log('UIOnline');

    refreshDeviceStats();
    refreshFolderStats();
    refreshGlobalChanges();
    refreshThemes();

    Promise.all([
      refreshSystem(),
      refreshDiscoveryCache(),
      refreshConfig(),
      refreshCluster(),
      refreshConnectionStats(),
    ]).then(async () => {
      try {
        const ver = await api.getSystemVersion();
        const $version = get(version);
        if ($version.version && $version.version !== ver.version) {
          document.location.reload(true);
        }
        version.set(ver);
      } catch (e) { console.error(e); }

      try {
        const up = await api.getSystemUpgrade();
        upgradeInfo.set(up);
      } catch (e) {
        upgradeInfo.set(null);
      }

      online.set(true);
      restarting.set(false);
      clearRestartExpectation();
    }).catch(e => {
      console.error('ONLINE handler error:', e);
    });
  });

  events.on(events.OFFLINE, () => {
    if (navigatingAway || !get(online)) return;

    console.log('UIOffline');
    online.set(false);
    if (!get(restarting)) {
      if (restartExpectedNow()) {
        restarting.set(true);
      }
    }
  });

  events.on(events.STATE_CHANGED, (event) => {
    const data = event.data;
    model.update(m => {
      if (m[data.folder]) {
        m[data.folder].state = data.to;
        m[data.folder].error = data.error;
      }
      return { ...m };
    });
    if (data.to === 'scanning') {
      scanProgress.update(sp => {
        delete sp[data.folder];
        return { ...sp };
      });
    }
    if (data.from === 'scanning' && data.to === 'idle') {
      refreshFolderStats();
    }
  });

  events.on(events.LOCAL_INDEX_UPDATED, () => {
    refreshFolderStats();
    refreshGlobalChanges();
  });

  events.on(events.DEVICE_DISCONNECTED, (event) => {
    connections.update(c => {
      if (c[event.data.id]) {
        c[event.data.id].connected = false;
      }
      return { ...c };
    });
    refreshDeviceStats();
  });

  events.on(events.DEVICE_CONNECTED, (event) => {
    connections.update(c => {
      if (!c[event.data.id]) {
        c[event.data.id] = {
          inbps: 0, outbps: 0, inBytesTotal: 0, outBytesTotal: 0,
          type: event.data.type, address: event.data.addr,
        };
      }
      return { ...c };
    });
    completion.update(comp => {
      if (!comp[event.data.id]) {
        comp[event.data.id] = { _total: 100, _needBytes: 0, _needItems: 0 };
      }
      return { ...comp };
    });
  });

  events.on(events.PENDING_DEVICES_CHANGED, (event) => {
    const arg = event.data;
    if (!(arg.added || arg.removed)) {
      refreshCluster();
      return;
    }
    pendingDevices.update(pd => {
      if (arg.added) {
        arg.added.forEach(rejected => {
          pd[rejected.deviceID] = {
            time: event.time,
            name: rejected.name,
            address: rejected.address,
          };
        });
      }
      if (arg.removed) {
        arg.removed.forEach(dev => {
          delete pd[dev.deviceID];
        });
      }
      return { ...pd };
    });
  });

  events.on(events.PENDING_FOLDERS_CHANGED, (event) => {
    const arg = event.data;
    if (!(arg.added || arg.removed)) {
      refreshCluster();
      return;
    }
    pendingFolders.update(pf => {
      if (arg.added) {
        arg.added.forEach(rejected => {
          if (!pf[rejected.folderID]) {
            pf[rejected.folderID] = { offeredBy: {} };
          }
          pf[rejected.folderID].offeredBy[rejected.deviceID] = {
            time: event.time,
            label: rejected.folderLabel,
            receiveEncrypted: rejected.receiveEncrypted,
          };
        });
      }
      if (arg.removed) {
        arg.removed.forEach(folderDev => {
          if (folderDev.deviceID === undefined) {
            delete pf[folderDev.folderID];
          } else if (pf[folderDev.folderID]) {
            delete pf[folderDev.folderID].offeredBy[folderDev.deviceID];
          }
        });
      }
      return { ...pf };
    });
  });

  events.on(events.CONFIG_SAVED, async (event) => {
    updateLocalConfig(event.data);
    try {
      const data = await api.getConfigInsync();
      configInSync.set(data.configInSync);
    } catch (e) { console.error(e); }
  });

  events.on(events.DOWNLOAD_PROGRESS, (event) => {
    const stats = event.data;
    const newProgress = {};
    for (const folder in stats) {
      newProgress[folder] = {};
      for (const file in stats[folder]) {
        const s = stats[folder][file];
        const reused = 100 * s.reused / s.total;
        const copiedFromOrigin = 100 * s.copiedFromOrigin / s.total;
        const copiedFromElsewhere = 100 * s.copiedFromElsewhere / s.total;
        const pulled = 100 * s.pulled / s.total;
        let pulling = 100 * s.pulling / s.total;
        if (pulling < 1 && pulled + copiedFromElsewhere + copiedFromOrigin + reused <= 99) {
          pulling = 1;
        }
        newProgress[folder][file] = {
          reused, copiedFromOrigin, copiedFromElsewhere, pulled, pulling,
          bytesTotal: s.bytesTotal, bytesDone: s.bytesDone,
        };
      }
    }
    progress.set(newProgress);
  });

  events.on(events.FOLDER_SUMMARY, (event) => {
    const data = event.data;
    model.update(m => ({ ...m, [data.folder]: data.summary }));
  });

  events.on(events.FOLDER_COMPLETION, (event) => {
    const data = event.data;
    completion.update(comp => {
      if (!comp[data.device]) comp[data.device] = {};
      comp[data.device][data.folder] = data;
      recalcCompletion(comp, data.device);
      return { ...comp };
    });
  });

  events.on(events.FOLDER_ERRORS, (event) => {
    model.update(m => {
      if (m[event.data.folder]) {
        m[event.data.folder].errors = event.data.errors.length;
      }
      return { ...m };
    });
  });

  events.on(events.FOLDER_SCAN_PROGRESS, (event) => {
    const data = event.data;
    scanProgress.update(sp => ({
      ...sp,
      [data.folder]: { current: data.current, total: data.total, rate: data.rate },
    }));
  });

  events.on(events.UPGRADE_RESTART_SCHEDULED, (event) => {
    let delayS = 0;
    if (event && event.data && event.data.delayS !== undefined) {
      delayS = parseInt(event.data.delayS, 10);
      if (isNaN(delayS) || delayS < 0) delayS = 0;
    }
    setRestartExpectation(delayS);
  });
}

// ===== Initialize =====

export function init() {
  // Check for authentication
  const metadata = window.metadata;
  if (metadata && metadata.authenticated) {
    authenticated.set(true);
    initCSRF(metadata);
  }

  // Load metric rates preference
  try {
    metricRates.set(window.localStorage['metricRates'] === 'true');
  } catch (e) { }

  setupEventHandlers();

  // Initialize i18n (auto-detect locale)
  autoConfigLocale();

  if (get(authenticated)) {
    setInterval(refresh, 10000);
    events.start();
  }
}
