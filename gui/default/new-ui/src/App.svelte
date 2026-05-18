<script>
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { init, authenticated, config, configInSync, myID, system, version, connections,
    connectionsTotal, completion, devices, devicesGrouped, folders, foldersGrouped,
    model, errors, seenError, pendingDevices, pendingFolders, deviceStats, folderStats,
    progress, scanProgress, discoveryCache, upgradeInfo, themes, globalChangeEvents,
    metricRates, online, restarting, listenersFailed, listenersRunning, listenersTotal,
    discoveryFailed, discoveryRunning, discoveryTotal, localStateTotal, openNoAuth,
    errorList, otherDevicesList, folderList as folderListStore, deviceList as deviceListStore,
    saveConfig, refresh, refreshCluster } from './lib/stores.js';
  import { api } from './lib/api.js';
  import * as utils from './lib/utils.js';
  import { t, translations } from './lib/i18n.js';
  import { tooltip } from './lib/tooltip.js';
  import Header from './components/Header.svelte';
  import FolderList from './components/FolderList.svelte';
  import DeviceList from './components/DeviceList.svelte';
  import LoginForm from './components/LoginForm.svelte';
  import Notifications from './components/Notifications.svelte';
  import Modal from './components/Modal.svelte';
  import FolderEdit from './components/modals/FolderEdit.svelte';
  import DeviceEdit from './components/modals/DeviceEdit.svelte';
  import Settings from './components/modals/Settings.svelte';
  import AdvancedSettings from './components/modals/AdvancedSettings.svelte';
  import About from './components/modals/About.svelte';
  import LogViewer from './components/modals/LogViewer.svelte';
  import ConnectivityStatus from './components/modals/ConnectivityStatus.svelte';
  import IdQR from './components/modals/IdQR.svelte';
  import GlobalChanges from './components/modals/GlobalChanges.svelte';
  import NeedFiles from './components/modals/NeedFiles.svelte';
  import FailedFiles from './components/modals/FailedFiles.svelte';
  import RemoteNeedFiles from './components/modals/RemoteNeedFiles.svelte';
  import LocalChanged from './components/modals/LocalChanged.svelte';
  import UsageReport from './components/modals/UsageReport.svelte';

  // Modal state
  let showSettingsModal = $state(false);
  let showAdvancedSettingsModal = $state(false);
  let showAboutModal = $state(false);
  let showLogViewerModal = $state(false);
  let showIdQRModal = $state(false);
  let showGlobalChangesModal = $state(false);
  let showFolderEditModal = $state(false);
  let showDeviceEditModal = $state(false);
  let showNeedModal = $state(false);
  let showFailedModal = $state(false);
  let showRemoteNeedModal = $state(false);
  let showLocalChangedModal = $state(false);
  let showUsageReportModal = $state(false);
  let showNetworkErrorModal = $state(false);
  let showRestartingModal = $state(false);
  let showShutdownModal = $state(false);
  let showUpgradeModal = $state(false);
  let showConnectivityModal = $state(false);
  let connectivityType = $state('listeners');

  // Current editing context
  let currentDevice = $state({});
  let currentFolder = $state({});
  let neededFolder = $state('');
  let failedFolder = $state('');
  let remoteNeedDevice = $state(null);
  let localChangedFolder = $state('');
  let localChangedType = $state('');
  let idQRDevice = $state({});

  // Restarting/shutdown tracking
  let isRestarting = $state(false);

  onMount(() => {
    init();
  });

  // ===== Actions =====

  function openSettings() {
    showSettingsModal = true;
  }

  function openAdvancedSettings() {
    showAdvancedSettingsModal = true;
  }

  function openAbout() {
    showAboutModal = true;
  }

  function openLogViewer() {
    showLogViewerModal = true;
  }

  function showDeviceIdentification(device) {
    idQRDevice = device;
    showIdQRModal = true;
  }

  function showListenersStatus() {
    connectivityType = 'listeners';
    showConnectivityModal = true;
  }

  function showDiscoveryStatus() {
    connectivityType = 'discovery';
    showConnectivityModal = true;
  }

  function openGlobalChanges() {
    showGlobalChangesModal = true;
  }

  function editFolder(folder, initialTab) {
    currentFolder = { ...folder, _editing: 'existing' };
    showFolderEditModal = true;
  }

  function addFolder() {
    api.getRandomString(10).then(data => {
      const folderID = (data.random.substr(0, 5) + '-' + data.random.substr(5, 5)).toLowerCase();
      api.getDefaultFolder().then(defaults => {
        currentFolder = { ...defaults, id: folderID, _editing: 'new' };
        showFolderEditModal = true;
      });
    });
  }

  function editDevice(device) {
    currentDevice = { ...device, _editing: 'existing' };
    showDeviceEditModal = true;
  }

  function editFolderDefaults() {
    const defaults = $config.defaults?.folder || {};
    const defaultIgnores = $config.defaults?.ignores || {};
    currentFolder = { ...defaults, _editing: 'defaults', _defaultIgnores: defaultIgnores };
    showFolderEditModal = true;
  }

  function editDeviceDefaults() {
    const defaults = $config.defaults?.device || {};
    currentDevice = { ...defaults, _editing: 'defaults' };
    showDeviceEditModal = true;
  }

  function addDevice(deviceID, name) {
    api.getDefaultDevice().then(defaults => {
      currentDevice = { ...defaults, deviceID: deviceID || '', name: name || '', _editing: deviceID ? 'new-pending' : 'new' };
      showDeviceEditModal = true;
    });
  }

  function showNeed(folder) {
    neededFolder = folder;
    showNeedModal = true;
  }

  function showFailed(folder) {
    failedFolder = folder;
    showFailedModal = true;
  }

  function showRemoteNeed(device) {
    remoteNeedDevice = device;
    showRemoteNeedModal = true;
  }

  function showLocalChanged(folder, type) {
    localChangedFolder = folder;
    localChangedType = type;
    showLocalChangedModal = true;
  }

  async function doRestart() {
    isRestarting = true;
    showRestartingModal = true;
    try {
      await api.postRestart();
      configInSync.set(true);
    } catch (e) {
      console.error('Restart error:', e);
    }
  }

  async function doShutdown() {
    try {
      await api.postShutdown();
      showShutdownModal = true;
      configInSync.set(true);
    } catch (e) {
      console.error('Shutdown error:', e);
    }
  }

  async function doUpgrade() {
    showUpgradeModal = false;
    isRestarting = true;
    try {
      await api.postUpgrade();
      showRestartingModal = true;
    } catch (e) {
      console.error('Upgrade error:', e);
    }
  }

  function doLogout() {
    api.logout().then(() => location.reload()).catch(e => console.error('Logout failed:', e));
  }

  function clearErrors() {
    const errs = get(errors);
    if (errs.length > 0) {
      seenError.set(errs[errs.length - 1].when);
    }
    api.clearErrors();
  }

  async function setFolderPause(folderId, pause) {
    folders.update(f => {
      if (f[folderId]) f[folderId].paused = pause;
      return { ...f };
    });
    config.update(c => {
      c.folders = Object.values(get(folders)).sort(utils.folderCompare);
      return { ...c };
    });
    await saveConfig();
  }

  async function setDevicePause(deviceId, pause) {
    devices.update(d => {
      if (d[deviceId]) d[deviceId].paused = pause;
      return { ...d };
    });
    config.update(c => {
      c.devices = Object.values(get(devices)).sort(utils.deviceCompare);
      return { ...c };
    });
    await saveConfig();
  }

  async function rescanFolder(folderId) {
    await api.postScan(folderId);
  }

  async function rescanAllFolders() {
    await api.postScan();
  }

  async function setAllFoldersPause(pause) {
    folders.update(f => {
      for (const id in f) f[id].paused = pause;
      return { ...f };
    });
    config.update(c => {
      c.folders = Object.values(get(folders)).sort(utils.folderCompare);
      return { ...c };
    });
    await saveConfig();
  }

  async function setAllDevicesPause(pause) {
    devices.update(d => {
      for (const id in d) d[id].paused = pause;
      return { ...d };
    });
    config.update(c => {
      c.devices = Object.values(get(devices)).sort(utils.deviceCompare);
      return { ...c };
    });
    await saveConfig();
  }

  function isAtleastOneFolderPausedStateSetTo(pause) {
    for (const f of Object.values(get(folders))) {
      if (f.paused === pause) return true;
    }
    return false;
  }

  function isAtleastOneDevicePausedStateSetTo(pause) {
    for (const d of Object.values(get(devices))) {
      if (d.paused === pause) return true;
    }
    return false;
  }

  function toggleUnits() {
    metricRates.update(v => {
      const newVal = !v;
      try { window.localStorage['metricRates'] = newVal; } catch (e) {}
      return newVal;
    });
  }

  function isAuthEnabled() {
    const cfg = get(config);
    const guiCfg = cfg && cfg.gui;
    if (guiCfg) {
      return guiCfg.authMode === 'ldap' || (guiCfg.user && guiCfg.password);
    }
    return false;
  }

  async function ignoreDevice(deviceID, pendingDevice) {
    const ignoredDevice = { ...pendingDevice, deviceID, time: new Date().toISOString() };
    config.update(c => {
      if (!c.remoteIgnoredDevices) c.remoteIgnoredDevices = [];
      c.remoteIgnoredDevices.push(ignoredDevice);
      return { ...c };
    });
    await saveConfig();
  }

  async function dismissPendingDevice(deviceID) {
    await api.dismissPendingDevice(deviceID);
  }

  async function addFolderAndShare(folderID, pendingFolder, deviceID) {
    const defaults = await api.getDefaultFolder();
    currentFolder = {
      ...defaults,
      id: folderID,
      label: pendingFolder.offeredBy[deviceID]?.label || '',
      _editing: 'new-pending',
      _shareWith: deviceID,
    };
    showFolderEditModal = true;
  }

  async function shareFolderWithDevice(folderID, deviceID) {
    folders.update(f => {
      if (f[folderID]) {
        f[folderID].devices.push({ deviceID });
      }
      return { ...f };
    });
    config.update(c => {
      c.folders = Object.values(get(folders)).sort(utils.folderCompare);
      return { ...c };
    });
    await saveConfig();
  }

  async function ignoreFolder(deviceID, folderID, offeringDevice) {
    devices.update(d => {
      if (d[deviceID]) {
        if (!d[deviceID].ignoredFolders) d[deviceID].ignoredFolders = [];
        d[deviceID].ignoredFolders.push({
          id: folderID,
          label: offeringDevice.label,
          time: new Date().toISOString(),
        });
      }
      return { ...d };
    });
    config.update(c => {
      c.devices = Object.values(get(devices)).sort(utils.deviceCompare);
      return { ...c };
    });
    await saveConfig();
  }

  async function dismissPendingFolder(folderID, deviceID) {
    await api.dismissPendingFolder(folderID, deviceID);
  }

  async function revertOverride(operation, folderID) {
    await api.post('db/' + operation + '?folder=' + encodeURIComponent(folderID));
  }

  // Expose global actions object for child components
  const actions = {
    openSettings, openAdvancedSettings, openAbout, openLogViewer, showDeviceIdentification, showListenersStatus, showDiscoveryStatus,
    editFolderDefaults, editDeviceDefaults,
    openGlobalChanges, editFolder, addFolder, editDevice, addDevice,
    showNeed, showFailed, showRemoteNeed, showLocalChanged,
    doRestart, doShutdown, doUpgrade, doLogout, clearErrors,
    setFolderPause, setDevicePause, rescanFolder, rescanAllFolders,
    setAllFoldersPause, setAllDevicesPause, toggleUnits,
    isAtleastOneFolderPausedStateSetTo, isAtleastOneDevicePausedStateSetTo,
    isAuthEnabled, ignoreDevice, dismissPendingDevice, addFolderAndShare,
    shareFolderWithDevice, ignoreFolder, dismissPendingFolder,
    revertOverride, saveConfig,
  };
