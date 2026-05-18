<script>
  import Modal from '../Modal.svelte';
  import { config as configStore, devices as devicesStore } from '../../lib/stores.js';
  import * as utils from '../../lib/utils.js';
  import { api } from '../../lib/api.js';
  import { get } from 'svelte/store';
  import { t, translations, currentLocale, getAvailableLocales, useLocale } from '../../lib/i18n.js';
  import { tooltip } from '../../lib/tooltip.js';

  let { config, system, devices, myID, themes, upgradeInfo, onclose, actions } = $props();

  let activeTab = $state('general');
  let dirty = $state({});

  // Working copies
  let tmpOptions = $state({ ...config.options });
  let tmpGUI = $state({ ...config.gui });
  let deviceName = $state('');
  let tmpRemoteIgnoredDevices = $state([...(config.remoteIgnoredDevices || [])]);
  let tmpDevices = $state(JSON.parse(JSON.stringify(config.devices || [])));

  // Initialize working copies once (not reactively)
  {
    if (devices[myID]) {
      deviceName = devices[myID].name || '';
    }
    tmpOptions = { ...config.options };
    tmpOptions._listenAddressesStr = (config.options?.listenAddresses || []).join(', ');
    tmpOptions._globalAnnounceServersStr = (config.options?.globalAnnounceServers || []).join(', ');

    // Determine upgrade setting
    tmpOptions.upgrades = 'none';
    if (tmpOptions.autoUpgradeIntervalH > 0) tmpOptions.upgrades = 'stable';
    if (tmpOptions.upgradeToPreReleases) tmpOptions.upgrades = 'candidate';

    // Usage reporting - keep as number
    if (tmpOptions.urAccepted === undefined || tmpOptions.urAccepted === null) {
      tmpOptions.urAccepted = 0;
    }

    tmpGUI = { ...config.gui };
    tmpRemoteIgnoredDevices = [...(config.remoteIgnoredDevices || [])];
    tmpDevices = JSON.parse(JSON.stringify(config.devices || []));
  }

  function urVersions() {
    const max = system?.urVersionMax || 3;
    const versions = [];
    for (let i = max; i >= 2; i--) {
      versions.push(i);
    }
    return versions;
  }

  function ignoredFoldersCount() {
    let count = 0;
    for (const dev of tmpDevices) {
      if (dev.ignoredFolders) {
        count += dev.ignoredFolders.length;
      }
    }
    return count;
  }

  function unignoreDevice(ignoredDevice) {
    tmpRemoteIgnoredDevices = tmpRemoteIgnoredDevices.filter(d => d.deviceID !== ignoredDevice.deviceID);
  }

  function unignoreFolder(deviceID, folderID) {
    tmpDevices = tmpDevices.map(dev => {
      if (dev.deviceID === deviceID) {
        return { ...dev, ignoredFolders: (dev.ignoredFolders || []).filter(f => f.id !== folderID) };
      }
      return dev;
    });
  }

  function friendlyNameFromID(deviceID) {
    if (devices[deviceID]) return utils.deviceName(devices[deviceID]);
    return utils.deviceShortID(deviceID);
  }

  function themeName(theme) {
    return theme.charAt(0).toUpperCase() + theme.slice(1).replace(/-/g, ' ');
  }

  let availableLocales = getAvailableLocales();

  async function saveSettings() {
    // Parse upgrade settings
    if (tmpOptions.upgrades === 'candidate') {
      tmpOptions.autoUpgradeIntervalH = tmpOptions.autoUpgradeIntervalH || 12;
      tmpOptions.upgradeToPreReleases = true;
      tmpOptions.urAccepted = system?.urVersionMax || 3;
      tmpOptions.urSeen = system?.urVersionMax || 3;
    } else if (tmpOptions.upgrades === 'stable') {
      tmpOptions.autoUpgradeIntervalH = tmpOptions.autoUpgradeIntervalH || 12;
      tmpOptions.upgradeToPreReleases = false;
    } else {
      tmpOptions.autoUpgradeIntervalH = 0;
      tmpOptions.upgradeToPreReleases = false;
    }

    // Parse address strings
    tmpOptions.listenAddresses = tmpOptions._listenAddressesStr.split(/[ ,]+/).map(x => x.trim()).filter(x => x);
    tmpOptions.globalAnnounceServers = tmpOptions._globalAnnounceServersStr.split(/[ ,]+/).map(x => x.trim()).filter(x => x);

    const themeChanged = config.gui.theme !== tmpGUI.theme;

    // Update device name
    devicesStore.update(d => {
      if (d[myID]) d[myID].name = deviceName;
      return { ...d };
    });

    configStore.update(c => {
      c.options = { ...tmpOptions };
      c.gui = { ...tmpGUI };
      c.devices = Object.values(get(devicesStore)).sort(utils.deviceCompare);
      c.remoteIgnoredDevices = tmpRemoteIgnoredDevices;
      // Update ignored folders in devices
      c.devices = c.devices.map(dev => {
        const tmpDev = tmpDevices.find(td => td.deviceID === dev.deviceID);
        if (tmpDev) {
          return { ...dev, ignoredFolders: tmpDev.ignoredFolders || [] };
        }
        return dev;
      });
      return { ...c };
    });

    await actions.saveConfig();

    if (themeChanged) {
      document.location.reload(true);
    } else {
      onclose();
    }
  }

  async function generateAPIKey() {
    const data = await api.getRandomString(32);
    tmpGUI.apiKey = data.random;
  }

  function editFolderDefaults() {
    if (actions?.editFolderDefaults) actions.editFolderDefaults();
  }

  function editDeviceDefaults() {
    if (actions?.editDeviceDefaults) actions.editDeviceDefaults();
  }
