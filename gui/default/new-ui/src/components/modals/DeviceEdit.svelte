<script>
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import { devices as devicesStore, folders as foldersStore, config } from '../../lib/stores.js';
  import * as utils from '../../lib/utils.js';
  import { get } from 'svelte/store';
  import { t, translations } from '../../lib/i18n.js';
  import { tooltip } from '../../lib/tooltip.js';

  let { device: initialDevice, folders, myID, onclose, actions } = $props();

  let device = $state({ ...initialDevice });
  let activeTab = $state('general');
  let addressesStr = $state((initialDevice.addresses || []).join(', '));
  let dirty = $state({});

  function editingDeviceNew() {
    return device._editing === 'new' || device._editing === 'new-pending';
  }

  function editingDeviceExisting() {
    return device._editing === 'existing';
  }

  function editingDeviceDefaults() {
    return device._editing === 'defaults';
  }

  function editDeviceUntrustedChanged() {
    if (device.untrusted) {
      device.introducer = false;
      device.autoAcceptFolders = false;
    }
  }

  // Sharing state
  let sharedFolders = $state([]);
  let unrelatedFolders = $state([]);
  let selectedFolders = $state({});
  let encryptionPasswords = $state({});
  let passwordPlain = $state({});

  $effect(() => {
    initSharing();
  });

  function initSharing() {
    const shared = [];
    const selected = {};
    const encPw = {};

    if (device._editing === 'existing') {
      for (const fid in folders) {
        const f = folders[fid];
        if (f.devices) {
          for (const d of f.devices) {
            if (d.deviceID === device.deviceID) {
              shared.push(f);
              selected[fid] = true;
              if (d.encryptionPassword) {
                encPw[fid] = d.encryptionPassword;
              }
              break;
            }
          }
        }
      }
    }

    const unrelated = Object.values(folders).filter(f => !selected[f.id]);

    sharedFolders = shared.sort(utils.folderCompare);
    unrelatedFolders = unrelated.sort(utils.folderCompare);
    selectedFolders = selected;
    encryptionPasswords = encPw;
  }

  function modalTitle() {
    if (device._editing === 'defaults') return t('Edit Device Defaults');
    if (device._editing === 'existing') return t('Edit Device') + ' (' + utils.deviceName(device) + ')';
    return t('Add Device');
  }

  function selectAllSharedFolders(val) {
    sharedFolders.forEach(f => { selectedFolders[f.id] = val; });
    selectedFolders = { ...selectedFolders };
  }

  function selectAllUnrelatedFolders(val) {
    unrelatedFolders.forEach(f => { selectedFolders[f.id] = val; });
    selectedFolders = { ...selectedFolders };
  }

  async function saveDevice() {
    device.addresses = addressesStr.split(',').map(x => x.trim()).filter(x => x);

    if (device._editing === 'defaults') {
      config.update(c => {
        c.defaults.device = device;
        return { ...c };
      });
    } else {
      devicesStore.update(d => {
        d[device.deviceID] = device;
        return { ...d };
      });
      config.update(c => {
        c.devices = Object.values(get(devicesStore)).sort(utils.deviceCompare);
        return { ...c };
      });

      // Update folder sharing
      for (const fid in selectedFolders) {
        foldersStore.update(f => {
          if (!f[fid]) return f;
          if (selectedFolders[fid]) {
            const found = f[fid].devices.some(d => d.deviceID === device.deviceID);
            if (!found) {
              f[fid].devices.push({
                deviceID: device.deviceID,
                encryptionPassword: encryptionPasswords[fid] || '',
              });
            } else {
              f[fid].devices = f[fid].devices.map(d => {
                if (d.deviceID === device.deviceID) {
                  return { ...d, encryptionPassword: encryptionPasswords[fid] || '' };
                }
                return d;
              });
            }
          } else {
            f[fid].devices = f[fid].devices.filter(d => d.deviceID !== device.deviceID);
          }
          return { ...f };
        });
      }

      config.update(c => {
        c.folders = Object.values(get(foldersStore)).sort(utils.folderCompare);
        return { ...c };
      });
    }

    await actions.saveConfig();
    onclose();
  }

  async function deleteDevice() {
    if (device._editing !== 'existing') return;

    devicesStore.update(d => {
      delete d[device.deviceID];
      return { ...d };
    });

    foldersStore.update(f => {
      for (const fid in f) {
        f[fid].devices = f[fid].devices.filter(d => d.deviceID !== device.deviceID);
      }
      return { ...f };
    });

    config.update(c => {
      c.devices = Object.values(get(devicesStore)).sort(utils.deviceCompare);
      c.folders = Object.values(get(foldersStore)).sort(utils.folderCompare);
      return { ...c };
    });

    await actions.saveConfig();
    onclose();
  }
