<script>
  import { slide } from 'svelte/transition';
  import * as utils from '../lib/utils.js';
  import { t, translations } from '../lib/i18n.js';
  import { tooltip } from '../lib/tooltip.js';

  let { folder, model, folders, folderStats, completion, devices, myID, scanProgress, groupIdx, folderIdx, actions } = $props();

  let expanded = $state(false);

  function status() {
    return utils.folderStatus(folder, model, folders);
  }

  function statusClass() {
    return utils.folderClass(folder, model, folders);
  }

  function statusIcon() {
    return utils.folderStatusIcon(folder, model, folders);
  }

  function statusText() {
    return t(utils.folderStatusText(folder, model, folders));
  }

  function syncPct() {
    return utils.syncPercentage(folder.id, model);
  }

  function scanPct() {
    return utils.scanPercentage(folder.id, scanProgress);
  }

  function folderTypeIcon() {
    switch (folder.type) {
      case 'sendreceive': return 'fas fa-fw fa-folder';
      case 'sendonly': return 'fas fa-fw fa-upload';
      case 'receiveonly': return 'fas fa-fw fa-download';
      case 'receiveencrypted': return 'fas fa-fw fa-lock';
      default: return 'fas fa-fw fa-folder';
    }
  }

  function folderTypeText() {
    switch (folder.type) {
      case 'sendreceive': return t('Send & Receive');
      case 'sendonly': return t('Send Only');
      case 'receiveonly': return t('Receive Only');
      case 'receiveencrypted': return t('Receive Encrypted');
      default: return folder.type;
    }
  }

  function displayLabel() {
    return (folder.label && folder.label.length > 0) ? folder.label : folder.id;
  }

  function otherDevices() {
    if (!folder.devices) return [];
    return folder.devices.filter(d => d.deviceID !== myID);
  }

  function canRescan() {
    const st = status();
    return ['idle', 'stopped', 'unshared', 'outofsync', 'faileditems', 'localadditions'].includes(st);
  }

  function versioningTypeText() {
    if (!folder.versioning || !folder.versioning.type) return '';
    const map = { 'trashcan': t('Trash Can'), 'simple': t('Simple'), 'staggered': t('Staggered'), 'external': t('External') };
    return map[folder.versioning.type] || folder.versioning.type;
  }

  function orderText() {
    const map = {
      'random': t('Random'), 'alphabetic': t('Alphabetic'),
      'smallestFirst': t('Smallest First'), 'largestFirst': t('Largest First'),
      'oldestFirst': t('Oldest First'), 'newestFirst': t('Newest First'),
    };
    return map[folder.order] || folder.order;
  }

  function fm() { return model[folder.id] || {}; }
  function fs() { return folderStats[folder.id] || {}; }
</script>

