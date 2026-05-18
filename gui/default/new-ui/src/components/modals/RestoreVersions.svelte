<script>
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import * as utils from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';
  import { folders } from '../../lib/stores.js';
  import { get } from 'svelte/store';

  let { folderID, onclose } = $props();

  let versions = $state(null);
  let loading = $state(true);
  let filterText = $state('');
  let selectedCount = $state(0);
  let errors = $state(null);

  import { onMount } from 'svelte';
  onMount(async () => {
    try {
      versions = await api.getFolderVersions(folderID);
    } catch (e) {
      console.error('Error loading versions:', e);
    }
    loading = false;
  });

  function folderLabel() {
    const f = get(folders);
    return f[folderID]?.label || folderID;
  }

  function filteredFiles() {
    if (!versions) return [];
    const files = Object.entries(versions);
    if (!filterText) return files;
    const lower = filterText.toLowerCase();
    return files.filter(([name]) => name.toLowerCase().includes(lower));
  }

  let selected = $state({});

  function toggleFile(name, version) {
    const key = name + '|' + version;
    if (selected[key]) {
      delete selected[key];
      selected = { ...selected };
    } else {
      selected[key] = { name, version };
      selected = { ...selected };
    }
    selectedCount = Object.keys(selected).length;
  }

  async function restoreSelected() {
    const restoreMap = {};
    for (const val of Object.values(selected)) {
      restoreMap[val.name] = [val.version];
    }
    try {
      const result = await api.postFolderVersions(folderID, restoreMap);
      if (result && Object.keys(result).length > 0) {
        errors = result;
      } else {
        onclose();
      }
    } catch (e) {
      console.error('Error restoring versions:', e);
    }
  }
</script>

<Modal title="{t('Restore Versions')} ({folderLabel()})" icon="fas fa-undo" large={true} {onclose}>
  <div class="modal-body">
    {#if loading}
      <p>{$translations, t('Loading data...')}</p>
    {:else if errors}
      <label>{$translations, t('Some items could not be restored:')}</label>
      <table class="table table-condensed table-striped">
        <tbody>
          {#each Object.entries(errors) as [file, error]}
            <tr><td>{file}</td><td>{error}</td></tr>
          {/each}
        </tbody>
      </table>
    {:else if !versions || Object.keys(versions).length === 0}
      <p>{$translations, t('There are no file versions to restore.')}</p>
    {:else}
      <div class="row form-inline">
        <div class="col-md-6">
          <div class="form-group">
            <label for="restoreVersionSearch">{$translations, t('Filter by name')}:&nbsp;</label>
            <input id="restoreVersionSearch" class="form-control" type="text" bind:value={filterText} />
          </div>
        </div>
      </div>
      <hr />
      <table class="table table-condensed table-striped">
        <thead>
          <tr>
            <th></th>
            <th>{$translations, t('Path')}</th>
            <th>{$translations, t('Version')}</th>
          </tr>
        </thead>
        <tbody>
          {#each filteredFiles() as [name, versionList]}
            {#each versionList as ver}
              <tr>
                <td><input type="checkbox" checked={!!selected[name + '|' + ver.versionTime]} onchange={() => toggleFile(name, ver.versionTime)} /></td>
                <td class="word-break-all">{name}</td>
                <td>{utils.formatDate(ver.versionTime)}</td>
              </tr>
            {/each}
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
  <div class="modal-footer">
    {#if versions && Object.keys(versions).length > 0 && !errors}
      <button type="button" class="btn btn-primary btn-sm" onclick={restoreSelected} disabled={selectedCount < 1}>
        <span class="fas fa-check"></span>&nbsp;{$translations, t('Restore')}
      </button>
    {/if}
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
  </div>
</Modal>