</script>

<Modal title={t('Settings')} icon="fas fa-cog" large={true} {onclose}>
  <div class="modal-body">
    <ul class="nav nav-tabs">
      <li class:active={activeTab === 'general'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'general'; }}>
          <span class="fas fa-cog"></span> {$translations, t('General')}
        </a>
      </li>
      <li class:active={activeTab === 'gui'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'gui'; }}>
          <span class="fas fa-desktop"></span> {$translations, t('GUI')}
        </a>
      </li>
      <li class:active={activeTab === 'connections'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'connections'; }}>
          <span class="fas fa-sitemap"></span> {$translations, t('Connections')}
        </a>
      </li>
      <li class:active={activeTab === 'ignored-devices'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'ignored-devices'; }}>
          <span class="fas fa-laptop"></span>&nbsp;{$translations, t('Ignored Devices')}&nbsp;
          <span class="badge">{tmpRemoteIgnoredDevices.length}</span>
        </a>
      </li>
      <li class:active={activeTab === 'ignored-folders'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'ignored-folders'; }}>
          <span class="fas fa-folder"></span>&nbsp;{$translations, t('Ignored Folders')}&nbsp;
          <span class="badge">{ignoredFoldersCount()}</span>
        </a>
      </li>
    </ul>

    <div class="tab-content" style="padding-top: 15px;">
      <!-- General Tab -->
      {#if activeTab === 'general'}
        <div class="form-group">
          <label for="settingsDeviceName">{$translations, t('Device Name')}</label>
          <input id="settingsDeviceName" class="form-control" type="text" bind:value={deviceName} />
        </div>

        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <label for="minHomeDiskFree">{$translations, t('Minimum Free Disk Space')}</label>
              {#if tmpOptions.minHomeDiskFree}
                <div class="row">
                  <div class="col-xs-9">
                    <input id="minHomeDiskFree" class="form-control" type="number" bind:value={tmpOptions.minHomeDiskFree.value} min="0" step="0.01" />
                  </div>
                  <div class="col-xs-3">
                    <select class="form-control" bind:value={tmpOptions.minHomeDiskFree.unit}>
                      <option value="%">%</option>
                      <option value="kB">kB</option>
                      <option value="MB">MB</option>
                      <option value="GB">GB</option>
                      <option value="TB">TB</option>
                    </select>
                  </div>
                </div>
                <p class="help-block">{t('This setting controls the free space required on the home (i.e., index database) disk.')}</p>
              {/if}
            </div>
          </div>
          <div class="col-md-6">
            <div class="form-group">
              <label>{$translations, t('API Key')}</label>
              <div class="input-group">
                <input type="text" readonly class="text-monospace form-control" value={tmpGUI.apiKey || '-'} />
                <span class="input-group-btn">
                  <button type="button" class="btn btn-default btn-secondary" onclick={generateAPIKey}>
                    <span class="fas fa-redo"></span>&nbsp;{t('Generate')}
                  </button>
                </span>
              </div>
            </div>
          </div>
        </div>

        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <label for="urAccepted">{$translations, t('Anonymous Usage Reporting')}</label>
              {#if tmpOptions.upgrades !== 'candidate' && !config?.version?.isCandidate}
                <select class="form-control" id="urAccepted" bind:value={tmpOptions.urAccepted}>
                  {#each urVersions() as v}
                    <option value={v}>{t('Version')} {v}</option>
                  {/each}
                  <option value={0}>{t('Undecided (will prompt)')}</option>
                  <option value={-1}>{t('Disabled')}</option>
                </select>
              {:else}
                <p class="help-block">{t('Usage reporting is always enabled for candidate releases.')}</p>
              {/if}
            </div>
          </div>
          <div class="col-md-6">
            <div class="form-group">
              <label>{$translations, t('Automatic upgrades')}</label>&emsp;<a href="{utils.docsURL(null, 'users/releases')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
              {#if upgradeInfo}
                <select class="form-control" bind:value={tmpOptions.upgrades}>
                  <option value="none">{t('No upgrades')}</option>
                  <option value="stable">{t('Stable releases only')}</option>
                  <option value="candidate">{t('Stable releases and release candidates')}</option>
                </select>
              {:else}
                <p class="help-block">{t('Unavailable/Disabled by administrator or maintainer')}</p>
              {/if}
            </div>
          </div>
        </div>

        <div>
          <label>{$translations, t('Default Configuration')}</label>
          <p>
            <button type="button" class="btn btn-default btn-secondary" onclick={editFolderDefaults}>
              {t('Edit Folder Defaults')}
            </button>
            <button type="button" class="btn btn-default btn-secondary" onclick={editDeviceDefaults}>
              {t('Edit Device Defaults')}
            </button>
          </p>
        </div>
      {/if}

      <!-- GUI Tab -->
      {#if activeTab === 'gui'}
        <div class="form-group">
          <label for="guiAddress">{$translations, t('GUI Listen Address')}</label>&emsp;<a href="{utils.docsURL(null, 'users/guilisten')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
          {#if system?.guiAddressOverridden}
            <p class="text-warning">
              <span class="fas fa-exclamation-triangle"></span>
              {t('The GUI address is overridden by startup options. Changes here will not take effect while the override is in place.')}
            </p>
          {/if}
          <input id="guiAddress" class="form-control" type="text" bind:value={tmpGUI.address} />
          <p class="help-block">{t('Enter a non-privileged port number (1024 - 65535).')}</p>
        </div>
        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <label for="guiUser">{$translations, t('GUI Authentication User')}</label>
              <input id="guiUser" class="form-control" type="text" bind:value={tmpGUI.user} autocomplete="username" />
            </div>
          </div>
          <div class="col-md-6">
            <div class="form-group">
              <label for="guiPassword">{$translations, t('GUI Authentication Password')}</label>
              <input id="guiPassword" class="form-control" type="password" bind:value={tmpGUI.password} autocomplete="new-password" />
            </div>
          </div>
        </div>
        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <div class="checkbox">
                <label>
                  <input type="checkbox" bind:checked={tmpGUI.useTLS} /> {$translations, t('Use HTTPS for GUI')}
                </label>
              </div>
            </div>
          </div>
          <div class="col-md-6">
            <div class="form-group">
              <div class="checkbox">
                <label>
                  <input type="checkbox" bind:checked={tmpOptions.startBrowser} /> {$translations, t('Start Browser')}
                </label>
              </div>
            </div>
          </div>
        </div>
        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <label>{$translations, t('GUI Theme')}</label>
              {#if themes.length > 1}
                <select class="form-control" bind:value={tmpGUI.theme}>
                  {#each themes.sort() as theme}
                    <option value={theme}>{themeName(theme)}</option>
                  {/each}
                </select>
              {:else}
                <p class="help-block">{t('Unavailable')}</p>
              {/if}
            </div>
          </div>
          <div class="col-md-6">
            {#if tmpGUI.address && (tmpGUI.address.startsWith('/') || tmpGUI.address.startsWith('unix:'))}
              <div class="form-group">
                <label>{$translations, t('UNIX Permissions')}</label>
                <input class="form-control" type="text" bind:value={tmpGUI.unixSocketPermissions} />
              </div>
            {/if}
            <!-- Language selector in settings -->
            <div class="form-group">
              <label>{$translations, t('Language')}</label>
              <select class="form-control" value={$currentLocale} onchange={(e) => useLocale(e.target.value, true)}>
                {#each availableLocales as loc}
                  <option value={loc.code}>{loc.name}</option>
                {/each}
              </select>
            </div>
          </div>
        </div>
      {/if}

      <!-- Connections Tab -->
      {#if activeTab === 'connections'}
        <div class="form-group">
          <label for="listenAddresses">{$translations, t('Sync Protocol Listen Addresses')}</label>&emsp;<a href="{utils.docsURL(null, 'users/config#listen-addresses')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
          <input id="listenAddresses" class="form-control" type="text" bind:value={tmpOptions._listenAddressesStr} />
        </div>
        <div class="row">
          <div class="col-md-6">
            <div class="form-group" class:has-error={dirty.maxRecvKbps && tmpOptions.maxRecvKbps < 0}>
              <label for="maxRecvKbps">{$translations, t('Incoming Rate Limit (KiB/s)')}</label>
              <input id="maxRecvKbps" class="form-control" type="number" bind:value={tmpOptions.maxRecvKbps} min="0" step="1024" oninput={() => dirty.maxRecvKbps = true} />
              {#if dirty.maxRecvKbps && tmpOptions.maxRecvKbps < 0}
                <p class="help-block">{t('The rate limit must be a non-negative number (0: no limit)')}</p>
              {/if}
            </div>
          </div>
          <div class="col-md-6">
            <div class="form-group" class:has-error={dirty.maxSendKbps && tmpOptions.maxSendKbps < 0}>
              <label for="maxSendKbps">{$translations, t('Outgoing Rate Limit (KiB/s)')}</label>
              <input id="maxSendKbps" class="form-control" type="number" bind:value={tmpOptions.maxSendKbps} min="0" step="1024" oninput={() => dirty.maxSendKbps = true} />
              {#if dirty.maxSendKbps && tmpOptions.maxSendKbps < 0}
                <p class="help-block">{t('The rate limit must be a non-negative number (0: no limit)')}</p>
              {/if}
            </div>
          </div>
        </div>
        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <div class="checkbox">
                <label>
                  <input type="checkbox" bind:checked={tmpOptions.limitBandwidthInLan} /> {$translations, t('Limit Bandwidth in LAN')}
                </label>
              </div>
            </div>
          </div>
        </div>
        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <div class="checkbox">
                <label>
                  <input type="checkbox" bind:checked={tmpOptions.natEnabled} /> {$translations, t('Enable NAT traversal')}
                </label>
              </div>
            </div>
          </div>
          <div class="col-md-6">
            <div class="form-group">
              <div class="checkbox">
                <label>
                  <input type="checkbox" bind:checked={tmpOptions.localAnnounceEnabled} /> {$translations, t('Local Discovery')}
                </label>
              </div>
            </div>
          </div>
        </div>
        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <div class="checkbox">
                <label>
                  <input type="checkbox" bind:checked={tmpOptions.globalAnnounceEnabled} /> {$translations, t('Global Discovery')}
                </label>
              </div>
            </div>
          </div>
          <div class="col-md-6">
            <div class="form-group">
              <div class="checkbox">
                <label>
                  <input type="checkbox" bind:checked={tmpOptions.relaysEnabled} /> {$translations, t('Enable Relaying')}
                </label>
              </div>
            </div>
          </div>
        </div>
        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <label for="globalAnnounce">{$translations, t('Global Discovery Servers')}</label>
              <input id="globalAnnounce" class="form-control" type="text" bind:value={tmpOptions._globalAnnounceServersStr}
                disabled={!tmpOptions.globalAnnounceEnabled} />
            </div>
          </div>
        </div>
      {/if}

      <!-- Ignored Devices Tab -->
      {#if activeTab === 'ignored-devices'}
        <div class="form-group">
          {#if tmpRemoteIgnoredDevices.length === 0}
            <span>{$translations, t('You have no ignored devices.')}</span>
          {:else}
            <div class="table-responsive">
              <table class="table-condensed table-striped table" style="table-layout: auto;">
                <thead>
                  <tr>
                    <th>{$translations, t('Ignored at')}</th>
                    <th>{$translations, t('Device')}</th>
                    <th>{$translations, t('Address')}</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {#each tmpRemoteIgnoredDevices as ignoredDevice}
                    <tr>
                      <td class="no-overflow-ellipse">{utils.formatDate(ignoredDevice.time)}</td>
                      <td>
                        {#if ignoredDevice.name}
                          <span use:tooltip={ignoredDevice.deviceID}>{ignoredDevice.name}</span>
                        {:else}
                          {ignoredDevice.deviceID}
                        {/if}
                      </td>
                      <td class="no-overflow-ellipse">{ignoredDevice.address || ''}</td>
                      <td>
                        <!-- svelte-ignore a11y_invalid_attribute -->
                        <a href="#" onclick={(e) => { e.preventDefault(); unignoreDevice(ignoredDevice); }}>
                          <span class="fas fa-times"></span>&nbsp;{t('Unignore')}
                        </a>
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Ignored Folders Tab -->
      {#if activeTab === 'ignored-folders'}
        <div class="form-group">
          {#if ignoredFoldersCount() === 0}
            <span>{$translations, t('You have no ignored folders.')}</span>
          {:else}
            <div class="table-responsive">
              <table class="table-condensed table-striped table" style="table-layout: auto;">
                <thead>
                  <tr>
                    <th>{$translations, t('Ignored at')}</th>
                    <th>{$translations, t('Folder')}</th>
                    <th>{$translations, t('Device')}</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {#each tmpDevices as device}
                    {#each (device.ignoredFolders || []) as ignoredFolder}
                      <tr>
                        <td class="no-overflow-ellipse">{utils.formatDate(ignoredFolder.time)}</td>
                        <td>{utils.folderLabel(get(configStore)?.folders ? Object.fromEntries((get(configStore).folders || []).map(f => [f.id, f])) : {}, ignoredFolder.id)}</td>
                        <td>
                          <span use:tooltip={device.deviceID}>{friendlyNameFromID(device.deviceID)}</span>
                        </td>
                        <td>
                          <!-- svelte-ignore a11y_invalid_attribute -->
                          <a href="#" onclick={(e) => { e.preventDefault(); unignoreFolder(device.deviceID, ignoredFolder.id); }}>
                            <span class="fas fa-times"></span>&nbsp;{t('Unignore')}
                          </a>
                        </td>
                      </tr>
                    {/each}
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  <div class="modal-footer">
    <button type="button" class="btn btn-primary btn-sm" onclick={saveSettings}>
      <span class="fas fa-check"></span>&nbsp;{$translations, t('Save')}
    </button>
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
  </div>
</Modal>
