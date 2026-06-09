// REST API client for Syncthing
// Mirrors the endpoints used by the AngularJS UI

const urlbase = 'rest';
const authUrlbase = urlbase + '/noauth/auth';

let csrfToken = '';
let csrfCookie = '';

// Initialize CSRF from metadata
export function initCSRF(metadata) {
  if (metadata && metadata.deviceIDShort) {
    csrfToken = 'X-CSRF-Token-' + metadata.deviceIDShort;
    csrfCookie = 'CSRF-Token-' + metadata.deviceIDShort;
  }
}

function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? match[2] : '';
}

async function request(method, path, body, opts = {}) {
  const headers = {};
  if (csrfToken && csrfCookie) {
    headers[csrfToken] = getCookie(csrfCookie);
  }
  if (body && typeof body === 'object') {
    headers['Content-Type'] = 'application/json';
    body = JSON.stringify(body);
  }

  const url = path.startsWith('http') ? path : (path.startsWith('/') ? path : '/' + path);

  const response = await fetch(url, {
    method,
    headers,
    body: method !== 'GET' ? body : undefined,
    credentials: 'same-origin',
    ...opts,
  });

  if (!response.ok) {
    if (response.status === 403) {
      // Session expired — reload to show login form
      location.reload();
      return;
    }
    const err = new Error(`HTTP ${response.status}: ${response.statusText}`);
    err.status = response.status;
    err.response = response;
    throw err;
  }

  const contentType = response.headers.get('content-type');
  if (contentType && contentType.includes('application/json')) {
    return response.json();
  }
  return response.text();
}

