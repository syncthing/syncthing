<script>
  import { onMount, onDestroy } from 'svelte';
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import * as utils from '../../lib/utils.js';
  import { generatePagesArray } from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';
  import { tooltip } from '../../lib/tooltip.js';

  let { folder, model, progress, config: cfg, folders, onclose } = $props();

  let needed = $state(null);
  let loading = $state(true);
  let page = $state(1);
  let perpage = $state(10);
  let totalItems = $state(0);

  let refreshTimer;
  onMount(() => {
    loadNeed();
    refreshTimer = setInterval(loadNeed, 10000);
  });
  onDestroy(() => {
    if (refreshTimer) clearInterval(refreshTimer);
  });

  async function loadNeed() {
    loading = true;
    try {
      const data = await api.getNeed(folder, page, perpage);
      needed = parseNeeded(data);
      totalItems = model?.[folder]?.needTotalItems || 0;
    } catch (e) {
      console.error('Error loading needed files:', e);
    }
    loading = false;
  }

  function parseNeeded(data) {
    const merged = [];
    (data.progress || []).forEach(item => {
      item.type = 'progress';
      item.action = needAction(item);
      merged.push(item);
    });
    (data.queued || []).forEach(item => {
      item.type = 'queued';
      item.action = needAction(item);
      merged.push(item);
    });
    (data.rest || []).forEach(item => {
      item.type = 'rest';
      item.action = needAction(item);
      merged.push(item);
    });
    data.items = merged;
    return data;
  }

  function needAction(file) {
    const fDelete = 4096;
    const fDirectory = 16384;
    if ((file.flags & (fDelete + fDirectory)) === fDelete + fDirectory) return 'rmdir';
    if ((file.flags & fDelete) === fDelete) return 'rm';
    if ((file.flags & fDirectory) === fDirectory) return 'touch';
    return 'sync';
  }

  function needActionText(action) {
    const map = { 'rm': t('Delete'), 'rmdir': t('Delete') + ' (dir)', 'sync': t('Sync'), 'touch': t('Update') };
    return map[action] || action;
  }

  const needIcons = {
    'rm': 'far fa-fw fa-trash-alt', 'rmdir': 'far fa-fw fa-trash-alt',
    'sync': 'far fa-fw fa-arrow-alt-circle-down', 'touch': 'fas fa-fw fa-asterisk'
  };

  async function bumpFile(file) {
    try {
      const data = await api.postBumpFile(folder, file, page, perpage);
      needed = parseNeeded(data);
    } catch (e) {
      console.error('Error bumping file:', e);
    }
  }

  function changePage(newPage) {
    if (newPage < 1 || newPage > totalPages()) return;
    page = newPage;
    loadNeed();
  }

  function changePerpage(newPerpage) {
    perpage = newPerpage;
    page = 1;
    loadNeed();
  }

  function totalPages() {
    if (totalItems <= 0) return 1;
    return Math.ceil(totalItems / perpage);
  }

  function downloadProgressEnabled() {
    // Check if the config has download progress enabled
    return cfg?.options?.progressUpdateIntervalS >= 0;
  }
</script>

<Modal title={t('Out of Sync Items')} status="info" icon="fas fa-cloud-download-alt" large={true} {onclose}>
  <div class="modal-body">
    {#if needed && needed.items && needed.items.length > 0}
      <!-- Download progress legend -->
      {#if downloadProgressEnabled()}
        <div id="download-legend">
          <div class="progress">
            <div class="progress-bar progress-bar-success" style="width: 20%"><span class="show">{$translations, t('Reused')}</span></div>
            <div class="progress-bar" style="width: 20%"><span class="show">{$translations, t('Copied from original')}</span></div>
            <div class="progress-bar progress-bar-info" style="width: 20%"><span class="show">{$translations, t('Copied from elsewhere')}</span></div>
            <div class="progress-bar progress-bar-warning" style="width: 20%"><span class="show">{$translations, t('Downloaded')}</span></div>
            <div class="progress-bar progress-bar-danger" style="width: 20%"><span class="show">{$translations, t('Downloading')}</span></div>
          </div>
          <hr />
        </div>
      {/if}

      <table class="table table-striped table-condensed">
        <tbody>
        {#each needed.items as item}
          <tr>
            <!-- Icon + Action -->
            <td class="small-data col-xs-2">
              <span class={needIcons[item.action] || ''}></span>
              {needActionText(item.action)}
            </td>

            <!-- Name -->
            <td class="small-data col-xs-6">
              {#if item.type === 'queued'}
                <!-- svelte-ignore a11y_invalid_attribute -->
                <a href="#" onclick={(e) => { e.preventDefault(); bumpFile(item.name); }} use:tooltip={t('Move to top of queue')}>
                  <span class="fas fa-eject"></span>
                </a>
                <span use:tooltip={item.name}>&nbsp;{utils.basename(item.name)}</span>
              {:else}
                <span use:tooltip={item.name}>{utils.basename(item.name)}</span>
              {/if}
            </td>

            <!-- Size / Progress -->
            <td class="col-xs-4">
              {#if item.type === 'progress' && item.action === 'sync' && progress?.[folder]?.[item.name]}
                {@const p = progress[folder][item.name]}
                <div class="progress">
                  <div class="progress-bar progress-bar-success" style="width: {utils.percentFilter(p.reused)}"></div>
                  <div class="progress-bar" style="width: {utils.percentFilter(p.copiedFromOrigin)}"></div>
                  <div class="progress-bar progress-bar-info" style="width: {utils.percentFilter(p.copiedFromElsewhere)}"></div>
                  <div class="progress-bar progress-bar-warning" style="width: {utils.percentFilter(p.pulled)}"></div>
                  <div class="progress-bar progress-bar-danger" style="width: {utils.percentFilter(p.pulling)}"></div>
                  <span class="show frontal">
                    {utils.binaryFilter(p.bytesDone)}B / {utils.binaryFilter(p.bytesTotal)}B
                  </span>
                </div>
              {:else}
                <div class="text-right small-data">
                  {#if item.size > 0}{utils.binaryFilter(item.size)}B{/if}
                </div>
              {/if}
            </td>
          </tr>
        {/each}
        </tbody>
      </table>

      <!-- Pagination -->
      {#if totalPages() > 1}
        <ul class="pagination">
          <li class:disabled={page <= 1}>
            <!-- svelte-ignore a11y_invalid_attribute -->
            <a href="" onclick={(e) => { e.preventDefault(); changePage(page - 1); }}>&lsaquo;</a>
          </li>
          {#each generatePagesArray(page, totalItems, perpage) as p}
            {#if p === '...'}
              <li class="disabled"><a>...</a></li>
            {:else}
              <li class:active={page === p}>
                <!-- svelte-ignore a11y_invalid_attribute -->
                <a href="" onclick={(e) => { e.preventDefault(); changePage(p); }}>{p}</a>
              </li>
            {/if}
          {/each}
          <li class:disabled={page >= totalPages()}>
            <!-- svelte-ignore a11y_invalid_attribute -->
            <a href="" onclick={(e) => { e.preventDefault(); changePage(page + 1); }}>&rsaquo;</a>
          </li>
        </ul>
      {/if}
      <ul class="pagination pull-right">
        {#each [10, 25, 50] as opt}
          <li class:active={perpage === opt}>
            <!-- svelte-ignore a11y_invalid_attribute -->
            <a href="#" onclick={(e) => { e.preventDefault(); changePerpage(opt); }}>{opt}</a>
          </li>
        {/each}
      </ul>
      <div class="clearfix"></div>
    {/if}
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
  </div>
</Modal>
