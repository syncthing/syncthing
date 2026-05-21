// Utility functions and formatters
// Mirrors the AngularJS filters and helpers

// Re-export comparison functions from stores for convenience
export { deviceCompare, folderCompare } from './stores.js';
import { version as versionStore } from './stores.js';
import { get } from 'svelte/store';

const shortIDStringLength = 7;

// ===== Unit formatting (from app.js unitPrefixed) =====

export function unitPrefixed(input, binary) {
  if (input === undefined || isNaN(input)) return '0 ';
  const factor = binary ? 1024 : 1000;
  const i = binary ? 'i' : '';

  if (input > factor * factor * factor * factor * 1000) {
    input /= factor * factor * factor * factor;
    return input.toLocaleString(undefined, { maximumFractionDigits: 0 }) + ' T' + i;
  }
  if (input > factor * factor * factor * factor) {
    input /= factor * factor * factor * factor;
    return input.toLocaleString(undefined, { maximumSignificantDigits: 3 }) + ' T' + i;
  }
  if (input > factor * factor * factor) {
    input /= factor * factor * factor;
    if (binary && input >= 1000) {
      return input.toLocaleString(undefined, { maximumFractionDigits: 0 }) + ' G' + i;
    }
    return input.toLocaleString(undefined, { maximumSignificantDigits: 3 }) + ' G' + i;
  }
  if (input > factor * factor) {
    input /= factor * factor;
    if (binary && input >= 1000) {
      return input.toLocaleString(undefined, { maximumFractionDigits: 0 }) + ' M' + i;
    }
    return input.toLocaleString(undefined, { maximumSignificantDigits: 3 }) + ' M' + i;
  }
  if (input > factor) {
    input /= factor;
    const prefix = binary ? ' K' : ' k';
    if (binary && input >= 1000) {
      return input.toLocaleString(undefined, { maximumFractionDigits: 0 }) + prefix + i;
    }
    return input.toLocaleString(undefined, { maximumSignificantDigits: 3 }) + prefix + i;
  }
  return Math.round(input).toLocaleString() + ' ';
}

export function binaryFilter(input) {
  return unitPrefixed(input, true);
}

export function metricFilter(input) {
  return unitPrefixed(input, false);
}

// ===== Duration formatting =====

import humanizeDuration from './humanize-duration.js';
import { currentLocale } from './i18n.js';

const SECONDS_IN = { d: 86400, h: 3600, m: 60, s: 1 };

export function durationFilter(input, precision = 's') {
  input = parseInt(input, 10);
  if (isNaN(input)) return '';

  const lang = get(currentLocale) || 'en';
  const langNorm = lang.replace('-', '_');
  const fallbacks = [];
  const langBase = langNorm.substr(0, 2);
  if (langBase === 'zh') fallbacks.push('zh_TW');
  if (langBase !== langNorm) fallbacks.push(langBase);
  fallbacks.push('en');

  const units = ['d', 'h', 'm', 's'];
  const precIdx = units.indexOf(precision);
  const activeUnits = precIdx >= 0 ? units.slice(0, precIdx + 1) : units;

  try {
    return humanizeDuration(input * 1000, {
      language: langNorm,
      maxDecimalPoints: 0,
      units: activeUnits,
      fallbacks: fallbacks
    });
  } catch (e) {
    // Fallback to English abbreviations
    let result = '';
    for (const k in SECONDS_IN) {
      const t = Math.floor(input / SECONDS_IN[k]);
      if (t > 0) {
        result += (result ? ' ' : '') + t + k;
      }
      if (precision === k) {
        return result || '<1' + k;
      }
      input %= SECONDS_IN[k];
    }
    return result || '0s';
  }
}

// ===== Percent formatting =====

export function percentFilter(input) {
  if (input === undefined || isNaN(input)) return '0%';
  return Math.floor(input) + '%';
}

// ===== Locale number formatting =====

