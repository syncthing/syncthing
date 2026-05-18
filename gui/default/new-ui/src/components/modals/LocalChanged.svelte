<script>
  import { onMount } from 'svelte';
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import * as utils from '../../lib/utils.js';
  import { generatePagesArray } from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';

  let { folder, folderType, model, onclose } = $props();

  let localChanged = $state(null);
  let loading = $state(true);
  let page = $state(1);
  let perpage = $state(10);

  onMount(() => loadLocalChanged());

  async function loadLocalChanged() {
    loading = true;
    try {
      localChanged = await api.getLocalChanged(folder, page, perpage);
    } catch (e) {
      console.error('Error loading local changed files:', e);
    }
    loading = false;
  }

  function changePage(newPage) {
    if (newPage < 1 || newPage > totalPages()) return;
    page = newPage;
    loadLocalChanged();
  }

  function changePerpage(newPerpage) {
    perpage = newPerpage;
    page = 1;
    loadLocalChanged();
  }

  function totalItems() {
    return model?.[folder]?.receiveOnlyTotalItems || 0;
  }

  function totalPages() {
    const total = totalItems();
    if (total <= 0) return 1;
    return Math.ceil(total / perpage);
  }

  function titleText() {
    if (folderType === 'receiveencrypted') {
      return t('Unexpected Items');
    }
    return t('Locally Changed Items');
  }
</script>

<Modal title="{titleText()} ({folder})" status="info" icon="fas fa-exclamation-circle" large={true} {onclose}>
  <div class="modal-body">
    {#if localChanged && localChanged.files && localChanged.files.length > 0}
      {#if folderType === 'receiveonly'}
        <p>{t('The following items were changed locally.')}</p>
      {:else if folderType === 'receiveencrypted'}
        <p>
          {t('The following unexpected items were found.')}
          {t('You should never add or change anything locally in a "Receive Encrypted" folder.')}
        </p>
      {/if}
      <table class="table table-striped">
        <thead>
          <tr>
            <th>{$translations, t('Path')}</th>
            <th>{$translations, t('Size')}</th>
          </tr>
        </thead>
        <tbody>
          {#each localChanged.files as file}
            <tr>
              <td class="word-break-all">{file.name}</td>
              <td>{#if file.type !== 'DIRECTORY'}{utils.binaryFilter(file.size)}B{/if}</td>
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
          {#each generatePagesArray(page, totalItems(), perpage) as p}
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
