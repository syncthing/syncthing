<script>
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import { folders, config, devices as devicesStore } from '../../lib/stores.js';
  import * as utils from '../../lib/utils.js';
  import { get } from 'svelte/store';
  import { t, translations } from '../../lib/i18n.js';
  import { tooltip } from '../../lib/tooltip.js';

  let { folder: initialFolder, devices, myID, system, config: cfgProp, onclose, actions } = $props();

  let folder = $state({ ...initialFolder });
  let activeTab = $state('general');
  let ignoresText = $state('');
  let ignoresError = $state(null);
  let ignoresDisabled = $state(false);

  // Field dirty tracking (set on user input)
  let dirty = $state({});

  // Versioning GUI state
  let versioningSelector = $state('none');
  let versioningTrashcanClean = $state(0);
  let versioningSimpleKeep = $state(5);
  let versioningStaggeredMaxAge = $state(365);
  let versioningExternalCommand = $state('');
  let versioningCleanupIntervalS = $state(3600);
  let versioningFsPath = $state('');

  // Sharing state
  let sharedDevices = $state([]);
  let unrelatedDevices = $state([]);
  let selectedDevices = $state({});
  let encryptionPasswords = $state({});
  let addIgnores = $state(false);

  // Password visibility toggle per device
  let passwordPlain = $state({});

  // XAttr filter state
  let xattrEntries = $state([]);
  let xattrMaxSingleEntrySize = $state(1024);
  let xattrMaxTotalSize = $state(4096);

  $effect(() => {
    initVersioning();
    initSharing();
    initXattr();
    if (folder._editing === 'existing') {
      loadIgnores();
    } else if (folder._editing === 'defaults') {
      // Load default ignore patterns from config
      const defaultIgnores = initialFolder._defaultIgnores?.lines || [];
      ignoresText = defaultIgnores.join('\n');
    }
  });

  function initXattr() {
    if (folder.xattrFilter) {
      xattrEntries = (folder.xattrFilter.entries || []).map(e => ({ ...e }));
      xattrMaxSingleEntrySize = folder.xattrFilter.maxSingleEntrySize || 1024;
      xattrMaxTotalSize = folder.xattrFilter.maxTotalSize || 4096;
    }
  }

  function newXattrEntry() {
    xattrEntries = [...xattrEntries, { match: '', permit: true }];
  }

  function removeXattrEntry(entry) {
    xattrEntries = xattrEntries.filter(e => e !== entry);
  }

  function getXattrDefault() {
    if (!xattrEntries || xattrEntries.length === 0) return t('permit');
    if (xattrEntries[xattrEntries.length - 1].match !== '*') return t('deny');
    return '';
  }

  function getXattrHint() {
    if (!xattrEntries || xattrEntries.length === 0) return '';
    if (xattrEntries.length === 1 && xattrEntries[0].match === '*') return '';
    if (xattrEntries.every(e => e.permit === false)) {
      return t('Hint: only deny-rules detected while the default is deny. Consider adding "permit any" as last rule.');
    }
    return '';
  }

  function setFSWatcherIntervalDefault() {
    if (folder.fsWatcherEnabled && folder.rescanIntervalS === 0) {
      folder.rescanIntervalS = 3600;
    }
  }

  function setDefaultsForFolderType() {
    if (folder.type === 'receiveencrypted') {
      folder.fsWatcherEnabled = false;
      folder.ignorePerms = true;
      folder.versioning = { type: '' };
      versioningSelector = 'none';
    } else {
      folder.fsWatcherEnabled = true;
    }
    if (folder._editing !== 'existing') {
      folder.blockIndexing = (folder.type === 'sendreceive' || folder.type === 'receiveonly');
    }
    setFSWatcherIntervalDefault();
  }

  function initVersioning() {
    if (!folder.versioning || !folder.versioning.type || folder.versioning.type === 'none') {
      versioningSelector = 'none';
      return;
    }
    versioningSelector = folder.versioning.type;
    versioningCleanupIntervalS = +(folder.versioning.cleanupIntervalS || 3600);
    versioningFsPath = folder.versioning.fsPath || '';
    switch (folder.versioning.type) {
      case 'trashcan':
        versioningTrashcanClean = +(folder.versioning.params?.cleanoutDays || 0);
        break;
      case 'simple':
        versioningSimpleKeep = +(folder.versioning.params?.keep || 5);
        versioningTrashcanClean = +(folder.versioning.params?.cleanoutDays || 0);
        break;
      case 'staggered':
        versioningStaggeredMaxAge = Math.floor(+(folder.versioning.params?.maxAge || 31536000) / 86400);
        break;
      case 'external':
        versioningExternalCommand = folder.versioning.params?.command || '';
        break;
    }
  }

  function initSharing() {
    const shared = [];
    const selected = {};
    const encPw = {};

    if (folder.devices) {
      folder.devices.forEach(d => {
        if (d.deviceID !== myID) {
          if (devices[d.deviceID]) {
            shared.push(devices[d.deviceID]);
          }
          selected[d.deviceID] = true;
          if (d.encryptionPassword) {
            encPw[d.deviceID] = d.encryptionPassword;
          }
        } else {
          selected[d.deviceID] = true;
        }
      });
    }

    // If adding from pending, auto-select the sharing device
    if (folder._shareWith) {
      selected[folder._shareWith] = true;
    }

    const unrelated = Object.values(devices).filter(d =>
      d.deviceID !== myID && !selected[d.deviceID]
    );

    if (folder._editing === 'new' || folder._editing === 'new-pending') {
      sharedDevices = [];
      unrelatedDevices = Object.values(devices).filter(d => d.deviceID !== myID);
      if (folder._shareWith) {
        selectedDevices = { [folder._shareWith]: true };
      } else {
        selectedDevices = {};
      }
    } else {
      sharedDevices = shared.sort(utils.deviceCompare);
      unrelatedDevices = unrelated.sort(utils.deviceCompare);
      selectedDevices = selected;
    }
    encryptionPasswords = encPw;
  }

  async function loadIgnores() {
    ignoresDisabled = true;
    ignoresText = t('Loading...');
    try {
      const data = await api.getIgnores(folder.id);
      ignoresText = data.ignore ? data.ignore.join('\n') : '';
      ignoresError = data.error;
      ignoresDisabled = false;
    } catch (e) {
      ignoresText = t('Failed to load ignore patterns.');
      ignoresDisabled = true;
    }
  }

  function internalVersioningEnabled() {
    return ['trashcan', 'simple', 'staggered'].includes(versioningSelector);
  }

  function editingFolderExisting() {
    return folder._editing === 'existing';
  }

  function editingFolderNew() {
    return folder._editing === 'new' || folder._editing === 'new-pending';
  }

  function editingFolderDefaults() {
    return folder._editing === 'defaults';
  }

  async function saveFolder() {
    // Build versioning config
    if (!folder.versioning) folder.versioning = { params: {} };
    folder.versioning.type = versioningSelector;
    if (internalVersioningEnabled()) {
      folder.versioning.cleanupIntervalS = versioningCleanupIntervalS;
      folder.versioning.fsPath = versioningFsPath;
    }
    switch (versioningSelector) {
      case 'trashcan':
        folder.versioning.params = { cleanoutDays: '' + versioningTrashcanClean };
        break;
      case 'simple':
        folder.versioning.params = { keep: '' + versioningSimpleKeep, cleanoutDays: '' + versioningTrashcanClean };
        break;
      case 'staggered':
        folder.versioning.params = { maxAge: '' + (versioningStaggeredMaxAge * 86400) };
        break;
      case 'external':
        folder.versioning.params = { command: '' + versioningExternalCommand };
        break;
      default:
        folder.versioning = { type: '' };
    }

    // Build xattr filter
    if (folder.syncXattrs || folder.sendXattrs) {
      if (!folder.xattrFilter) folder.xattrFilter = {};
      folder.xattrFilter.entries = xattrEntries;
      folder.xattrFilter.maxSingleEntrySize = xattrMaxSingleEntrySize;
      folder.xattrFilter.maxTotalSize = xattrMaxTotalSize;
    }

    // Build devices list from sharing selection
    selectedDevices[myID] = true;
    const newDevices = [];
    if (folder.devices) {
      folder.devices.forEach(dev => {
        if (selectedDevices[dev.deviceID]) {
          dev.encryptionPassword = encryptionPasswords[dev.deviceID] || '';
          newDevices.push(dev);
          delete selectedDevices[dev.deviceID];
        }
      });
    }
    for (const deviceID in selectedDevices) {
      if (selectedDevices[deviceID]) {
        newDevices.push({
          deviceID,
          encryptionPassword: encryptionPasswords[deviceID] || '',
        });
      }
    }
    folder.devices = newDevices;

    // Clean up temp props
    const saveCfg = { ...folder };
    delete saveCfg._editing;
    delete saveCfg._shareWith;
    delete saveCfg._guiVersioning;
    delete saveCfg._recvEnc;
    delete saveCfg._addIgnores;

    // Update stores
    if (folder._editing === 'defaults') {
      config.update(c => {
        c.defaults.folder = saveCfg;
        return { ...c };
      });
    } else {
      folders.update(f => {
        f[saveCfg.id] = saveCfg;
        return { ...f };
      });
      config.update(c => {
        c.folders = Object.values(get(folders)).sort(utils.folderCompare);
        return { ...c };
      });
    }

    // Save ignores for existing folders
    if (folder._editing === 'existing' && !ignoresDisabled) {
      const ignores = ignoresText.split('\n').filter(l => l !== '' || ignoresText === '');
      try {
        await api.postIgnores(folder.id, ignores.length === 1 && ignores[0] === '' ? [] : ignores);
      } catch (e) {
        console.error('Error saving ignores:', e);
      }
    }

    await actions.saveConfig();
    onclose();
  }

  async function deleteFolder() {
    if (folder._editing !== 'existing') return;

    folders.update(f => {
      delete f[folder.id];
      return { ...f };
    });
    config.update(c => {
      c.folders = Object.values(get(folders)).sort(utils.folderCompare);
      return { ...c };
    });

    await actions.saveConfig();
    onclose();
  }

  function modalTitle() {
    if (folder._editing === 'defaults') return t('Edit Folder Defaults');
    if (folder._editing === 'existing') return t('Edit Folder') + ' (' + utils.folderLabel(get(folders), folder.id) + ')';
    return t('Add Folder');
  }

  function folderTypeText(type) {
    switch (type || folder.type) {
      case 'sendreceive': return t('Send & Receive');
      case 'sendonly': return t('Send Only');
      case 'receiveonly': return t('Receive Only');
      case 'receiveencrypted': return t('Receive Encrypted');
      default: return type || folder.type;
    }
  }

  function selectAllSharedDevices(val) {
    sharedDevices.forEach(d => { selectedDevices[d.deviceID] = val; });
    selectedDevices = { ...selectedDevices };
  }

  function selectAllUnrelatedDevices(val) {
    unrelatedDevices.forEach(d => { selectedDevices[d.deviceID] = val; });
    selectedDevices = { ...selectedDevices };
  }