export function localeNumber(input) {
  if (input === undefined || input === null) return '0';
  return Number(input).toLocaleString();
}

export function alwaysNumber(input) {
  if (input === undefined || input === null || isNaN(input)) return 0;
  return input;
}

// ===== Basename filter =====

export function basename(input) {
  if (!input) return '';
  const parts = input.replace(/\\/g, '/').split('/');
  return parts[parts.length - 1];
}

// ===== Date formatting =====

export function formatDate(input, format = 'yyyy-MM-dd HH:mm:ss') {
  if (!input) return '';
  const d = new Date(input);
  if (isNaN(d.getTime())) return '';

  const pad = (n) => String(n).padStart(2, '0');
  return format
    .replace('yyyy', d.getFullYear())
    .replace('MM', pad(d.getMonth() + 1))
    .replace('dd', pad(d.getDate()))
    .replace('HH', pad(d.getHours()))
    .replace('mm', pad(d.getMinutes()))
    .replace('ss', pad(d.getSeconds()));
}

// ===== Device helpers =====

export function deviceShortID(deviceID) {
  if (!deviceID) return '';
  return deviceID.substr(0, shortIDStringLength);
}

export function deviceName(deviceCfg) {
  if (!deviceCfg) return '';
  return deviceCfg.name || deviceShortID(deviceCfg.deviceID);
}

export function folderLabel(folders, folderID) {
  if (!folders || !folders[folderID]) return folderID;
  const label = folders[folderID].label;
  return label && label.length > 0 ? label : folderID;
}

// ===== Status helpers =====

export function folderStatus(folderCfg, model, folders) {
  if (!folderCfg) return 'unknown';
  if (folderCfg.paused) return 'paused';

  const folderInfo = model[folderCfg.id];
  if (!folderInfo || !folderInfo.state) return 'unknown';

  const state = '' + folderInfo.state;
  if (state === 'error') return 'stopped';
  if (state !== 'idle') return state;

  if (folderInfo.needTotalItems > 0) return 'outofsync';
  if (hasFailedFiles(folderCfg.id, model)) return 'faileditems';
  if (hasReceiveOnlyChanged(folderCfg, model)) {
    if (folderCfg.type === 'receiveonly') return 'localadditions';
    return 'localunencrypted';
  }
  if (folderCfg.devices && folderCfg.devices.length <= 1) return 'unshared';

  return state;
}

export function hasFailedFiles(folder, model) {
  return model[folder] && model[folder].errors !== 0;
}

export function hasReceiveOnlyChanged(folderCfg, model) {
  if (!folderCfg || ['receiveonly', 'receiveencrypted'].indexOf(folderCfg.type) === -1) return false;
  const counts = model[folderCfg.id];
  return counts && counts.receiveOnlyTotalItems > 0;
}

export function hasReceiveEncryptedItems(folderCfg, model) {
  if (!folderCfg || folderCfg.type !== 'receiveencrypted') return false;
  const counts = model[folderCfg.id];
  return counts && counts.receiveOnlyTotalItems > 0;
}

export function folderClass(folderCfg, model, folders) {
  const status = folderStatus(folderCfg, model, folders);
  if (status === 'idle' || status === 'localadditions') return 'success';
  if (status === 'paused') return 'default';
  if (['syncing', 'sync-preparing', 'scanning', 'cleaning', 'starting'].includes(status)) return 'primary';
  if (status === 'unknown') return 'info';
  if (['stopped', 'outofsync', 'error', 'faileditems', 'localunencrypted'].includes(status)) return 'danger';
  if (['unshared', 'scan-waiting', 'sync-waiting', 'clean-waiting'].includes(status)) return 'warning';
  return 'info';
}