<div class="panel panel-default">
  <button class="btn panel-heading" onclick={() => expanded = !expanded} aria-expanded={expanded}>
    {#if status() === 'syncing'}
      <div class="panel-progress" style="width: {syncPct()}%"></div>
    {/if}
    {#if status() === 'scanning' && scanPct() !== undefined}
      <div class="panel-progress" style="width: {scanPct()}%"></div>
    {/if}
    <h4 class="panel-title">
      <div class="panel-icon hidden-xs">
        <span class={folderTypeIcon()}></span>
      </div>
      <div class="panel-status pull-right text-{statusClass()}">
        <span class="hidden-xs">{$translations, statusText()}</span>
        {#if status() === 'scanning' && scanPct() !== undefined}
          ({utils.percentFilter(scanPct())})
        {/if}
        {#if status() === 'syncing'}
          ({utils.percentFilter(syncPct())}, {utils.binaryFilter(fm().needBytes)}B)
        {/if}
        <span class="inline-icon">
          <span class="visible-xs fa fa-fw {statusIcon()}" aria-label={statusText()}></span>
        </span>
      </div>
      <div class="panel-title-text">
        <span use:tooltip={folder.label && folder.label.length > 0 ? folder.id : ''}>{displayLabel()}</span>
      </div>
    </h4>
  </button>

  {#if expanded}
    <div class="panel-collapse collapse in" transition:slide={{ duration: 200 }}>
      <div class="panel-body less-padding">
        <table class="table table-condensed table-striped table-auto">
          <tbody>
            <!-- Folder Status (visible on mobile) -->
            <tr class="visible-xs">
              <th><span class="fa fa-fw {statusIcon()}"></span>&nbsp;{$translations, t('Folder Status')}</th>
              <td class="text-right">{statusText()}</td>
            </tr>

            <!-- Folder ID (shown when label exists) -->
            {#if folder.label && folder.label.length > 0}
              <tr>
                <th><span class="fas fa-fw fa-info-circle"></span>&nbsp;{$translations, t('Folder ID')}</th>
                <td class="text-right no-overflow-ellipse">{folder.id}</td>
              </tr>
            {/if}

            <!-- Folder Path -->
            <tr>
              <th><span class="fas fa-fw fa-folder-open"></span>&nbsp;{$translations, t('Folder Path')}</th>
              <td class="text-right"><span use:tooltip={folder.path}>{folder.path}</span></td>
            </tr>

            <!-- Error -->
            {#if !folder.paused && (fm().invalid || fm().error)}
              <tr>
                <th><span class="fas fa-fw fa-exclamation-triangle"></span>&nbsp;{$translations, t('Error')}</th>
                <td class="text-right" title={fm().invalid || fm().error}>{fm().invalid || fm().error}</td>
              </tr>
            {/if}

            <!-- Global State -->
            {#if !folder.paused}
              <tr>
                <th><span class="fas fa-fw fa-globe"></span>&nbsp;{$translations, t('Global State')}</th>
                <td class="text-right">
                  <span use:tooltip={utils.localeNumber(utils.alwaysNumber(fm().globalFiles)) + ' ' + t('files') + ', ' + utils.localeNumber(utils.alwaysNumber(fm().globalDirectories)) + ' ' + t('directories') + ', ~' + utils.binaryFilter(fm().globalBytes) + 'B'}>
                    <span class="far fa-copy"></span>&nbsp;{utils.localeNumber(utils.alwaysNumber(fm().globalFiles))}&ensp;
                    <span class="far fa-folder"></span>&nbsp;{utils.localeNumber(utils.alwaysNumber(fm().globalDirectories))}&ensp;
                    <span class="far fa-hdd"></span>&nbsp;~{utils.binaryFilter(fm().globalBytes)}B
                  </span>
                </td>
              </tr>

              <!-- Local State -->
              <tr>
                <th><span class="fas fa-fw fa-home"></span>&nbsp;{$translations, t('Local State')}</th>
                <td class="text-right">
                  <span use:tooltip={utils.localeNumber(utils.alwaysNumber(fm().localFiles)) + ' ' + t('files') + ', ' + utils.localeNumber(utils.alwaysNumber(fm().localDirectories)) + ' ' + t('directories') + ', ~' + utils.binaryFilter(fm().localBytes) + 'B'}>
                    <span class="far fa-copy"></span>&nbsp;{utils.localeNumber(utils.alwaysNumber(fm().localFiles))}&ensp;
                    <span class="far fa-folder"></span>&nbsp;{utils.localeNumber(utils.alwaysNumber(fm().localDirectories))}&ensp;
                    <span class="far fa-hdd"></span>&nbsp;~{utils.binaryFilter(fm().localBytes)}B
                  </span>
                  {#if fm().ignorePatterns}
                    <div>
                      <!-- svelte-ignore a11y_invalid_attribute -->
                      <a href="#" onclick={(e) => { e.preventDefault(); actions.editFolder(folder, '#folder-ignores'); }}><i class="small">{$translations, t('Reduced by ignore patterns')}</i></a>
                    </div>
                  {/if}
                  {#if folder.ignoreDelete}
                    <div>
                      <i class="small">
                        {t('Altered by ignoring deletes.')}
                        <a href="{utils.docsURL(null, 'advanced/folder-ignoredelete')}" target="_blank">
                          <span class="fas fa-question-circle"></span>&nbsp;{t('Help')}
                        </a>
                      </i>
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}

            <!-- Out of Sync Items -->
            {#if fm().needTotalItems > 0}
              <tr>
                <th><span class="fas fa-fw fa-cloud-download-alt"></span>&nbsp;{$translations, t('Out of Sync Items')}</th>
                <td class="text-right">
                  <!-- svelte-ignore a11y_invalid_attribute -->
                  <a href="#" onclick={(e) => { e.preventDefault(); actions.showNeed(folder.id); }}>
                    {utils.localeNumber(utils.alwaysNumber(fm().needTotalItems))} {t('items')}, ~{utils.binaryFilter(fm().needBytes)}B
                  </a>
                </td>
              </tr>
            {/if}

            <!-- Scan Time Remaining -->
            {#if status() === 'scanning' && utils.scanRate(folder.id, scanProgress) > 0}
              <tr>
                <th><span class="fas fa-fw fa-hourglass-half"></span>&nbsp;{$translations, t('Scan Time Remaining')}</th>
                <td class="text-right" title="{utils.binaryFilter(utils.scanRate(folder.id, scanProgress))}B/s">
                  ~ {utils.scanRemaining(folder.id, scanProgress)}
                </td>
              </tr>
            {/if}

            <!-- Failed Items -->
            {#if utils.hasFailedFiles(folder.id, model)}
              <tr>
                <th><span class="fas fa-fw fa-exclamation-circle"></span>&nbsp;{$translations, t('Failed Items')}</th>
                <td class="text-right">
                  <!-- svelte-ignore a11y_invalid_attribute -->
                  <a href="#" onclick={(e) => { e.preventDefault(); actions.showFailed(folder.id); }}>
                    {utils.localeNumber(utils.alwaysNumber(fm().pullErrors))}&nbsp;{t('items')}
                  </a>
                </td>
              </tr>
            {/if}

            <!-- Locally Changed Items -->
            {#if utils.hasReceiveOnlyChanged(folder, model)}
              <tr>
                <th><span class="fas fa-fw fa-exclamation-circle"></span>&nbsp;{$translations, t('Locally Changed Items')}</th>
                <td class="text-right">
                  <!-- svelte-ignore a11y_invalid_attribute -->
                  <a href="#" onclick={(e) => { e.preventDefault(); actions.showLocalChanged(folder.id, folder.type); }}>
                    {utils.localeNumber(utils.alwaysNumber(fm().receiveOnlyTotalItems))} {t('items')}, ~{utils.binaryFilter(fm().receiveOnlyChangedBytes)}B
                  </a>
                </td>
              </tr>
            {/if}

            <!-- Folder Type -->
            <tr>
              <th><span class="fas fa-fw fa-folder"></span>&nbsp;{$translations, t('Folder Type')}</th>
              <td class="text-right">{folderTypeText()}</td>
            </tr>

            <!-- Block Indexing -->
            <tr>
              <th><span class="far fa-fw fa-book"></span>&nbsp;{$translations, t('Block Indexing')}</th>
              <td class="text-right">{folder.blockIndexing ? t('Yes') : t('No')}</td>
            </tr>

            <!-- Ignore Permissions -->
            {#if folder.ignorePerms}
              <tr>
                <th><span class="far fa-fw fa-minus-square"></span>&nbsp;{$translations, t('Ignore Permissions')}</th>
                <td class="text-right">{t('Yes')}</td>
              </tr>
            {/if}

            <!-- Rescans -->
            <tr>
              <th><span class="fas fa-fw fa-refresh"></span>&nbsp;{$translations, t('Rescans')}</th>
              <td class="text-right">
                {#if folder.rescanIntervalS > 0}
                  {#if !folder.fsWatcherEnabled}
                    <span use:tooltip={t('Periodic scanning at given interval and disabled watching for changes')}>
                      <span class="far fa-clock"></span>&nbsp;{utils.durationFilter(folder.rescanIntervalS)}&ensp;
                      <span class="fas fa-eye-slash"></span>&nbsp;{t('Disabled')}
                    </span>
                  {:else if !fm().watchError || folder.paused || status() === 'stopped'}
                    <span use:tooltip={t('Periodic scanning at given interval and enabled watching for changes')}>
                      <span class="far fa-clock"></span>&nbsp;{utils.durationFilter(folder.rescanIntervalS)}&ensp;
                      <span class="fas fa-eye"></span>&nbsp;{t('Enabled')}
                    </span>
                  {:else}
                    <span use:tooltip={t('Periodic scanning at given interval and failed setting up watching for changes, retrying every 1m:') + '\n' + (fm().watchError || '')}>
                      <span class="far fa-clock"></span>&nbsp;{utils.durationFilter(folder.rescanIntervalS)}&ensp;
                      <span class="fas fa-eye-slash"></span>&nbsp;{t('Failed to set up, retrying')}
                    </span>
                  {/if}
                {:else}
                  {#if !folder.fsWatcherEnabled}
                    <span use:tooltip={t('Disabled periodic scanning and disabled watching for changes')}>
                      <span class="far fa-clock"></span>&nbsp;{t('Disabled')}&ensp;
                      <span class="fas fa-eye-slash"></span>&nbsp;{t('Disabled')}
                    </span>
                  {:else if !fm().watchError || folder.paused || status() === 'stopped'}
                    <span use:tooltip={t('Disabled periodic scanning and enabled watching for changes')}>
                      <span class="far fa-clock"></span>&nbsp;{t('Disabled')}&ensp;
                      <span class="fas fa-eye"></span>&nbsp;{t('Enabled')}
                    </span>
                  {:else}
                    <span use:tooltip={t('Disabled periodic scanning and failed setting up watching for changes, retrying every 1m:') + '\n' + (fm().watchError || '')}>
                      <span class="far fa-clock"></span>&nbsp;{t('Disabled')}&ensp;
                      <span class="fas fa-eye-slash"></span>&nbsp;{t('Failed to set up, retrying')}
                    </span>
                  {/if}
                {/if}
              </td>
            </tr>

            <!-- File Pull Order -->
            {#if folder.type !== 'sendonly'}
              <tr>
                <th><span class="fas fa-fw fa-sort"></span>&nbsp;{$translations, t('File Pull Order')}</th>
                <td class="text-right">{orderText()}</td>
              </tr>
            {/if}

            <!-- File Versioning -->
            {#if folder.versioning?.type}
              <tr>
                <th><span class="fa fa-fw fa-files-o"></span>&nbsp;{$translations, t('File Versioning')}</th>
                <td class="text-right">
                  {#if folder.versioning.type === 'external'}
                    <span use:tooltip={folder.versioning.params?.command || ''}>{versioningTypeText()}</span>
                  {:else}
                    {versioningTypeText()}
                  {/if}
                  {#if folder.versioning.type !== 'external'}
                    {#if (folder.versioning.type === 'trashcan' || folder.versioning.type === 'simple')}
                      <span use:tooltip={t('Clean out after')}>
                        &ensp;<span class="fa fa-calendar"></span>&nbsp;{#if folder.versioning.params?.cleanoutDays === '0' || folder.versioning.params?.cleanoutDays === 0}{t('Disabled')}{:else}{utils.durationFilter((folder.versioning.params?.cleanoutDays || 0) * 86400, 'd')}{/if}
                      </span>
                    {/if}
                    {#if folder.versioning.type === 'simple'}
                      <span use:tooltip={t('Keep Versions')}>
                        &ensp;<span class="fa fa-file-archive-o"></span>&nbsp;{folder.versioning.params?.keep || ''}
                      </span>
                    {/if}
                    {#if folder.versioning.type === 'staggered'}
                      <span use:tooltip={t('Maximum Age')}>
                        &ensp;<span class="fa fa-calendar"></span>&nbsp;{#if folder.versioning.params?.maxAge === '0' || folder.versioning.params?.maxAge === 0}{t('Forever')}{:else}{utils.durationFilter(folder.versioning.params?.maxAge || 0)}{/if}
                      </span>
                    {/if}
                    <span use:tooltip={t('Cleanup Interval')}>
                      &ensp;<span class="fa fa-recycle"></span>&nbsp;{#if !folder.versioning.cleanupIntervalS}{t('Disabled')}{:else}{utils.durationFilter(folder.versioning.cleanupIntervalS)}{/if}
                    </span>
                    <span use:tooltip={folder.versioning.fsPath === '' ? '.stversions' : folder.versioning.fsPath}>
                      &ensp;<span class="fa fa-folder-open-o"></span>&nbsp;{folder.versioning.fsPath === '' ? '.stversions' : folder.versioning.fsPath}
                    </span>
                  {/if}
                </td>
              </tr>
            {/if}

            <!-- Shared With -->
            <tr>
              <th><span class="fas fa-fw fa-share-alt"></span>&nbsp;{$translations, t('Shared With')}</th>
              <td class="text-right no-overflow-ellipse overflow-break-word">{#each otherDevices() as dev, idx}{#if folder.type !== 'receiveencrypted' && dev.encryptionPassword}<span class="text-nowrap"><span class="fa fa-lock"></span>&nbsp;</span>{/if}{@const remoteState = completion?.[dev.deviceID]?.[folder.id]?.remoteState}{#if remoteState === 'notSharing'}<a href="" onclick={(e) => { e.preventDefault(); actions.editDevice(devices[dev.deviceID]); }} use:tooltip={t('The remote device has not accepted sharing this folder.')}>{utils.deviceName(devices[dev.deviceID])}<sup>1</sup></a>{:else if remoteState === 'paused'}<a href="" onclick={(e) => { e.preventDefault(); actions.editDevice(devices[dev.deviceID]); }} use:tooltip={t('The remote device has paused this folder.')}>{utils.deviceName(devices[dev.deviceID])}<sup>2</sup></a>{:else}<a href="" onclick={(e) => { e.preventDefault(); actions.editDevice(devices[dev.deviceID]); }}>{utils.deviceName(devices[dev.deviceID])}</a>{/if}{#if idx < otherDevices().length - 1}{', '}{/if}{/each}</td>
            </tr>

            <!-- Last Scan -->
            {#if fs().lastScan}
              <tr>
                <th><span class="far fa-fw fa-clock"></span>&nbsp;{$translations, t('Last Scan')}</th>
                <td class="text-right">
                  {#if fs().lastScanDays >= 365}
                    {t('Never')}
                  {:else}
                    {utils.formatDate(fs().lastScan)}
                  {/if}
                </td>
              </tr>
            {/if}

            <!-- Latest Change -->
            {#if folder.type !== 'sendonly' && folder.type !== 'receiveencrypted' && fs().lastFile?.filename}
              <tr>
                <th><span class="fas fa-fw fa-exchange-alt"></span>&nbsp;{$translations, t('Latest Change')}</th>
                <td class="text-right" title="{fs().lastFile.filename} @ {utils.formatDate(fs().lastFile.at)}">
                  {fs().lastFile.deleted ? t('Deleted {%file%}', { file: utils.basename(fs().lastFile.filename) }) : t('Updated') + ' ' + utils.basename(fs().lastFile.filename)}
                </td>
              </tr>
            {/if}
          </tbody>
        </table>
      </div>

      <!-- Footer buttons -->
      <div class="panel-footer">
        {#if status() === 'outofsync' && folder.type === 'sendonly'}
          <button type="button" class="btn btn-sm btn-danger pull-left" onclick={() => actions.revertOverride('override', folder.id)}>
            <span class="fas fa-arrow-circle-up"></span>&nbsp;{$translations, t('Override Changes')}
          </button>
        {/if}
        {#if utils.hasReceiveOnlyChanged(folder, model) && ['outofsync', 'faileditems', 'localadditions'].includes(status())}
          <button type="button" class="btn btn-sm btn-danger pull-left" onclick={() => actions.revertOverride('revert', folder.id)}>
            <span class="fa fa-arrow-circle-down"></span>&nbsp;{$translations, t('Revert Local Changes')}
          </button>
        {/if}
        {#if utils.hasReceiveEncryptedItems(folder, model) && ['outofsync', 'faileditems', 'localunencrypted'].includes(status())}
          <button type="button" class="btn btn-sm btn-danger pull-left" onclick={() => actions.revertOverride('deleteEnc', folder.id)}>
            <span class="fa fa-minus-circle"></span>&nbsp;{$translations, t('Delete Unexpected Items')}
          </button>
        {/if}

        <span class="pull-right">
          {#if !folder.paused}
            <button type="button" class="btn btn-sm btn-default" onclick={() => actions.setFolderPause(folder.id, true)}>
              <span class="fas fa-pause"></span>&nbsp;{$translations, t('Pause')}
            </button>
          {:else}
            <button type="button" class="btn btn-sm btn-default" onclick={() => actions.setFolderPause(folder.id, false)}>
              <span class="fas fa-play"></span>&nbsp;{$translations, t('Resume')}
            </button>
          {/if}
          {#if folder.versioning?.type && folder.versioning.type !== 'external'}
            <button type="button" class="btn btn-default btn-sm" disabled={folder.paused}
              onclick={() => { if (actions.showRestoreVersions) actions.showRestoreVersions(folder.id); }}>
              <span class="fas fa-undo"></span>&nbsp;{$translations, t('Versions')}
            </button>
          {/if}
          <button type="button" class="btn btn-sm btn-default" onclick={() => actions.rescanFolder(folder.id)} disabled={!canRescan()}>
            <span class="fas fa-refresh"></span>&nbsp;{$translations, t('Rescan')}
          </button>
          <button type="button" class="btn btn-sm btn-default" onclick={() => actions.editFolder(folder)}>
            <span class="fas fa-pencil-alt"></span>&nbsp;{$translations, t('Edit')}
          </button>
        </span>
        <div class="clearfix"></div>
      </div>
    </div>
  {/if}
</div>
