<script>
  import { slide } from 'svelte/transition';
  import * as utils from '../lib/utils.js';
  import { t, translations } from '../lib/i18n.js';
  import { tooltip } from '../lib/tooltip.js';

  let { deviceCfg, connections, completion, deviceStats, discoveryCache,
    devices, folders, myID, system, metricRates, groupIdx, deviceIdx, actions } = $props();

  let expanded = $state(false);

  function status() {
    return utils.deviceStatus(deviceCfg, connections, completion, devices, myID, deviceStats, folders);
  }

  function devClass() {
    return utils.deviceClass(deviceCfg, connections, completion);
  }

  function statusIcon() {
    return utils.deviceStatusIcon(status());
  }

  function statusText() {
    return t(utils.deviceStatusText(status()));
  }

  function conn() {
    return connections[deviceCfg.deviceID] || {};
  }

  function comp() {
    return (completion[deviceCfg.deviceID] || {});
  }

  function stats() {
    return deviceStats[deviceCfg.deviceID] || {};
  }

  function connType() {
    return utils.rdConnType(deviceCfg.deviceID, connections);
  }

  function devFolders() {
    return utils.deviceFolders(deviceCfg, folders);
  }

  function compressionText() {
    switch (deviceCfg.compression) {
      case 'always': return t('All Data');
      case 'metadata': return t('Metadata Only');
      case 'never': return t('Off');
      default: return deviceCfg.compression;
    }
  }
</script>