export function folderStatusIcon(folderCfg, model, folders) {
  const status = folderStatus(folderCfg, model, folders);
  switch (status) {
    case 'clean-waiting':
    case 'scan-waiting':
    case 'sync-preparing':
    case 'sync-waiting':
    case 'starting':
      return 'fa-hourglass-half';
    case 'cleaning':
      return 'fa-recycle';
    case 'faileditems':
    case 'localunencrypted':
    case 'outofsync':
      return 'fa-exclamation-circle';
    case 'idle':
    case 'localadditions':
      return 'fa-check';
    case 'paused':
      return 'fa-pause';
    case 'scanning':
      return 'fa-search';
    case 'stopped':
      return 'fa-stop';
    case 'syncing':
      return 'fa-sync';
    case 'unknown':
      return 'fa-question-circle';
    case 'unshared':
      return 'fa-unlink';
    default:
      return 'fa-question-circle';
  }
}

export function folderStatusText(folderCfg, model, folders) {
  const status = folderStatus(folderCfg, model, folders);
  const map = {
    'clean-waiting': 'Waiting to Clean',
    'cleaning': 'Cleaning Versions',
    'faileditems': 'Failed Items',
    'idle': 'Up to Date',
    'starting': 'Starting',
    'localadditions': 'Local Additions',
    'localunencrypted': 'Unexpected Items',
    'outofsync': 'Out of Sync',
    'paused': 'Paused',
    'scan-waiting': 'Waiting to Scan',
    'scanning': 'Scanning',
    'stopped': 'Stopped',
    'sync-preparing': 'Preparing to Sync',
    'sync-waiting': 'Waiting to Sync',
    'syncing': 'Syncing',
    'unknown': 'Unknown',
    'unshared': 'Unshared',
  };
  return map[status] || 'Unknown';
}

export function deviceStatus(deviceCfg, connections, completion, devices, myID, deviceStats, folders) {
  if (!deviceCfg) return 'unknown';
  const unused = deviceFolders(deviceCfg, folders || devices).length === 0;
  let status = unused ? 'unused-' : '';

  if (!connections[deviceCfg.deviceID]) return 'unknown';
  if (deviceCfg.paused) return status + 'paused';

  if (connections[deviceCfg.deviceID].connected) {
    if (completion[deviceCfg.deviceID] && completion[deviceCfg.deviceID]._total === 100) {
      return status + 'insync';
    }
    return 'syncing';
  }

  // Disconnected
  if (!unused && deviceStats[deviceCfg.deviceID] &&
      (!deviceStats[deviceCfg.deviceID].lastSeenDays || deviceStats[deviceCfg.deviceID].lastSeenDays >= 7)) {
    return status + 'disconnected-inactive';
  }
  return status + 'disconnected';
}

export function deviceClass(deviceCfg, connections, completion) {
  if (!deviceCfg || !connections[deviceCfg.deviceID]) return 'info';
  if (deviceCfg.paused) return 'default';
  if (connections[deviceCfg.deviceID].connected) {
    if (completion[deviceCfg.deviceID] && completion[deviceCfg.deviceID]._total === 100) {
      return 'success';
    }
    return 'primary';
  }
  return 'info';
}

export function deviceStatusIcon(status) {
  switch (status) {
    case 'disconnected':
    case 'disconnected-inactive':
      return 'fa-power-off';
    case 'insync':
      return 'fa-check';
    case 'paused':
      return 'fa-pause';
    case 'syncing':
      return 'fa-sync';
    case 'unused-disconnected':
    case 'unused-insync':
    case 'unused-paused':
      return 'fa-unlink';
    default:
      return 'fa-question-circle';
  }
}

export function deviceStatusText(status) {
  const map = {
    'disconnected': 'Disconnected',
    'disconnected-inactive': 'Disconnected (Inactive)',
    'insync': 'Up to Date',
    'paused': 'Paused',
    'syncing': 'Syncing',
    'unused-disconnected': 'Disconnected (Unused)',
    'unused-insync': 'Connected (Unused)',
    'unused-paused': 'Paused (Unused)',
    'unknown': 'Unknown',
  };
  return map[status] || 'Unknown';
}

