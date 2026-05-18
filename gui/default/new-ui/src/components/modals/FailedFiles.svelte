<script>
  import { onMount } from 'svelte';
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import * as utils from '../../lib/utils.js';
  import { generatePagesArray } from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';

  let { folder, model, onclose } = $props();

  let failed = $state(null);
  let loading = $state(true);
  let page = $state(1);
  let perpage = $state(10);

  onMount(() => loadFailed());

  async function loadFailed() {
    loading = true;
    try {
      failed = await api.getFolderErrors(folder, page, perpage);
    } catch (e) {
      console.error('Error loading failed files:', e);
    }
    loading = false;
  }

  function changePage(newPage) {
    if (newPage < 1 || newPage > totalPages()) return;
    page = newPage;
    loadFailed();
  }

  function changePerpage(newPerpage) {
    perpage = newPerpage;
    page = 1;
    loadFailed();
  }

  function totalItems() {
    return model?.[folder]?.pullErrors || 0;
  }

  function totalPages() {
    const total = totalItems();
    if (total <= 0) return 1;
    return Math.ceil(total / perpage);
  }

</script>

<Modal title="{t('Failed Items')} ({folder})" status="warning" icon="fas fa-exclamation-circle" large={true} {onclose}>
  <div class="modal-body">
    {#if failed && failed.errors && failed.errors.length > 0}
      <p>
        {t('The following items could not be synchronized.')}
        {t('They are retried automatically and will be synced when the error is resolved.')}
      </p>
      <table class="table table-striped table-dynamic">
        <tbody>
          {#each failed.errors as err}
            <tr>
              <td>{err.path}</td>
              <td>{err.error}</td>
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