<div class="panel panel-default">
  <button class="btn panel-heading" onclick={() => expanded = !expanded} aria-expanded={expanded}>
    {#if status() === 'syncing'}
      <div class="panel-progress" style="width: {comp()._total || 0}%"></div>
    {/if}
    <h4 class="panel-title">
      <span class="panel-icon identicon">{@html utils.generateIdenticon(deviceCfg.deviceID)}</span>
      <div class="panel-status pull-right text-{devClass()}">
        <span class="hidden-xs">{$translations, statusText()}</span>
        {#if status() === 'syncing'}
          ({utils.percentFilter(comp()._total)}, {utils.binaryFilter(comp()._needBytes)}B)
        {/if}
        <span class="inline-icon">
          <span class="visible-xs fa fa-fw {statusIcon()}" aria-label={statusText()}></span>
        </span>
        <span class="inline-icon">
          <span class="{utils.rdConnTypeIcon(connType())} reception reception-theme"></span>
        </span>
      </div>
      <div class="panel-title-text">{utils.deviceName(deviceCfg)}</div>
    </h4>
  </button>

  {#if expanded}
    <div class="panel-collapse collapse in" transition:slide={{ duration: 200 }}>
      <div class="panel-body less-padding">
        <table class="table table-condensed table-striped table-auto">
          <tbody>
            <!-- Status (mobile) -->
            <tr class="visible-xs">
              <th><span class="fa fa-fw {statusIcon()}"></span>&nbsp;{$translations, t('Device Status')}</th>
              <td class="text-right">{statusText()}</td>
            </tr>

            <!-- Last Seen (disconnected) -->
            {#if !conn().connected}
              <tr>
                <th><span class="fas fa-fw fa-eye"></span>&nbsp;{$translations, t('Last seen')}</th>
                <td class="text-right">
                  {#if !stats().lastSeenDays}
                    {t('Never')}
                  {:else}
                    <div>{utils.formatDate(stats().lastSeen)}</div>
                    {#if stats().lastSeenDays >= 7}
                      <div>
                        {#if stats().lastSeenDays < 30}
                          <i>{t('More than a week ago')}</i>
                        {:else if stats().lastSeenDays < 365}
                          <i class="text-warning">{t('More than a month ago')}</i>
                        {:else}
                          <i class="text-danger">{t('More than a year ago')}</i>
                        {/if}
                      </div>
                    {/if}
                  {/if}
                </td>
              </tr>
            {/if}

            <!-- Sync Status (disconnected + has folders) -->
            {#if !conn().connected && devFolders().length > 0}
              <tr>
                <th><span class="fas fa-fw fa-cloud"></span>&nbsp;{$translations, t('Sync Status')}</th>
                <td class="text-right">
                  {#if comp()._total === 100}
                    {t('Up to Date')}
                  {:else}
                    <span class="hidden-xs">{t('Out of Sync')}</span> ({utils.percentFilter(comp()._total)})
                  {/if}
                </td>
              </tr>
            {/if}

            <!-- Download/Upload Rate (connected) -->
            {#if conn().connected}
              <tr>
                <th><span class="fas fa-fw fa-cloud-download-alt"></span>&nbsp;{$translations, t('Download Rate')}</th>
                <td class="text-right">
                  <!-- svelte-ignore a11y_invalid_attribute -->
                  <a href="#" class="toggler" onclick={(e) => { e.preventDefault(); actions.toggleUnits(); }}>
                    {#if !metricRates}
                      {utils.binaryFilter(conn().inbps)}B/s
                    {:else}
                      {utils.metricFilter(conn().inbps * 8)}bps
                    {/if}
                    ({utils.binaryFilter(conn().inBytesTotal)}B)
                  </a>
                </td>
              </tr>
              <tr>
                <th><span class="fas fa-fw fa-cloud-upload-alt"></span>&nbsp;{$translations, t('Upload Rate')}</th>
                <td class="text-right">
                  <!-- svelte-ignore a11y_invalid_attribute -->
                  <a href="#" class="toggler" onclick={(e) => { e.preventDefault(); actions.toggleUnits(); }}>
                    {#if !metricRates}
                      {utils.binaryFilter(conn().outbps)}B/s
                    {:else}
                      {utils.metricFilter(conn().outbps * 8)}bps
                    {/if}
                    ({utils.binaryFilter(conn().outBytesTotal)}B)
                  </a>
                </td>
              </tr>
            {/if}

            <!-- Out of Sync Items -->
            {#if comp()._needItems}
              <tr>
                <th><span class="fas fa-fw fa-exchange-alt"></span>&nbsp;{$translations, t('Out of Sync Items')}</th>
                <td class="text-right">
                  <!-- svelte-ignore a11y_invalid_attribute -->
                  <a href="#" onclick={(e) => { e.preventDefault(); actions.showRemoteNeed(deviceCfg); }}>
                    {utils.localeNumber(utils.alwaysNumber(comp()._needItems))} {t('items')}, ~{utils.binaryFilter(comp()._needBytes)}B
                  </a>
                </td>
              </tr>
            {/if}

            <!-- Address -->
            <tr>
              <th><span class="fas fa-fw fa-link"></span>&nbsp;{$translations, t('Address')}</th>
              <td class="text-right">
                {#if conn().connected}
                  <span title="{conn().type} {conn().crypto || ''}">
                    {conn().address || '?'}
                  </span>
                {:else}
                  {#each (deviceCfg.addresses || []) as addr}
                    <span>{addr}</span><br/>
                    {#if system?.lastDialStatus?.[addr]?.error && !deviceCfg.paused}
                      <small class="text-danger" title={system.lastDialStatus[addr].error}>
                        {utils.abbreviatedError(addr, system)}
                      </small><br/>
                    {/if}
                  {/each}
                  {#if discoveryCache[deviceCfg.deviceID]}
                    {#each (discoveryCache[deviceCfg.deviceID].addresses || []) as addr}
                      <span title="{t('Discovered')}">{addr}</span><br/>
                    {/each}
                  {/if}
                {/if}
              </td>
            </tr>

            <!-- Connection Type (connected) -->
            {#if conn().connected}
              <tr>
                <th><span class="reception reception-4 reception-theme"></span>&nbsp;{$translations, t('Connection Type')}</th>
                <td class="text-right">{utils.rdConnTypeString(connType())}</td>
              </tr>
              <tr>
                <th><span class="fas fa-fw fa-random"></span>&nbsp;{$translations, t('Number of Connections')}</th>
                <td class="text-right">
                  {#if conn().secondary?.length}
                    1 + {conn().secondary.length}
                  {:else}
                    1
                  {/if}
                </td>
              </tr>
            {/if}

            <!-- Allowed Networks -->
            {#if deviceCfg.allowedNetworks?.length > 0}
              <tr>
                <th><span class="fas fa-fw fa-filter"></span>&nbsp;{$translations, t('Allowed Networks')}</th>
                <td class="text-right">{deviceCfg.allowedNetworks.join(', ')}</td>
              </tr>
            {/if}

            <!-- Compression -->
            <tr>
              <th><span class="fas fa-fw fa-compress"></span>&nbsp;{$translations, t('Compression')}</th>
              <td class="text-right">{compressionText()}</td>
            </tr>

            <!-- Introducer -->
            {#if deviceCfg.introducer}
              <tr>
                <th><span class="far fa-fw fa-thumbs-up"></span>&nbsp;{$translations, t('Introducer')}</th>
                <td class="text-right">{t('Yes')}</td>
              </tr>
            {/if}

            <!-- Introduced By -->
            {#if deviceCfg.introducedBy}
              <tr>
                <th><span class="far fa-fw fa-handshake-o"></span>&nbsp;{$translations, t('Introduced By')}</th>
                <td class="text-right">{utils.deviceName(devices[deviceCfg.introducedBy]) || utils.deviceShortID(deviceCfg.introducedBy)}</td>
              </tr>
            {/if}

            <!-- Auto Accept -->
            {#if deviceCfg.autoAcceptFolders}
              <tr>
                <th><span class="fa fa-fw fa-level-down"></span>&nbsp;{$translations, t('Auto Accept')}</th>
                <td class="text-right">{t('Yes')}</td>
              </tr>
            {/if}

            <!-- Identification -->
            <tr>
              <th><span class="fas fa-fw fa-qrcode"></span>&nbsp;{$translations, t('Identification')}</th>
              <td class="text-right">
                <span use:tooltip={t('Click to see full identification string and QR code.')}>
                  <!-- svelte-ignore a11y_invalid_attribute -->
                  <a href="#" onclick={(e) => { e.preventDefault(); actions.showDeviceIdentification(deviceCfg); }}>
                    {utils.deviceShortID(deviceCfg.deviceID)}
                  </a>
                </span>
              </td>
            </tr>

            <!-- Untrusted -->
            {#if deviceCfg.untrusted}
              <tr>
                <th><span class="fa fa-fw fa-user-secret"></span>&nbsp;{$translations, t('Untrusted')}</th>
                <td class="text-right">{t('Yes')}</td>
              </tr>
            {/if}

            <!-- Version (connected) -->
            {#if conn().clientVersion}
              <tr>
                <th><span class="fas fa-fw fa-tag"></span>&nbsp;{$translations, t('Version')}</th>
                <td class="text-right">{conn().clientVersion}</td>
              </tr>
            {/if}

            <!-- Folders -->
            {#if devFolders().length > 0}
              <tr>
                <th><span class="fas fa-fw fa-folder"></span>&nbsp;{$translations, t('Folders')}</th>
                <td class="text-right no-overflow-ellipse overflow-break-word">{#each devFolders() as folderID, idx}{#if folders[folderID]?.type !== 'receiveencrypted' && folders[folderID]?.devices?.some(d => d.deviceID === deviceCfg.deviceID && d.encryptionPassword)}<span class="text-nowrap"><span class="fa fa-lock"></span>&nbsp;</span>{/if}{@const remoteState = completion?.[deviceCfg.deviceID]?.[folderID]?.remoteState}{#if remoteState === 'notSharing'}<span use:tooltip={t('The remote device has not accepted sharing this folder.')}>{utils.folderLabel(folders, folderID)}<sup>1</sup></span>{:else if remoteState === 'paused'}<span use:tooltip={t('The remote device has paused this folder.')}>{utils.folderLabel(folders, folderID)}<sup>2</sup></span>{:else}{utils.folderLabel(folders, folderID)}{/if}{#if idx < devFolders().length - 1}{', '}{/if}{/each}</td>
              </tr>
            {/if}

            <!-- Remote GUI -->
            {#if deviceCfg.remoteGUIPort > 0}
              <tr>
                <th><span class="fas fa-fw fa-desktop"></span>&nbsp;{$translations, t('Remote GUI')}</th>
                <td class="text-right" title="Port {deviceCfg.remoteGUIPort}">
                  {#if utils.hasRemoteGUIAddress(deviceCfg, connections)}
                    <a href={utils.remoteGUIAddress(deviceCfg, connections)}>{utils.remoteGUIAddress(deviceCfg, connections)}</a>
                  {:else}
                    {t('Unknown')}
                  {/if}
                </td>
              </tr>
            {/if}
          </tbody>
        </table>
      </div>

      <!-- Footer buttons -->
      <div class="panel-footer">
        <span class="pull-right">
          {#if !deviceCfg.paused}
            <button type="button" class="btn btn-sm btn-default" onclick={() => actions.setDevicePause(deviceCfg.deviceID, true)}>
              <span class="fas fa-pause"></span>&nbsp;{$translations, t('Pause')}
            </button>
          {:else}
            <button type="button" class="btn btn-sm btn-default" onclick={() => actions.setDevicePause(deviceCfg.deviceID, false)}>
              <span class="fas fa-play"></span>&nbsp;{$translations, t('Resume')}
            </button>
          {/if}
          <button type="button" class="btn btn-sm btn-default" onclick={() => actions.editDevice(deviceCfg)}>
            <span class="fas fa-pencil-alt"></span>&nbsp;{$translations, t('Edit')}
          </button>
        </span>
        <div class="clearfix"></div>
      </div>
    </div>
  {/if}
</div>