export function deviceFolders(deviceCfg, allFolders) {
  if (!deviceCfg || !allFolders) return [];
  const result = [];
  for (const fid in allFolders) {
    const folder = allFolders[fid];
    if (folder.devices) {
      for (const d of folder.devices) {
        if (d.deviceID === deviceCfg.deviceID) {
          result.push(fid);
          break;
        }
      }
    }
  }
  return result;
}

export function syncPercentage(folder, model) {
  if (!model[folder]) return 100;
  if (model[folder].needTotalItems === 0) return 100;
  if ((model[folder].needBytes === 0 && model[folder].needDeletes > 0) || model[folder].globalBytes === 0) {
    return 95;
  }
  return progressIntegerPercentage(model[folder].inSyncBytes, model[folder].globalBytes);
}

export function scanPercentage(folder, scanProgress) {
  if (!scanProgress[folder]) return undefined;
  return progressIntegerPercentage(scanProgress[folder].current, scanProgress[folder].total);
}

function progressIntegerPercentage(current, total) {
  if (current === total) return 99;
  return Math.floor(100 * current / total);
}

export function scanRate(folder, scanProgress) {
  if (!scanProgress[folder]) return 0;
  return scanProgress[folder].rate;
}

export function scanRemaining(folder, scanProg) {
  if (!scanProg[folder]) return '';
  const remainingBytes = scanProg[folder].total - scanProg[folder].current;
  let seconds = remainingBytes / scanProg[folder].rate;
  seconds = Math.ceil(seconds / 10) * 10;

  let days = 0;
  const res = [];
  if (seconds >= 86400) {
    days = Math.floor(seconds / 86400);
    if (days > 31) return '> 1 month';
    res.push(days + 'd');
    seconds = seconds % 86400;
  }
  let hours = 0;
  if (seconds > 3600) {
    hours = Math.floor(seconds / 3600);
    res.push(hours + 'h');
    seconds = seconds % 3600;
  }
  if (days === 0) {
    res.push(Math.floor(seconds / 60) + 'm');
  }
  if (days === 0 && hours === 0) {
    res.push(String(Math.floor(seconds % 60)).padStart(2, '0') + 's');
  }
  return res.join(' ');
}

export function rdConnType(deviceID, connections) {
  const conn = connections[deviceID];
  if (!conn) return 'disconnected';
  let type = 'disconnected';
  if (conn.type && conn.type.indexOf('relay') === 0) type = 'relay';
  else if (conn.type && conn.type.indexOf('quic') === 0) type = 'quic';
  else if (conn.type && conn.type.indexOf('tcp') === 0) type = 'tcp';
  else return type;

  return type + (conn.isLocal ? 'lan' : 'wan');
}

export function rdConnTypeString(type) {
  const map = {
    'relaywan': 'Relay WAN',
    'relaylan': 'Relay LAN',
    'quicwan': 'QUIC WAN',
    'quiclan': 'QUIC LAN',
    'tcpwan': 'TCP WAN',
    'tcplan': 'TCP LAN',
  };
  return map[type] || 'Disconnected';
}

export function rdConnTypeIcon(type) {
  switch (type) {
    case 'tcplan': case 'quiclan': return 'reception-4';
    case 'tcpwan': case 'quicwan': return 'reception-3';
    case 'relaylan': return 'reception-2';
    case 'relaywan': return 'reception-1';
    default: return 'reception-0';
  }
}