</script>

<Header
  authenticated={$authenticated}
  version={$version}
  upgradeInfo={$upgradeInfo}
  {actions}
/>

<div class="container content">
  {#if $openNoAuth}
    <!-- Panel: Open, no auth -->
    <div class="row">
      <div class="col-md-12">
        <div class="panel panel-danger">
          <div class="panel-heading">
            <h3 class="panel-title">
              <div class="panel-icon">
                <span class="fas fa-exclamation-circle"></span>
              </div>
              {$translations, t('Danger!')}
            </h3>
          </div>
          <div class="panel-body">
            <p>
              {$translations, t('The Syncthing admin interface is configured to allow remote access without a password.')}
              <b>{$translations, t('This can easily give hackers access to read and change any files on your computer.')}</b>
              {$translations, t('Please set a GUI Authentication User and Password in the Settings dialog.')}
            </p>
          </div>
          <div class="panel-footer">
            <button type="button" class="btn btn-sm btn-default pull-right" onclick={() => actions.openSettings()}>
              <span class="fas fa-cog"></span>&nbsp;{$translations, t('Settings')}
            </button>
            <div class="clearfix"></div>
          </div>
        </div>
      </div>
    </div>
  {/if}

  {#if !$configInSync}
    <!-- Panel: Restart Needed -->
    <div class="row">
      <div class="col-md-12">
        <div class="panel panel-warning">
          <div class="panel-heading">
            <h3 class="panel-title">
              <div class="panel-icon">
                <span class="fas fa-exclamation-circle"></span>
              </div>
              {$translations, t('Restart Needed')}
            </h3>
          </div>
          <div class="panel-body">
            <p>{$translations, t('The configuration has been saved but not activated. Syncthing must restart to activate the new configuration.')}</p>
          </div>
          <div class="panel-footer">
            <button type="button" class="btn btn-sm btn-default pull-right" onclick={() => actions.doRestart()}>
              <span class="fas fa-refresh"></span>&nbsp;{$translations, t('Restart')}
            </button>
            <div class="clearfix"></div>
          </div>
        </div>
      </div>
    </div>
  {/if}

  {#if $config && $config.options}
    <!-- Notifications for pending devices -->
    {#each Object.entries($pendingDevices) as [deviceID, pendingDevice]}
      <div class="row">
        <div class="col-md-12">
          <div class="panel panel-warning">
            <div class="panel-heading">
              <h3 class="panel-title">
                <span class="fas fa-desktop panel-icon"></span>
                {$translations, t('New Device')}
                <span class="pull-right">{utils.formatDate(pendingDevice.time)}</span>
              </h3>
            </div>
            <div class="panel-body">
              <p>
                {$translations, t('Device "{%name%}" ({%device%} at {%address%}) wants to connect. Add new device?', { name: pendingDevice.name, device: deviceID, address: pendingDevice.address })}
              </p>
            </div>
            <div class="panel-footer clearfix">
              <div class="pull-right">
                <button type="button" class="btn btn-sm btn-success" onclick={() => actions.addDevice(deviceID, pendingDevice.name)}>
                  <span class="fas fa-plus"></span>&nbsp;{$translations, t('Add Device')}
                </button>
                <button type="button" class="btn btn-sm btn-danger" use:tooltip={t('Permanently add it to the ignore list, suppressing further notifications.')} onclick={() => actions.ignoreDevice(deviceID, pendingDevice)}>
                  <span class="fas fa-times"></span>&nbsp;{$translations, t('Ignore')}
                </button>
                <button type="button" class="btn btn-sm btn-default" use:tooltip={t('Do not add it to the ignore list, so this notification may recur.')} onclick={() => actions.dismissPendingDevice(deviceID)}>
                  <span class="far fa-clock"></span>&nbsp;{$translations, t('Dismiss')}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    {/each}

    <!-- Notifications for pending folders -->
    {#each Object.entries($pendingFolders) as [folderID, pendingFolder]}
      {#each Object.entries(pendingFolder.offeredBy || {}) as [deviceID, offeringDevice]}
        <div class="row">
          <div class="col-md-12">
            <div class="panel panel-warning">
              <div class="panel-heading">
                <h3 class="panel-title">
                  <div class="panel-icon">
                    <span class="fas fa-folder"></span>
                  </div>
                  {#if !$folders[folderID]}{$translations, t('New Folder')}{:else}{$translations, t('Share Folder')}{/if}
                  <span class="pull-right">{utils.formatDate(offeringDevice.time)}</span>
                </h3>
              </div>
              <div class="panel-body">
                <p>
                  {utils.deviceName($devices[deviceID])} {$translations, t('wants to share folder')}
                  {#if offeringDevice.label}"{offeringDevice.label}" ({folderID}){:else}"{folderID}"{/if}.
                  {#if $folders[folderID]}{$translations, t('Share this folder?')}{:else}{$translations, t('Add new folder?')}{/if}
                </p>
              </div>
              <div class="panel-footer clearfix">
                <div class="pull-right">
                  {#if !$folders[folderID]}
                    <button type="button" class="btn btn-sm btn-success" onclick={() => actions.addFolderAndShare(folderID, pendingFolder, deviceID)}>
                      <span class="fas fa-check"></span>&nbsp;{$translations, t('Add')}
                    </button>
                  {:else}
                    <button type="button" class="btn btn-sm btn-success" onclick={() => actions.shareFolderWithDevice(folderID, deviceID)}>
                      <span class="fas fa-check"></span>&nbsp;{$translations, t('Share')}
                    </button>
                  {/if}
                  <button type="button" class="btn btn-sm btn-danger" use:tooltip={t('Permanently add it to the ignore list, suppressing further notifications.')} onclick={() => actions.ignoreFolder(deviceID, folderID, offeringDevice)}>
                    <span class="fas fa-times"></span>&nbsp;{$translations, t('Ignore')}
                  </button>
                  <button type="button" class="btn btn-sm btn-default" use:tooltip={t('Do not add it to the ignore list, so this notification may recur.')} onclick={() => actions.dismissPendingFolder(folderID, deviceID)}>
                    <span class="far fa-clock"></span>&nbsp;{$translations, t('Dismiss')}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      {/each}
    {/each}
  {/if}

  <!-- Error notices -->
  {#if $errorList.length > 0}
    <div class="row">
      <div class="col-md-12">
        <div class="panel panel-warning">
          <div class="panel-heading">
            <h3 class="panel-title">
              <div class="panel-icon">
                <span class="fas fa-exclamation-circle"></span>
              </div>
              {$translations, t('Notice')}
            </h3>
          </div>
          <div class="panel-body">
            {#each $errorList as err}
              <p>
                <small>{utils.formatDate(err.when)}:</small>
                {err.message}
              </p>
            {/each}
          </div>
          <div class="panel-footer">
            <button type="button" class="btn btn-sm btn-default pull-right" onclick={() => {
              const errs = $errors;
              if (errs.length > 0) seenError.set(errs[errs.length - 1].when);
              api.clearErrors();
            }}>
              <span class="fas fa-check"></span>&nbsp;{$translations, t('OK')}
            </button>
            <div class="clearfix"></div>
          </div>
        </div>
      </div>
    </div>
  {/if}

  <!-- FS Watcher errors -->
  {#if Object.keys(utils.fsWatcherErrorMap($folders, $model)).length > 0}
    <div class="row">
      <div class="col-md-12">
        <div class="panel panel-warning">
          <div class="panel-heading">
            <h3 class="panel-title">
              <div class="panel-icon">
                <span class="fas fa-exclamation-circle"></span>
              </div>
              {$translations, t('Filesystem Watcher Errors')}
            </h3>
          </div>
          <div class="panel-body">
            <p>{$translations, t('For the following folders an error occurred while starting to watch for changes.')}</p>
            <table>
              <tbody>
                {#each Object.entries(utils.fsWatcherErrorMap($folders, $model)) as [id, err]}
                  <tr>
                    <td>{utils.folderLabel($folders, id)}: </td><td>{err}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  {/if}

  <!-- Login form -->
  {#if !$authenticated}
    <LoginForm />
  {/if}

  <!-- Main content -->
  {#if $authenticated}
    <div class="row">
      <!-- Folder list (left column) -->
      <div class="col-md-6">
        <FolderList
          foldersGrouped={$foldersGrouped}
          folders={$folders}
          model={$model}
          folderStats={$folderStats}
          completion={$completion}
          devices={$devices}
          myID={$myID}
          scanProgress={$scanProgress}
          {actions}
        />
      </div>

      <!-- Device list (right column) -->
      <div class="col-md-6">
        <DeviceList
          devicesGrouped={$devicesGrouped}
          devices={$devices}
          connections={$connections}
          connectionsTotal={$connectionsTotal}
          completion={$completion}
          deviceStats={$deviceStats}
          discoveryCache={$discoveryCache}
          config={$config}
          model={$model}
          folders={$folders}
          myID={$myID}
          system={$system}
          version={$version}
          localStateTotal={$localStateTotal}
          metricRates={$metricRates}
          listenersFailed={$listenersFailed}
          listenersRunning={$listenersRunning}
          listenersTotal={$listenersTotal}
          discoveryFailed={$discoveryFailed}
          discoveryTotal={$discoveryTotal}
          {actions}
        />
      </div>
    </div>
  {/if}
</div>

<!-- Modals -->
{#if showSettingsModal}
  <Settings
    config={$config}
    system={$system}
    devices={$devices}
    myID={$myID}
    themes={$themes}
    upgradeInfo={$upgradeInfo}
    onclose={() => showSettingsModal = false}
    {actions}
  />
{/if}

{#if showAdvancedSettingsModal}
  <AdvancedSettings
    config={$config}
    onclose={() => showAdvancedSettingsModal = false}
  />
{/if}

{#if showAboutModal}
  <About
    version={$version}
    system={$system}
    onclose={() => showAboutModal = false}
  />
{/if}

{#if showLogViewerModal}
  <LogViewer onclose={() => showLogViewerModal = false} />
{/if}

{#if showConnectivityModal}
  <ConnectivityStatus
    type={connectivityType}
    listenersRunning={$listenersRunning}
    listenersFailed={$listenersFailed}
    discoveryRunning={$discoveryRunning}
    discoveryFailed={$discoveryFailed}
    onclose={() => showConnectivityModal = false}
  />
{/if}

{#if showIdQRModal}
  <IdQR
    device={idQRDevice}
    onclose={() => showIdQRModal = false}
  />
{/if}

{#if showGlobalChangesModal}
  <GlobalChanges
    events={$globalChangeEvents}
    devices={$devices}
    onclose={() => showGlobalChangesModal = false}
  />
{/if}

{#if showFolderEditModal}
  <FolderEdit
    folder={currentFolder}
    devices={$devices}
    myID={$myID}
    system={$system}
    config={$config}
    onclose={() => showFolderEditModal = false}
    {actions}
  />
{/if}

{#if showDeviceEditModal}
  <DeviceEdit
    device={currentDevice}
    folders={$folders}
    myID={$myID}
    onclose={() => showDeviceEditModal = false}
    {actions}
  />
{/if}

{#if showNeedModal}
  <NeedFiles
    folder={neededFolder}
    model={$model}
    progress={$progress}
    config={$config}
    folders={$folders}
    onclose={() => { showNeedModal = false; neededFolder = ''; }}
  />
{/if}

{#if showFailedModal}
  <FailedFiles
    folder={failedFolder}
    model={$model}
    onclose={() => { showFailedModal = false; failedFolder = ''; }}
  />
{/if}

{#if showRemoteNeedModal}
  <RemoteNeedFiles
    device={remoteNeedDevice}
    completion={$completion}
    folders={$folders}
    devices={$devices}
    onclose={() => { showRemoteNeedModal = false; remoteNeedDevice = null; }}
  />
{/if}

{#if showLocalChangedModal}
  <LocalChanged
    folder={localChangedFolder}
    folderType={localChangedType}
    model={$model}
    onclose={() => { showLocalChangedModal = false; localChangedFolder = ''; }}
  />
{/if}

{#if showUsageReportModal}
  <UsageReport
    system={$system}
    config={$config}
    onclose={() => showUsageReportModal = false}
    {actions}
  />
{/if}

<!-- Network error modal -->
{#if showNetworkErrorModal}
  <Modal title={t('Connection Error')} onclose={() => showNetworkErrorModal = false}>
    <p>{$translations, t('Syncthing seems to be experiencing a problem processing your request. Please refresh the page or try again later.')}</p>
  </Modal>
{/if}

<!-- Restarting modal -->
{#if showRestartingModal}
  <Modal title={t('Restarting')} onclose={() => {}}>
    <div class="text-center">
      <p>{$translations, t('Syncthing is restarting.')}</p>
      <p><span class="fas fa-spinner fa-spin fa-3x"></span></p>
    </div>
  </Modal>
{/if}

<!-- Shutdown modal -->
{#if showShutdownModal}
  <Modal title={t('Syncthing has been shut down.')} onclose={() => {}}>
    <p>{$translations, t('Syncthing has been shut down.')}</p>
  </Modal>
{/if}