</script>

<Modal title={modalTitle()} icon={folder._editing === 'existing' ? 'fas fa-pencil-alt' : 'fas fa-folder'} large={true} {onclose}>
  <div class="modal-body">
    <!-- Tabs -->
    <ul class="nav nav-tabs">
      <li class:active={activeTab === 'general'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'general'; }}>
          <span class="fas fa-cog"></span> {$translations, t('General')}
        </a>
      </li>
      <li class:active={activeTab === 'sharing'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'sharing'; }}>
          <span class="fas fa-share-alt"></span> {$translations, t('Sharing')}
        </a>
      </li>
      <li class:active={activeTab === 'versioning'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'versioning'; }}>
          <span class="fa fa-files-o"></span> {$translations, t('File Versioning')}
        </a>
      </li>
      <li class:active={activeTab === 'ignores'} class:disabled={folder.type === 'receiveencrypted'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); if (folder.type !== 'receiveencrypted') activeTab = 'ignores'; }}>
          <span class="fas fa-filter"></span> {$translations, t('Ignore Patterns')}
        </a>
      </li>
      <li class:active={activeTab === 'advanced'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'advanced'; }}>
          <span class="fas fa-cogs"></span> {$translations, t('Advanced')}
        </a>
      </li>
    </ul>

    <div class="tab-content" style="padding-top: 15px;">
      <!-- General Tab -->
      {#if activeTab === 'general'}
        <div class="form-group">
          <label for="folderLabel">{$translations, t('Folder Label')}</label>
          <input id="folderLabel" class="form-control" type="text" bind:value={folder.label} />
          <p class="help-block">{t('Optional descriptive label for the folder. Can be different on each device.')}</p>
        </div>
        <div class="form-group">
          <label for="folderGroup">{$translations, t('Folder Group')}</label>
          <input id="folderGroup" class="form-control" type="text" bind:value={folder.group} />
          <p class="help-block">{t('Optional group for the folder. Can be different on each device.')}</p>
        </div>
        {#if !editingFolderDefaults()}
          <div class="form-group" class:has-error={dirty.folderID && !folder.id}>
            <label for="folderID">{$translations, t('Folder ID')}</label>
            <input id="folderID" class="form-control" type="text" bind:value={folder.id}
              oninput={() => dirty.folderID = true}
              disabled={editingFolderExisting() || folder._editing === 'new-pending'} required />
            <p class="help-block">
              {#if dirty.folderID && !folder.id}
                {t('The folder ID cannot be blank.')}
              {:else}
                {t('Required identifier for the folder. Must be the same on all cluster devices.')}
              {/if}
              {#if !editingFolderExisting()}
                <span>{t('When adding a new folder, keep in mind that the Folder ID is used to tie folders together between devices. They are case sensitive and must match exactly between all devices.')}</span>
              {/if}
            </p>
          </div>
        {/if}
        <div class="form-group" class:has-error={dirty.folderPath && !folder.path && !editingFolderDefaults()}>
          <label for="folderPath">{$translations, t('Folder Path')}</label>
          <input id="folderPath" class="form-control" type="text" bind:value={folder.path}
            oninput={() => dirty.folderPath = true}
            disabled={editingFolderExisting()} />
          <p class="help-block">
            {#if dirty.folderPath && !folder.path && !editingFolderDefaults()}
              {t('The folder path cannot be blank.')}
            {:else}
              {t('Path to the folder on the local computer. Will be created if it does not exist. The tilde character (~) can be used as a shortcut for')} <code>{system?.tilde || '~'}</code>.
            {/if}
          </p>
        </div>
      {/if}

      <!-- Sharing Tab -->
      {#if activeTab === 'sharing'}
        {#if sharedDevices.length > 0}
          <div class="form-horizontal">
            <label>{$translations, t('Currently Shared With Devices')}</label>
            <p class="help-block">
              {t('Deselect devices to stop sharing this folder with.')}&emsp;
              <!-- svelte-ignore a11y_invalid_attribute -->
              <small><a href="#" onclick={(e) => { e.preventDefault(); selectAllSharedDevices(true); }}>{t('Select All')}</a>&emsp;
              <!-- svelte-ignore a11y_invalid_attribute -->
              <a href="#" onclick={(e) => { e.preventDefault(); selectAllSharedDevices(false); }}>{t('Deselect All')}</a></small>
            </p>
            {#each sharedDevices as dev}
              <div class="form-group">
                <div class="col-md-6 checkbox">
                  <label for="sharedwith-{dev.deviceID}">
                    <input id="sharedwith-{dev.deviceID}" type="checkbox" bind:checked={selectedDevices[dev.deviceID]} />
                    <span use:tooltip={dev.deviceID}>{utils.deviceName(dev)}</span>
                  </label>
                </div>
                <div class="col-md-6">
                  <div class="input-group">
                    <span class="input-group-addon">
                      {#if folder.type !== 'receiveencrypted' && !encryptionPasswords[dev.deviceID]}
                        <span class="fas fa-fw fa-unlock"></span>
                      {:else}
                        <span class="fas fa-fw fa-lock"></span>
                      {/if}
                    </span>
                    {#if folder.type === 'receiveencrypted'}
                      <input class="form-control input-sm" type="password" placeholder="{t('Received data is already encrypted')}" disabled />
                    {:else if selectedDevices[dev.deviceID]}
                      <input class="form-control input-sm" type="{passwordPlain[dev.deviceID] ? 'text' : 'password'}" bind:value={encryptionPasswords[dev.deviceID]} autocomplete="off" placeholder="{t('If untrusted, enter encryption password')}" />
                    {:else}
                      <input class="form-control input-sm" type="password" placeholder="{t('Not shared')}" disabled />
                    {/if}
                    <span class="input-group-addon">
                      {#if selectedDevices[dev.deviceID] && folder.type !== 'receiveencrypted'}
                        <span class="button fas fa-fw {passwordPlain[dev.deviceID] ? 'fa-eye-slash' : 'fa-eye'}" onclick={() => passwordPlain[dev.deviceID] = !passwordPlain[dev.deviceID]}></span>
                      {:else}
                        <span class="button fas fa-fw fa-eye" style="opacity: 0.5"></span>
                      {/if}
                    </span>
                  </div>
                </div>
              </div>
            {/each}
          </div>
        {/if}
        <div class="form-horizontal">
          <label>{$translations, t('Unshared Devices')}</label>
          {#if Object.values(devices).filter(d => d.deviceID !== myID).length > 0}
            <p class="help-block">
              {t('Select additional devices to share this folder with.')}&emsp;
              <!-- svelte-ignore a11y_invalid_attribute -->
              <small><a href="#" onclick={(e) => { e.preventDefault(); selectAllUnrelatedDevices(true); }}>{t('Select All')}</a>&emsp;
              <!-- svelte-ignore a11y_invalid_attribute -->
              <a href="#" onclick={(e) => { e.preventDefault(); selectAllUnrelatedDevices(false); }}>{t('Deselect All')}</a></small>
            </p>
          {:else}
            <p class="help-block">{t('There are no devices to share this folder with.')}</p>
          {/if}
          {#each unrelatedDevices as dev}
            <div class="form-group">
              <div class="col-md-6 checkbox">
                <label for="sharedwith-{dev.deviceID}">
                  <input id="sharedwith-{dev.deviceID}" type="checkbox" bind:checked={selectedDevices[dev.deviceID]} />
                  <span use:tooltip={dev.deviceID}>{utils.deviceName(dev)}</span>
                </label>
              </div>
              <div class="col-md-6">
                <div class="input-group">
                  <span class="input-group-addon">
                    {#if folder.type !== 'receiveencrypted' && !encryptionPasswords[dev.deviceID]}
                      <span class="fas fa-fw fa-unlock"></span>
                    {:else}
                      <span class="fas fa-fw fa-lock"></span>
                    {/if}
                  </span>
                  {#if folder.type === 'receiveencrypted'}
                    <input class="form-control input-sm" type="password" placeholder="{t('Received data is already encrypted')}" disabled />
                  {:else if selectedDevices[dev.deviceID]}
                    <input class="form-control input-sm" type="{passwordPlain[dev.deviceID] ? 'text' : 'password'}" bind:value={encryptionPasswords[dev.deviceID]} autocomplete="off" placeholder="{t('If untrusted, enter encryption password')}" />
                  {:else}
                    <input class="form-control input-sm" type="password" placeholder="{t('Not shared')}" disabled />
                  {/if}
                  <span class="input-group-addon">
                    {#if selectedDevices[dev.deviceID] && folder.type !== 'receiveencrypted'}
                      <span class="button fas fa-fw {passwordPlain[dev.deviceID] ? 'fa-eye-slash' : 'fa-eye'}" onclick={() => passwordPlain[dev.deviceID] = !passwordPlain[dev.deviceID]}></span>
                    {:else}
                      <span class="button fas fa-fw fa-eye" style="opacity: 0.5"></span>
                    {/if}
                  </span>
                </div>
              </div>
            </div>
          {/each}
        </div>
      {/if}

      <!-- Versioning Tab -->
      {#if activeTab === 'versioning'}
        <div class="form-group">
          <label>{$translations, t('File Versioning')}</label>&emsp;<a href="{utils.docsURL(null, 'users/versioning')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
          <select class="form-control" bind:value={versioningSelector}>
            <option value="none">{t('No File Versioning')}</option>
            <option value="trashcan">{t('Trash Can File Versioning')}</option>
            <option value="simple">{t('Simple File Versioning')}</option>
            <option value="staggered">{t('Staggered File Versioning')}</option>
            <option value="external">{t('External File Versioning')}</option>
          </select>
        </div>

        {#if versioningSelector === 'trashcan'}
          <p class="help-block">{t('Files are moved to .stversions directory when replaced or deleted by Syncthing.')}</p>
          <div class="form-group" class:has-error={dirty.trashcanClean && (versioningTrashcanClean === '' || versioningTrashcanClean === null || versioningTrashcanClean < 0)}>
            <label for="trashcanClean">{$translations, t('Clean out after')}</label>
            <div class="input-group">
              <input id="trashcanClean" class="form-control text-right" type="number" bind:value={versioningTrashcanClean} min="0" required oninput={() => dirty.trashcanClean = true} />
              <div class="input-group-addon">{t('days')}</div>
            </div>
            <p class="help-block">
              {#if dirty.trashcanClean && (versioningTrashcanClean === '' || versioningTrashcanClean === null)}
                {t('The number of days must be a number and cannot be blank.')}
              {:else if dirty.trashcanClean && versioningTrashcanClean < 0}
                {t("A negative number of days doesn't make sense.")}
              {:else}
                {t('The number of days to keep files in the trash can. Zero means forever.')}
              {/if}
            </p>
          </div>
        {/if}

        {#if versioningSelector === 'simple'}
          <p class="help-block">{t('Files are moved to date stamped versions in a .stversions directory when replaced or deleted by Syncthing.')}</p>
          <div class="form-group" class:has-error={dirty.trashcanClean && (versioningTrashcanClean === '' || versioningTrashcanClean === null || versioningTrashcanClean < 0)}>
            <label for="trashcanClean">{$translations, t('Clean out after')}</label>
            <div class="input-group">
              <input id="trashcanClean" class="form-control text-right" type="number" bind:value={versioningTrashcanClean} min="0" required oninput={() => dirty.trashcanClean = true} />
              <div class="input-group-addon">{t('days')}</div>
            </div>
            <p class="help-block">
              {#if dirty.trashcanClean && (versioningTrashcanClean === '' || versioningTrashcanClean === null)}
                {t('The number of days must be a number and cannot be blank.')}
              {:else if dirty.trashcanClean && versioningTrashcanClean < 0}
                {t("A negative number of days doesn't make sense.")}
              {:else}
                {t('The number of days to keep files in the trash can. Zero means forever.')}
              {/if}
            </p>
          </div>
          <div class="form-group" class:has-error={dirty.simpleKeep && (versioningSimpleKeep === '' || versioningSimpleKeep === null || versioningSimpleKeep < 1)}>
            <label for="simpleKeep">{$translations, t('Keep Versions')}</label>
            <input id="simpleKeep" class="form-control" type="number" bind:value={versioningSimpleKeep} min="1" required oninput={() => dirty.simpleKeep = true} />
            <p class="help-block">
              {#if dirty.simpleKeep && (versioningSimpleKeep === '' || versioningSimpleKeep === null)}
                {t('The number of versions must be a number and cannot be blank.')}
              {:else if dirty.simpleKeep && versioningSimpleKeep < 1}
                {t('You must keep at least one version.')}
              {:else}
                {t('The number of old versions to keep, per file.')}
              {/if}
            </p>
          </div>
        {/if}

        {#if versioningSelector === 'staggered'}
          <p class="help-block">{t('Files are moved to date stamped versions in a .stversions directory when replaced or deleted by Syncthing.')} {t('Versions are automatically deleted if they are older than the maximum age or exceed the number of files allowed in an interval.')}</p>
          <p class="help-block">{t('The following intervals are used: for the first hour a version is kept every 30 seconds, for the first day a version is kept every hour, for the first 30 days a version is kept every day, until the maximum age a version is kept every week.')}</p>
          <div class="form-group" class:has-error={dirty.staggeredMaxAge && (versioningStaggeredMaxAge === '' || versioningStaggeredMaxAge === null || versioningStaggeredMaxAge < 0)}>
            <label for="staggeredMaxAge">{$translations, t('Maximum Age')}</label>
            <div class="input-group">
              <input id="staggeredMaxAge" class="form-control text-right" type="number" bind:value={versioningStaggeredMaxAge} min="0" required oninput={() => dirty.staggeredMaxAge = true} />
              <div class="input-group-addon">{t('days')}</div>
            </div>
            <p class="help-block">
              {#if dirty.staggeredMaxAge && (versioningStaggeredMaxAge === '' || versioningStaggeredMaxAge === null)}
                {t('The maximum age must be a number and cannot be blank.')}
              {:else if dirty.staggeredMaxAge && versioningStaggeredMaxAge < 0}
                {t("A negative number of days doesn't make sense.")}
              {:else}
                {t('The maximum time to keep a version (in days, set to 0 to keep versions forever).')}
              {/if}
            </p>
          </div>
        {/if}

        {#if versioningSelector === 'external'}
          <p class="help-block">{t('An external command handles the versioning. It has to remove the file from the shared folder. If the path to the application contains spaces, it should be quoted.')}</p>
          <div class="form-group" class:has-error={dirty.externalCommand && !versioningExternalCommand}>
            <label for="externalCommand">{$translations, t('Command')}</label>
            <input id="externalCommand" class="form-control" type="text" bind:value={versioningExternalCommand} required oninput={() => dirty.externalCommand = true} />
            <p class="help-block">
              {#if dirty.externalCommand && !versioningExternalCommand}
                {t('The path cannot be blank.')}
              {:else}
                {t('See external versioning help for supported templated command line parameters.')}
              {/if}
            </p>
          </div>
        {/if}

        {#if internalVersioningEnabled()}
          <div class="form-group">
            <label for="versionsFsPath">{$translations, t('Versions Path')}</label>
            <input id="versionsFsPath" class="form-control" type="text" bind:value={versioningFsPath} />
            <p class="help-block">{t('Path where versions should be stored (leave empty for the default .stversions directory in the shared folder).')}</p>
          </div>
          <div class="form-group" class:has-error={dirty.cleanupInterval && (versioningCleanupIntervalS === '' || versioningCleanupIntervalS === null || versioningCleanupIntervalS < 0)}>
            <label for="cleanupInterval">{$translations, t('Cleanup Interval')}</label>
            <div class="input-group">
              <input id="cleanupInterval" class="form-control text-right" type="number" bind:value={versioningCleanupIntervalS} min="0" max="31536000" step="3600" required oninput={() => dirty.cleanupInterval = true} />
              <div class="input-group-addon">{t('seconds')}</div>
            </div>
            <p class="help-block">
              {#if dirty.cleanupInterval && (versioningCleanupIntervalS === '' || versioningCleanupIntervalS === null)}
                {t('The cleanup interval cannot be blank.')}
              {:else if dirty.cleanupInterval && versioningCleanupIntervalS < 0}
                {t('The interval must be a positive number of seconds.')}
              {:else}
                {t('The interval, in seconds, for running cleanup in the versions directory. Zero to disable periodic cleaning.')}
              {/if}
            </p>
          </div>
        {/if}
      {/if}

      <!-- Ignores Tab -->
      {#if activeTab === 'ignores'}
        {#if editingFolderNew()}
          <label>
            <input type="checkbox" bind:checked={addIgnores} />&nbsp;{$translations, t('Add ignore patterns')}
          </label>
          <p class="help-block">{t('Ignore patterns can only be added after the folder is created. If checked, an input field to enter ignore patterns will be presented after saving.')}</p>
        {:else}
          <p>{$translations, t('Enter ignore patterns, one per line.')}</p>
          {#if ignoresError}
            <div class="has-error">
              <p class="help-block">{ignoresError}</p>
            </div>
          {/if}
          <textarea class="form-control" rows="5" bind:value={ignoresText}
            disabled={ignoresDisabled}></textarea>
          <hr />
          <p class="small">{t('Quick guide to supported patterns')} (<a href="{utils.docsURL(null, 'users/ignoring')}" target="_blank">{t('full documentation')}</a>):</p>
          <dl class="dl-horizontal dl-narrow small">
            <dt><code>(?d)</code></dt>
            <dd><b>{t('Prefix indicating that the file can be deleted if preventing directory removal')}</b></dd>
            <dt><code>(?i)</code></dt>
            <dd>{t('Prefix indicating that the pattern should be matched without case sensitivity')}</dd>
            <dt><code>!</code></dt>
            <dd>{t('Inversion of the given condition (i.e. do not exclude)')}</dd>
            <dt><code>*</code></dt>
            <dd>{t('Single level wildcard (matches within a directory only)')}</dd>
            <dt><code>**</code></dt>
            <dd>{t('Multi level wildcard (matches multiple directory levels)')}</dd>
            <dt><code>//</code></dt>
            <dd>{t('Comment, when used at the start of a line')}</dd>
          </dl>
          {#if !editingFolderDefaults()}
            <hr />
            <span>{t('Editing')} {folder.path}{system?.pathSeparator || '/'}.stignore.</span>
          {/if}
        {/if}
      {/if}

      <!-- Advanced Tab -->
      {#if activeTab === 'advanced'}
        <!-- Scanning section -->
        <div class="row form-group">
          <div class="col-md-12">
            <label>{$translations, t('Scanning')}</label>
            &nbsp;<a href="{utils.docsURL(null, 'users/syncing#scanning')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
            <div class="row">
              <div class="col-md-6">
                <label>
                  <input type="checkbox" bind:checked={folder.fsWatcherEnabled} onchange={setFSWatcherIntervalDefault} />&nbsp;{t('Watch for Changes')}
                </label>
                <p class="help-block">
                  {t('Use notifications from the filesystem to detect changed items.')}
                  {t('Watching for changes discovers most changes without periodic scanning.')}
                </p>
              </div>
              <div class="col-md-6">
                <label for="rescanIntervalS">{t('Full Rescan Interval (s)')}</label>
                <input id="rescanIntervalS" class="form-control" type="number" bind:value={folder.rescanIntervalS} min="0" />
                <p class="help-block" style="display:{folder.rescanIntervalS < 0 ? 'block' : 'none'}">
                  {t('The rescan interval must be a non-negative number of seconds.')}
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Folder Type + File Pull Order -->
        <div class="row">
          <div class="col-md-6 form-group">
            <label>{$translations, t('Folder Type')}</label>
            &nbsp;<a href="{utils.docsURL(null, 'users/foldertypes')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
            <select class="form-control" bind:value={folder.type}
              onchange={setDefaultsForFolderType}
              disabled={editingFolderExisting() && folder.type === 'receiveencrypted'}>
              <option value="sendreceive">{t('Send & Receive')}</option>
              <option value="sendonly">{t('Send Only')}</option>
              <option value="receiveonly">{t('Receive Only')}</option>
              <option value="receiveencrypted" disabled={editingFolderExisting()}>{t('Receive Encrypted')}</option>
            </select>
            {#if folder.type === 'sendonly'}
              <p class="help-block">{t('Files are protected from changes made on other devices, but changes made on this device will be sent to the rest of the cluster.')}</p>
            {/if}
            {#if folder.type === 'receiveonly'}
              <p class="help-block">{t('Files are synchronized from the cluster, but any changes made locally will not be sent to other devices.')}</p>
            {/if}
            {#if folder.type === 'receiveencrypted'}
              <p class="help-block">{t('Stores and syncs only encrypted data. Folders on all connected devices need to be set up with the same password or be of type "Receive Encrypted" too.')}</p>
            {/if}
            {#if editingFolderExisting() && folder.type === 'receiveencrypted'}
              <p class="help-block">{t('Folder type "Receive Encrypted" cannot be changed after adding the folder. You need to remove the folder, delete or decrypt the data on disk, and add the folder again.')}</p>
            {/if}
            {#if editingFolderExisting() && folder.type !== 'receiveencrypted'}
              <p class="help-block">{t('Folder type "Receive Encrypted" can only be set when adding a new folder.')}</p>
            {/if}
          </div>
          <div class="col-md-6 form-group">
            <label>{$translations, t('File Pull Order')}</label>
            {#if folder.type !== 'sendonly'}
              <select class="form-control" bind:value={folder.order}>
                <option value="random">{t('Random')}</option>
                <option value="alphabetic">{t('Alphabetic')}</option>
                <option value="smallestFirst">{t('Smallest First')}</option>
                <option value="largestFirst">{t('Largest First')}</option>
                <option value="oldestFirst">{t('Oldest First')}</option>
                <option value="newestFirst">{t('Newest First')}</option>
              </select>
            {:else}
              <select class="form-control" disabled>
                <option value="disabled">{t('Disabled')}</option>
              </select>
              <p class="help-block">{t('Cannot be enabled when the folder type is "{%foldertype%}".', { foldertype: t('Send Only') })}</p>
            {/if}
          </div>
        </div>

        <!-- Min Free Disk + Ignore Permissions -->
        <div class="row">
          <div class="col-md-6 form-group">
            <label for="minDiskFree">{$translations, t('Minimum Free Disk Space')}</label><br />
            {#if folder.minDiskFree !== undefined}
              <div class="row">
                <div class="col-xs-9">
                  <input id="minDiskFree" class="form-control" type="number" bind:value={folder.minDiskFree.value} min="0" step="0.01" />
                </div>
                <div class="col-xs-3">
                  <select class="form-control" bind:value={folder.minDiskFree.unit}>
                    <option value="%">%</option>
                    <option value="kB">kB</option>
                    <option value="MB">MB</option>
                    <option value="GB">GB</option>
                    <option value="TB">TB</option>
                  </select>
                </div>
              </div>
            {/if}
          </div>
          <div class="col-md-6 form-group">
            <label>
              <input type="checkbox" bind:checked={folder.ignorePerms}
                disabled={folder.type === 'receiveencrypted'} /> {t('Ignore Permissions')}
            </label>
            <p class="help-block">{t('Disables comparing and syncing file permissions. Useful on systems with nonexistent or custom permissions (e.g. FAT, exFAT, Synology, Android).')}</p>
            {#if folder.type === 'receiveencrypted'}
              <p class="help-block">{t('Always turned on when the folder type is "{%foldertype%}".', { foldertype: t('Receive Encrypted') })}</p>
            {/if}
          </div>
        </div>

        <!-- Ownership + Extended Attributes -->
        <div class="row">
          <div class="col-md-6 form-group">
            <p>
              <label>{$translations, t('Ownership')}</label>
              &nbsp;<a href="{utils.docsURL(null, 'advanced/folder-sync-ownership')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
            </p>
            <label>
              <input type="checkbox" bind:checked={folder.syncOwnership}
                disabled={folder.type === 'sendonly' || folder.type === 'receiveencrypted'} /> {t('Sync Ownership')}
            </label>
            <p class="help-block">{t('Enables sending ownership information to other devices, and applying incoming ownership information. Typically requires running with elevated privileges.')}</p>
            {#if folder.type === 'sendonly' || folder.type === 'receiveencrypted'}
              <p class="help-block">{t('Cannot be enabled when the folder type is "{%foldertype%}".', { foldertype: folder.type === 'sendonly' ? t('Send Only') : t('Receive Encrypted') })}</p>
            {/if}
            <label>
              <input type="checkbox" checked={folder.sendOwnership || folder.syncOwnership}
                onchange={(e) => { folder.sendOwnership = e.target.checked; }}
                disabled={folder.type === 'receiveonly' || folder.type === 'receiveencrypted' || folder.syncOwnership} /> {t('Send Ownership')}
            </label>
            <p class="help-block">{t('Enables sending ownership information to other devices, but not applying incoming ownership information. This can have a significant performance impact. Always enabled when "Sync Ownership" is enabled.')}</p>
            {#if folder.type === 'receiveonly' || folder.type === 'receiveencrypted'}
              <p class="help-block">{t('Cannot be enabled when the folder type is "{%foldertype%}".', { foldertype: folder.type === 'receiveonly' ? t('Receive Only') : t('Receive Encrypted') })}</p>
            {/if}
          </div>
          <div class="col-md-6 form-group">
            <p>
              <label>{$translations, t('Extended Attributes')}</label>
              &nbsp;<a href="{utils.docsURL(null, 'advanced/folder-sync-xattrs')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
            </p>
            <label>
              <input type="checkbox" bind:checked={folder.syncXattrs}
                disabled={folder.type === 'sendonly' || folder.type === 'receiveencrypted'} /> {t('Sync Extended Attributes')}
            </label>
            <p class="help-block">{t('Enables sending extended attributes to other devices, and applying incoming extended attributes. May require running with elevated privileges.')}</p>
            {#if folder.type === 'sendonly' || folder.type === 'receiveencrypted'}
              <p class="help-block">{t('Cannot be enabled when the folder type is "{%foldertype%}".', { foldertype: folder.type === 'sendonly' ? t('Send Only') : t('Receive Encrypted') })}</p>
            {/if}
            <label>
              <input type="checkbox" checked={folder.sendXattrs || folder.syncXattrs}
                onchange={(e) => { folder.sendXattrs = e.target.checked; }}
                disabled={folder.type === 'receiveonly' || folder.type === 'receiveencrypted' || folder.syncXattrs} /> {t('Send Extended Attributes')}
            </label>
            <p class="help-block">{t('Enables sending extended attributes to other devices, but not applying incoming extended attributes. This can have a significant performance impact. Always enabled when "Sync Extended Attributes" is enabled.')}</p>
            {#if folder.type === 'receiveonly' || folder.type === 'receiveencrypted'}
              <p class="help-block">{t('Cannot be enabled when the folder type is "{%foldertype%}".', { foldertype: folder.type === 'receiveonly' ? t('Receive Only') : t('Receive Encrypted') })}</p>
            {/if}
          </div>
        </div>

        <!-- Block Indexing -->
        <div class="row">
          <div class="col-md-6 form-group">
            <label>
              <input type="checkbox" bind:checked={folder.blockIndexing} /> {$translations, t('Block Indexing')}
            </label>
            <p class="help-block">{t('Maintain an index of all blocks in the folder, enabling reuse of blocks from other files when syncing changes. Disable to reduce database size at the cost of not being able to reuse blocks across files.')}</p>
          </div>
        </div>

        <!-- Extended Attributes Filter -->
        {#if folder.syncXattrs || folder.sendXattrs}
          <div class="row">
            <div class="col-md-12">
              <p>
                <label>{$translations, t('Extended Attributes Filter')}</label>
                &nbsp;<a href="{utils.docsURL(null, 'advanced/folder-xattr-filter')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
              </p>
            </div>
            <div class="col-md-6">
              <p class="help-block">{t('To permit a rule, have the checkbox checked. To deny a rule, leave it unchecked.')}</p>
              <label>{$translations, t('Active filter rules')}</label>
              {#if xattrEntries.length > 0}
                <table class="table table-condensed">
                  <colgroup>
                    <col class="col-xs-1 center"/>
                    <col class="col-xs-9"/>
                    <col class="col-xs-2"/>
                  </colgroup>
                  <tbody>
                  {#each xattrEntries as entry, idx}
                    <tr>
                      <td>
                        <input type="checkbox" bind:checked={xattrEntries[idx].permit} class="extended-attributes-filter-rule-checkbox"/>
                      </td>
                      <td><input class="form-control text-left" bind:value={xattrEntries[idx].match}/></td>
                      <td>
                        <button type="button" class="btn btn-default form-control" onclick={() => removeXattrEntry(entry)}>
                          <span class="fas fa-trash-alt"></span>
                        </button>
                      </td>
                    </tr>
                  {/each}
                  </tbody>
                </table>
              {:else}
                <p><i>{$translations, t('No rules set')}</i></p>
              {/if}
              <div class="form-group">
                <button type="button" class="btn btn-default" onclick={newXattrEntry}>
                  <span class="fas fa-plus"></span>&nbsp;{$translations, t('Add filter entry')}
                </button>
              </div>
              {#if getXattrDefault()}
                <p><i>{$translations, t('Default')}: {getXattrDefault()}</i></p>
              {/if}
              {#if getXattrHint()}
                <p><i>{getXattrHint()}</i></p>
              {/if}
            </div>
            <div class="col-md-6 form-group">
              <label for="xattrMaxSingleEntrySize">{$translations, t('Maximum single entry size')}</label>
              <input id="xattrMaxSingleEntrySize" class="form-control" type="number" bind:value={xattrMaxSingleEntrySize} min="0" />
            </div>
            <div class="col-md-6 form-group">
              <label for="xattrMaxTotalSize">{$translations, t('Maximum total size')}</label>
              <input id="xattrMaxTotalSize" class="form-control" type="number" bind:value={xattrMaxTotalSize} min="0" />
            </div>
          </div>
        {/if}
      {/if}
    </div>
  </div>

  <div class="modal-footer">
    <button type="button" class="btn btn-primary btn-sm" onclick={saveFolder}>
      <span class="fas fa-check"></span>&nbsp;{$translations, t('Save')}
    </button>
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
    {#if folder._editing === 'existing'}
      <button type="button" class="btn btn-warning pull-left btn-sm" onclick={() => actions.showRemoveFolderConfirm()}>
        <span class="fas fa-minus-circle"></span>&nbsp;{$translations, t('Remove')}
      </button>
    {/if}
  </div>
</Modal>