</script>

<Modal title={modalTitle()} icon={device._editing === 'existing' ? 'fas fa-pencil-alt' : 'fas fa-desktop'} large={true} {onclose}>
  <div class="modal-body">
    <!-- Tabs -->
    <ul class="nav nav-tabs">
      <li class:active={activeTab === 'general'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'general'; }}>
          <span class="fas fa-cog"></span> {$translations, t('General')}
        </a>
      </li>
      {#if !editingDeviceDefaults()}
        <li class:active={activeTab === 'sharing'}>
          <!-- svelte-ignore a11y_invalid_attribute -->
          <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'sharing'; }}>
            <span class="fas fa-share-alt"></span> {$translations, t('Sharing')}
          </a>
        </li>
      {/if}
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
        {#if !editingDeviceDefaults()}
          <div class="form-group">
            <label for="deviceID">{$translations, t('Device ID')}</label>
            <div class="input-group">
              {#if editingDeviceNew()}
                <input id="deviceID" class="form-control text-monospace" type="text" bind:value={device.deviceID} />
              {:else}
                <div class="well well-sm form-control text-monospace select-on-click" style="height: auto;">{device.deviceID}</div>
              {/if}
              <div id="shareDeviceIdButtons" class="input-group-btn">
                <button type="button" class="btn btn-default" onclick={() => utils.copyToClipboard(device.deviceID)} use:tooltip={t('Copy')}>
                  <span class="fa fa-lg fa-clone"></span>
                </button>
                <button type="button" class="btn btn-default" onclick={() => { if (actions.showDeviceIdentification) actions.showDeviceIdentification(device); }} use:tooltip={t('Show QR')}>
                  <span class="fa fa-lg fa-qrcode"></span>
                </button>
              </div>
            </div>
            {#if editingDeviceNew()}
              <p class="help-block">
                {t('The device ID to enter here can be found in the "Actions > Show ID" dialog on the other device. Spaces and dashes are optional (ignored).')}
                {t('When adding a new device, keep in mind that this device must be added on the other side too.')}
              </p>
            {/if}
          </div>
        {/if}
        <div class="form-group">
          <label for="deviceName">{$translations, t('Device Name')}</label>
          <input id="deviceName" class="form-control" type="text" bind:value={device.name} />
          {#if device.deviceID === myID}
            <p class="help-block">{t('Shown instead of Device ID in the cluster status. Will be advertised to other devices as an optional default name.')}</p>
          {:else}
            <p class="help-block">{t('Shown instead of Device ID in the cluster status. Will be updated to the name the device advertises if left empty.')}</p>
          {/if}
        </div>
        <div class="form-group">
          <label for="deviceGroup">{$translations, t('Device Group')}</label>
          <input id="deviceGroup" class="form-control" type="text" bind:value={device.group} />
          <p class="help-block">{t('Optional group for the device. Can be different on each device.')}</p>
        </div>
      {/if}

      <!-- Sharing Tab -->
      {#if activeTab === 'sharing' && !editingDeviceDefaults()}
        <div class="row">
          <div class="col-md-6">
            <div class="form-group">
              <div class="checkbox" use:tooltip={device.untrusted ? t('Always disabled for untrusted devices') : ''}>
                <label>
                  <input type="checkbox" bind:checked={device.introducer} disabled={device.untrusted} />
                  <span>{$translations, t('Introducer')}</span>
                  <p class="help-block">{t('Add devices from the introducer to our device list, for mutually shared folders.')}</p>
                </label>
              </div>
            </div>
          </div>
          <div class="col-md-6">
            <div class="form-group">
              <div class="checkbox" use:tooltip={device.untrusted ? t('Always disabled for untrusted devices') : ''}>
                <label>
                  <input type="checkbox" bind:checked={device.autoAcceptFolders} disabled={device.untrusted} />
                  <span>{$translations, t('Auto Accept')}</span>
                  <p class="help-block">{t('Automatically create or share folders that this device advertises at the default path.')}</p>
                </label>
              </div>
            </div>
          </div>
        </div>
        <div class="form-group">
          {#if sharedFolders.length > 0}
            <div class="form-horizontal">
              <label>{$translations, t('Shared Folders')}</label>
              <p class="help-block">
                {t('Deselect folders to stop sharing with this device.')}&emsp;
                <!-- svelte-ignore a11y_invalid_attribute -->
                <small><a href="#" onclick={(e) => { e.preventDefault(); selectAllSharedFolders(true); }}>{t('Select All')}</a>&emsp;
                <!-- svelte-ignore a11y_invalid_attribute -->
                <a href="#" onclick={(e) => { e.preventDefault(); selectAllSharedFolders(false); }}>{t('Deselect All')}</a></small>
              </p>
              {#each sharedFolders as f}
                <div class="form-group">
                  <div class="col-md-6 checkbox">
                    <label for="sharedwith-{f.id}">
                      <input id="sharedwith-{f.id}" type="checkbox" bind:checked={selectedFolders[f.id]} />
                      <span use:tooltip={f.id}>{f.label || f.id}</span>
                    </label>
                  </div>
                  <div class="col-md-6">
                    <div class="input-group">
                      <span class="input-group-addon">
                        {#if f.type !== 'receiveencrypted' && !encryptionPasswords[f.id]}
                          <span class="fas fa-fw fa-unlock"></span>
                        {:else}
                          <span class="fas fa-fw fa-lock"></span>
                        {/if}
                      </span>
                      {#if f.type === 'receiveencrypted'}
                        <input class="form-control input-sm" type="password" placeholder="{t('Received data is already encrypted')}" disabled />
                      {:else if selectedFolders[f.id]}
                        <input class="form-control input-sm" type="{passwordPlain[f.id] ? 'text' : 'password'}" bind:value={encryptionPasswords[f.id]} autocomplete="off" placeholder="{t('If untrusted, enter encryption password')}" />
                      {:else}
                        <input class="form-control input-sm" type="password" placeholder="{t('Not shared')}" disabled />
                      {/if}
                      <span class="input-group-addon">
                        {#if selectedFolders[f.id] && f.type !== 'receiveencrypted'}
                          <span class="button fas fa-fw {passwordPlain[f.id] ? 'fa-eye-slash' : 'fa-eye'}" onclick={() => passwordPlain[f.id] = !passwordPlain[f.id]}></span>
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
          {#if unrelatedFolders.length > 0}
            <div class="form-horizontal">
              <label>{$translations, t('Unshared Folders')}</label>
              <p class="help-block">
                {t('Select additional folders to share with this device.')}&emsp;
                <!-- svelte-ignore a11y_invalid_attribute -->
                <small><a href="#" onclick={(e) => { e.preventDefault(); selectAllUnrelatedFolders(true); }}>{t('Select All')}</a>&emsp;
                <!-- svelte-ignore a11y_invalid_attribute -->
                <a href="#" onclick={(e) => { e.preventDefault(); selectAllUnrelatedFolders(false); }}>{t('Deselect All')}</a></small>
              </p>
              {#each unrelatedFolders as f}
                <div class="form-group">
                  <div class="col-md-6 checkbox">
                    <label for="sharedwith-{f.id}">
                      <input id="sharedwith-{f.id}" type="checkbox" bind:checked={selectedFolders[f.id]} />
                      <span use:tooltip={f.id}>{f.label || f.id}</span>
                    </label>
                  </div>
                  <div class="col-md-6">
                    <div class="input-group">
                      <span class="input-group-addon">
                        {#if f.type !== 'receiveencrypted' && !encryptionPasswords[f.id]}
                          <span class="fas fa-fw fa-unlock"></span>
                        {:else}
                          <span class="fas fa-fw fa-lock"></span>
                        {/if}
                      </span>
                      {#if f.type === 'receiveencrypted'}
                        <input class="form-control input-sm" type="password" placeholder="{t('Received data is already encrypted')}" disabled />
                      {:else if selectedFolders[f.id]}
                        <input class="form-control input-sm" type="{passwordPlain[f.id] ? 'text' : 'password'}" bind:value={encryptionPasswords[f.id]} autocomplete="off" placeholder="{t('If untrusted, enter encryption password')}" />
                      {:else}
                        <input class="form-control input-sm" type="password" placeholder="{t('Not shared')}" disabled />
                      {/if}
                      <span class="input-group-addon">
                        {#if selectedFolders[f.id] && f.type !== 'receiveencrypted'}
                          <span class="button fas fa-fw {passwordPlain[f.id] ? 'fa-eye-slash' : 'fa-eye'}" onclick={() => passwordPlain[f.id] = !passwordPlain[f.id]}></span>
                        {:else}
                          <span class="button fas fa-fw fa-eye" style="opacity: 0.5"></span>
                        {/if}
                      </span>
                    </div>
                  </div>
                </div>
              {/each}
            </div>
          {:else if sharedFolders.length === 0}
            <p class="help-block">{t('There are no folders to share with this device.')}</p>
          {/if}
        </div>
      {/if}

      <!-- Advanced Tab -->
      {#if activeTab === 'advanced'}
        <div class="row form-group">
          <div class="col-md-6">
            <div class="form-group">
              <label for="addresses">{$translations, t('Addresses')}</label>
              <input id="addresses" class="form-control" type="text" bind:value={addressesStr}
                disabled={device.deviceID === myID} />
              <p class="help-block">{t('Enter comma separated ("tcp://ip:port", "tcp://host:port") addresses or "dynamic" to perform automatic discovery of the address.')}</p>
            </div>
          </div>
          <div class="col-md-6">
            <div class="form-group">
              <label>{$translations, t('Compression')}</label>
              <select class="form-control" bind:value={device.compression}>
                <option value="always">{t('All Data')}</option>
                <option value="metadata">{t('Metadata Only')}</option>
                <option value="never">{t('Off')}</option>
              </select>
            </div>
          </div>
        </div>
        <div class="row">
          <div class="col-md-6" class:has-error={dirty.numConnections && device.numConnections < 0}>
            <label>{$translations, t('Connection Management')}</label>
            <div class="row">
              <span class="col-md-8">
                {$translations, t('Number of Connections')}
                &nbsp;<a href="{utils.docsURL(null, 'advanced/device-numconnections')}" target="_blank"><span class="fas fa-question-circle"></span>&nbsp;{t('Help')}</a>
              </span>
              <div class="col-md-4">
                <input id="numConnections" class="form-control" type="number" bind:value={device.numConnections} min="0" oninput={() => dirty.numConnections = true} />
              </div>
            </div>
            <p class="help-block">
              {#if dirty.numConnections && device.numConnections < 0}
                {t('The number of connections must be a non-negative number.')}
              {:else}
                {t('When set to more than one on both devices, Syncthing will attempt to establish multiple concurrent connections. If the values differ, the highest will be used. Set to zero to let Syncthing decide.')}
              {/if}
            </p>
          </div>
          <div class="col-md-6 form-group">
            <label>{$translations, t('Device rate limits')}</label>
            <div class="row">
              <div class="col-md-12" class:has-error={dirty.maxRecvKbps && device.maxRecvKbps < 0}>
                <div class="row">
                  <span class="col-md-8">{$translations, t('Incoming Rate Limit (KiB/s)')}</span>
                  <div class="col-md-4">
                    <input id="maxRecvKbps" class="form-control" type="number" bind:value={device.maxRecvKbps} min="0" step="1024" oninput={() => dirty.maxRecvKbps = true} />
                  </div>
                </div>
                {#if dirty.maxRecvKbps && device.maxRecvKbps < 0}
                  <p class="help-block">{t('The rate limit must be a non-negative number (0: no limit)')}</p>
                {/if}
              </div>
              <div class="col-md-12" class:has-error={dirty.maxSendKbps && device.maxSendKbps < 0}>
                <div class="row">
                  <span class="col-md-8">{$translations, t('Outgoing Rate Limit (KiB/s)')}</span>
                  <div class="col-md-4">
                    <input id="maxSendKbps" class="form-control" type="number" bind:value={device.maxSendKbps} min="0" step="1024" oninput={() => dirty.maxSendKbps = true} />
                  </div>
                </div>
                {#if dirty.maxSendKbps && device.maxSendKbps < 0}
                  <p class="help-block">{t('The rate limit must be a non-negative number (0: no limit)')}</p>
                {:else}
                  <p class="help-block">{t('The rate limit is applied to the accumulated traffic of all connections to this device.')}</p>
                {/if}
              </div>
            </div>
          </div>
        </div>
        <div class="row">
          <div class="form-group col-md-6">
            <input type="checkbox" id="untrusted" bind:checked={device.untrusted} onchange={editDeviceUntrustedChanged} />
            <label for="untrusted">{$translations, t('Untrusted')}</label>
            <p class="help-block">{t('All folders shared with this device must be protected by a password, such that all sent data is unreadable without the given password.')}</p>
          </div>
        </div>
      {/if}
    </div>
  </div>

  <div class="modal-footer">
    <button type="button" class="btn btn-primary btn-sm" onclick={saveDevice}>
      <span class="fas fa-check"></span>&nbsp;{$translations, t('Save')}
    </button>
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
    {#if editingDeviceExisting()}
      <div class="pull-left">
        <button type="button" class="btn btn-warning btn-sm" onclick={() => actions.showRemoveDeviceConfirm()}>
          <span class="fas fa-minus-circle"></span>&nbsp;{$translations, t('Remove')}
        </button>
      </div>
    {/if}
  </div>
</Modal>
