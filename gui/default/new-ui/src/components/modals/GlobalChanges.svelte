<script>
  import Modal from '../Modal.svelte';
  import * as utils from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';
  import { folders as foldersStore } from '../../lib/stores.js';
  import { get } from 'svelte/store';

  let { events, devices, onclose } = $props();

  function friendlyDevice(shortId) {
    if (!shortId || !devices) return t('Unknown');
    for (const devID in devices) {
      if (devID.substring(0, shortId.length) === shortId) {
        return utils.deviceName(devices[devID]);
      }
    }
    return shortId;
  }

  function actionText(action) {
    switch (action) {
      case 'modified': return t('modified');
      case 'deleted': return t('deleted');
      default: return action || '';
    }
  }

  function typeText(type) {
    switch (type) {
      case 'file': return t('file');
      case 'folder': return t('folder');
      default: return type || '';
    }
  }

  function getFolderLabel(fid) {
    const flds = get(foldersStore);
    return utils.folderLabel(flds, fid);
  }
</script>

<Modal title={t('Recent Changes')} icon="fas fa-info-circle" large={true} {onclose}>
  <div class="modal-body">
    {#if !events || events.length === 0}
      <p class="text-muted text-center">{$translations, t('No recent changes.')}</p>
    {:else}
      <div class="table-responsive">
        <table class="table-condensed table-striped table" style="table-layout: auto;">
          <thead>
            <tr>
              <th>{$translations, t('Device')}</th>
              <th>{$translations, t('Action')}</th>
              <th>{$translations, t('Type')}</th>
              <th>{$translations, t('Folder')}</th>
              <th>{$translations, t('Path')}</th>
              <th>{$translations, t('Time')}</th>
            </tr>
          </thead>
          <tbody>
            {#each events as event}
              <tr>
                <td>{event.data?.modifiedBy ? friendlyDevice(event.data.modifiedBy) : t('Unknown')}</td>
                <td>{actionText(event.data?.action)}</td>
                <td>{typeText(event.data?.type)}</td>
                <td class="no-overflow-ellipse">{getFolderLabel(event.data?.folder || event.data?.folderID || '')}</td>
                <td class="word-break-all no-overflow-ellipse">{event.data?.path || ''}</td>
                <td class="no-overflow-ellipse">{utils.formatDate(event.time)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
  </div>
</Modal>