export function syncthingStatus(folders, model, devices, connections, myID, openNoAuth, configInSync, errorList, online, pendingDevices, pendingFolders) {
  let syncCount = 0;
  let notifyCount = 0;
  let pauseCount = 0;
  let deviceCount = 0;

  for (const f of Object.values(folders)) {
    const status = folderStatus(f, model, folders);
    if (status === 'sync-preparing' || status === 'syncing') syncCount++;
    if (['stopped', 'unknown', 'outofsync', 'error'].includes(status)) notifyCount++;
  }

  for (const id in devices) {
    if (id === myID) continue;
    const status = deviceStatus({ deviceID: id }, connections, {}, devices, myID, {});
    if (status === 'unknown') notifyCount++;
    if (status === 'paused') pauseCount++;
    if (status === 'unused') deviceCount--;
    deviceCount++;
  }

  if (openNoAuth || !configInSync || (errorList && errorList.length > 0) || !online ||
      Object.keys(pendingDevices || {}).length > 0 || Object.keys(pendingFolders || {}).length > 0) {
    notifyCount++;
  }

  if (syncCount > 0) return 'sync';
  if (notifyCount > 0) return 'notify';
  if (pauseCount === deviceCount && deviceCount > 0) return 'pause';
  return 'default';
}

// ===== Version string helper =====

export function versionString(ver) {
  if (!ver || !ver.version) return '';
  const osMap = {
    'darwin': 'macOS', 'dragonfly': 'DragonFly BSD', 'freebsd': 'FreeBSD',
    'openbsd': 'OpenBSD', 'netbsd': 'NetBSD', 'linux': 'Linux',
    'windows': 'Windows', 'solaris': 'Solaris',
  };
  const archMap = {
    '386': '32-bit Intel/AMD', 'amd64': '64-bit Intel/AMD',
    'arm': '32-bit ARM', 'arm64': '64-bit ARM',
    'ppc64': '64-bit PowerPC', 'ppc64le': '64-bit PowerPC (LE)',
    'mips': '32-bit MIPS', 'mipsle': '32-bit MIPS (LE)',
    'mips64': '64-bit MIPS', 'mips64le': '64-bit MIPS (LE)',
    'riscv64': '64-bit RISC-V', 's390x': '64-bit z/Architecture',
  };

  const os = osMap[ver.os] || ver.os;
  let arch = archMap[ver.arch] || ver.arch;
  if (ver.container) arch += ' Container';

  let verStr = ver.version;
  if (ver.extra) verStr += ' (' + ver.extra + ')';
  return verStr + ', ' + os + ' (' + arch + ')';
}

export function versionBase(ver) {
  if (!ver || !ver.version) return '';
  const pos = ver.version.indexOf('-');
  return pos > 0 ? ver.version.slice(0, pos) : ver.version;
}

export function docsURL(ver, path) {
  let url = 'https://docs.syncthing.net';
  if (!path) path = '';
  // Auto-resolve version from store if not provided
  if (!ver) ver = get(versionStore);
  const vb = versionBase(ver);
  if (!path && !vb) return url;
  const hash = path.indexOf('#');
  if (hash !== -1) {
    url += '/' + path.slice(0, hash);
    if (vb) url += '?version=' + vb;
    url += path.slice(hash);
  } else {
    if (path) url += '/' + path;
    if (vb) url += '?version=' + vb;
  }
  return url;
}

// ===== Remote GUI helpers =====

export function hasRemoteGUIAddress(device, connections) {
  if (!device || !device.remoteGUIPort) return false;
  const conn = connections?.[device.deviceID];
  if (!conn?.connected) return false;
  const addr = conn.address;
  if (!addr) return false;
  return true;
}

export function remoteGUIAddress(device, connections) {
  const conn = connections?.[device.deviceID];
  if (!conn?.connected || !conn.address) return '';
  const addr = conn.address;
  // Extract host from address (e.g., "192.168.0.1:22000" -> "192.168.0.1")
  const lastColon = addr.lastIndexOf(':');
  const host = lastColon > 0 ? addr.substring(0, lastColon) : addr;
  return 'https://' + host.replace('%', '%25') + ':' + device.remoteGUIPort;
}

// ===== Identicon generation =====

