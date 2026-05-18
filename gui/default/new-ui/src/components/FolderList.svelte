<script>
  import FolderItem from './FolderItem.svelte';
  import * as utils from '../lib/utils.js';
  import { t, translations } from '../lib/i18n.js';

  let { foldersGrouped, folders, model, folderStats, completion, devices, myID, scanProgress, actions } = $props();

  function folderCount() {
    return Object.values(folders).length;
  }

  function isAtleastOneFolderPausedStateSetTo(pause) {
    for (const f of Object.values(folders)) {
      if (f.paused === pause) return true;
    }
    return false;
  }
</script>

<h3>
  {$translations, t('Folders')}
  {#if folderCount() > 1}
    <span>({folderCount()})</span>
  {/if}
</h3>

{#each Object.entries(foldersGrouped) as [groupName, groupedFolders], groupIdx}
  {#if groupName !== ''}
    <h4>
      {groupName}
      {#if groupedFolders.length > 1 && groupName.length > 0}
        ({groupedFolders.length})
      {/if}
    </h4>
  {/if}

  <div class="panel-group" id="folders-{groupIdx}">
    {#each groupedFolders as folder, folderIdx (folder.id)}
      <FolderItem
        {folder}
        {model}
        {folders}
        {folderStats}
        {completion}
        {devices}
        {myID}
        {scanProgress}
        {groupIdx}
        {folderIdx}
        {actions}
      />
    {/each}
  </div>
{/each}

<span class="pull-right">
  {#if isAtleastOneFolderPausedStateSetTo(false)}
    <button type="button" class="btn btn-sm btn-default" onclick={() => actions.setAllFoldersPause(true)}>
      <span class="fas fa-pause"></span>&nbsp;{$translations, t('Pause All')}
    </button>
  {/if}
  {#if isAtleastOneFolderPausedStateSetTo(true)}
    <button type="button" class="btn btn-sm btn-default" onclick={() => actions.setAllFoldersPause(false)}>
      <span class="fas fa-play"></span>&nbsp;{$translations, t('Resume All')}
    </button>
  {/if}
  {#if folderCount() > 0}
    <button type="button" class="btn btn-sm btn-default" onclick={() => actions.rescanAllFolders()}
      disabled={!isAtleastOneFolderPausedStateSetTo(false)}>
      <span class="fas fa-refresh"></span>&nbsp;{$translations, t('Rescan All')}
    </button>
  {/if}
  <button type="button" class="btn btn-sm btn-default" onclick={() => actions.addFolder()}>
    <span class="fas fa-plus"></span>&nbsp;{$translations, t('Add Folder')}
  </button>
</span>
<div class="clearfix"></div>