export const api = {
  get: (path) => request('GET', urlbase + '/' + path),
  post: (path, body) => request('POST', urlbase + '/' + path, body),
  put: (path, body) => request('PUT', urlbase + '/' + path, body),
  delete: (path) => request('DELETE', urlbase + '/' + path),

  // Authentication
  login: (username, password, stayLoggedIn) =>
    request('POST', '/' + authUrlbase + '/password', { username, password, stayLoggedIn }),
  logout: () => request('POST', '/' + authUrlbase + '/logout', {}),

  // System
  getConfig: () => request('GET', '/' + urlbase + '/config'),
  putConfig: (config) => request('PUT', '/' + urlbase + '/config', config),
  getConfigInsync: () => request('GET', '/' + urlbase + '/config/insync'),
  getSystemStatus: () => request('GET', '/' + urlbase + '/system/status'),
  getSystemVersion: () => request('GET', '/' + urlbase + '/system/version'),
  getSystemConnections: () => request('GET', '/' + urlbase + '/system/connections'),
  getSystemDiscovery: () => request('GET', '/' + urlbase + '/system/discovery'),
  getSystemError: () => request('GET', '/' + urlbase + '/system/error'),
  clearErrors: () => request('POST', '/' + urlbase + '/system/error/clear'),
  getSystemUpgrade: () => request('GET', '/' + urlbase + '/system/upgrade'),
  getSystemBrowse: (current) => request('GET', '/' + urlbase + '/system/browse?current=' + encodeURIComponent(current)),
  getSystemPaths: () => request('GET', '/' + urlbase + '/system/paths'),
  getSystemLog: (since) => request('GET', '/' + urlbase + '/system/log' + (since ? '?since=' + encodeURIComponent(since) : '')),
  getLogLevels: () => request('GET', '/' + urlbase + '/system/loglevels'),
  setLogLevels: (levels) => request('POST', '/' + urlbase + '/system/loglevels', levels),
  postRestart: () => request('POST', '/' + urlbase + '/system/restart'),
  postShutdown: () => request('POST', '/' + urlbase + '/system/shutdown'),
  postUpgrade: () => request('POST', '/' + urlbase + '/system/upgrade'),

  // Database
  getFolderStatus: (folder) => request('GET', '/' + urlbase + '/db/status?folder=' + encodeURIComponent(folder)),
  getCompletion: (device, folder) => request('GET', '/' + urlbase + '/db/completion?device=' + device + '&folder=' + encodeURIComponent(folder)),
  getNeed: (folder, page, perpage) => request('GET', '/' + urlbase + '/db/need?folder=' + encodeURIComponent(folder) + '&page=' + page + '&perpage=' + perpage),
  getRemoteNeed: (device, folder, page, perpage) => request('GET', '/' + urlbase + '/db/remoteneed?device=' + device + '&folder=' + encodeURIComponent(folder) + '&page=' + page + '&perpage=' + perpage),
  getLocalChanged: (folder, page, perpage) => request('GET', '/' + urlbase + '/db/localchanged?folder=' + encodeURIComponent(folder) + '&page=' + page + '&perpage=' + perpage),
  getIgnores: (folder) => request('GET', '/' + urlbase + '/db/ignores?folder=' + encodeURIComponent(folder)),
  postIgnores: (folder, ignores) => request('POST', '/' + urlbase + '/db/ignores?folder=' + encodeURIComponent(folder), { ignore: ignores }),
  postScan: (folder) => request('POST', '/' + urlbase + '/db/scan' + (folder ? '?folder=' + encodeURIComponent(folder) : '')),
  postBumpFile: (folder, file, page, perpage) => request('POST', '/' + urlbase + '/db/prio?folder=' + encodeURIComponent(folder) + '&file=' + encodeURIComponent(file) + '&page=' + page + '&perpage=' + perpage),
  postOverride: (folder) => request('POST', '/' + urlbase + '/db/override?folder=' + encodeURIComponent(folder)),
  postRevert: (folder) => request('POST', '/' + urlbase + '/db/revert?folder=' + encodeURIComponent(folder)),

  // Config
  getDefaultFolder: () => request('GET', '/' + urlbase + '/config/defaults/folder'),
  getDefaultDevice: () => request('GET', '/' + urlbase + '/config/defaults/device'),
  getDefaultIgnores: () => request('GET', '/' + urlbase + '/config/defaults/ignores'),

  // Cluster
  getPendingDevices: () => request('GET', '/' + urlbase + '/cluster/pending/devices'),
  getPendingFolders: () => request('GET', '/' + urlbase + '/cluster/pending/folders'),
  dismissPendingDevice: (device) => request('DELETE', '/' + urlbase + '/cluster/pending/devices?device=' + encodeURIComponent(device)),
  dismissPendingFolder: (folder, device) => request('DELETE', '/' + urlbase + '/cluster/pending/folders?folder=' + encodeURIComponent(folder) + '&device=' + encodeURIComponent(device)),

  // Stats
  getDeviceStats: () => request('GET', '/' + urlbase + '/stats/device'),
  getFolderStats: () => request('GET', '/' + urlbase + '/stats/folder'),

  // Events
  getEvents: (since, limit) => {
    let url = '/' + urlbase + '/events';
    const params = [];
    if (since !== undefined) params.push('since=' + since);
    if (limit !== undefined) params.push('limit=' + limit);
    if (params.length) url += '?' + params.join('&');
    return request('GET', url);
  },
  getDiskEvents: (limit) => request('GET', '/' + urlbase + '/events/disk?limit=' + (limit || 25)),

  // Services
  getReport: (version) => request('GET', '/' + urlbase + '/svc/report' + (version ? '?version=' + version : '')),
  getRandomString: (length) => request('GET', '/' + urlbase + '/svc/random/string?length=' + (length || 32)),

  // Folder
  getFolderErrors: (folder, page, perpage) => request('GET', '/' + urlbase + '/folder/errors?folder=' + encodeURIComponent(folder) + '&page=' + page + '&perpage=' + perpage),
  getFolderVersions: (folder) => request('GET', '/' + urlbase + '/folder/versions?folder=' + encodeURIComponent(folder)),
  postFolderVersions: (folder, selections) => request('POST', '/' + urlbase + '/folder/versions?folder=' + encodeURIComponent(folder), selections),

  // Themes
  getThemes: () => request('GET', '/themes.json'),
};