export function generateIdenticon(id) {
  if (!id) return '';
  // Exact port of Syncthing's AngularJS identiconDirective.js
  const value = id.toString().replace(/[\W_]/g, '');
  const size = 5;
  const rectSize = 100 / size;
  const middleCol = Math.ceil(size / 2) - 1;

  let rects = '';
  for (let row = 0; row < size; row++) {
    for (let col = middleCol; col > -1; col--) {
      if (!(parseInt(value.charCodeAt(row + col * size), 10) % 2)) {
        rects += `<rect x="${col * rectSize}%" y="${row * rectSize}%" width="${rectSize}%" height="${rectSize}%"/>`;
        // Mirror (unless it's the middle column on odd-sized grids)
        if (!(size % 2 && col === middleCol)) {
          const mirrorCol = size - col - 1;
          rects += `<rect x="${mirrorCol * rectSize}%" y="${row * rectSize}%" width="${rectSize}%" height="${rectSize}%"/>`;
        }
      }
    }
  }

  return `<svg class="identicon" viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">${rects}</svg>`;
}

// ===== Clipboard utility =====

export async function copyToClipboard(text) {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    await navigator.clipboard.writeText(text);
    return true;
  }
  // Fallback
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  try {
    document.execCommand('copy');
    return true;
  } catch (e) {
    return false;
  } finally {
    document.body.removeChild(textarea);
  }
}

// ===== FS Watcher error map =====

export function fsWatcherErrorMap(folders, model) {
  const errs = {};
  for (const id in folders) {
    const cfg = folders[id];
    if (cfg.fsWatcherEnabled && model[id] && model[id].watchError && !cfg.paused && folderStatus(cfg, model, folders) !== 'stopped') {
      errs[id] = model[id].watchError;
    }
  }
  return errs;
}

// ===== Abbreviate error =====

export function abbreviatedError(addr, system) {
  if (!system || !system.lastDialStatus || !system.lastDialStatus[addr]) return null;
  const status = system.lastDialStatus[addr];
  if (!status.error) return null;
  const time = formatDate(status.when, 'HH:mm:ss');
  const err = status.error.replace(/.+: /, '');
  return err + ' (' + time + ')';
}

// Pagination: exact port of angular-dirPagination.js generatePagesArray
// maxSize defaults to 9 (same as original)
export function generatePagesArray(currentPage, totalItems, itemsPerPage, maxSize = 9) {
  const totalPages = Math.ceil(totalItems / itemsPerPage);
  if (totalPages <= 0) return [];
  const paginationRange = Math.max(maxSize, 5);
  const halfWay = Math.ceil(paginationRange / 2);
  let position;
  if (currentPage <= halfWay) {
    position = 'start';
  } else if (totalPages - halfWay < currentPage) {
    position = 'end';
  } else {
    position = 'middle';
  }
  const ellipsesNeeded = paginationRange < totalPages;
  const pages = [];
  let i = 1;
  while (i <= totalPages && i <= paginationRange) {
    const pageNumber = calculatePageNumber(i, currentPage, paginationRange, totalPages);
    const openingEllipsesNeeded = (i === 2 && (position === 'middle' || position === 'end'));
    const closingEllipsesNeeded = (i === paginationRange - 1 && (position === 'middle' || position === 'start'));
    if (ellipsesNeeded && (openingEllipsesNeeded || closingEllipsesNeeded)) {
      pages.push('...');
    } else {
      pages.push(pageNumber);
    }
    i++;
  }
  return pages;
}

function calculatePageNumber(i, currentPage, paginationRange, totalPages) {
  const halfWay = Math.ceil(paginationRange / 2);
  if (i === paginationRange) {
    return totalPages;
  } else if (i === 1) {
    return i;
  } else if (paginationRange < totalPages) {
    if (totalPages - halfWay < currentPage) {
      return totalPages - paginationRange + i;
    } else if (halfWay < currentPage) {
      return currentPage - halfWay + i;
    } else {
      return i;
    }
  } else {
    return i;
  }
}
