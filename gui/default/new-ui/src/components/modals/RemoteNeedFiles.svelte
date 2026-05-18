<script>
  import { onMount } from 'svelte';
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import * as utils from '../../lib/utils.js';
  import { generatePagesArray } from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';
  import { tooltip } from '../../lib/tooltip.js';

  let { device, completion, folders, devices, onclose } = $props();

  let remoteNeed = $state({});
  let needFolders = $state([]);
  let pages = $state({});
  let perpages = $state({});

  onMount(() => {
    if (!device) return;
    const flds = [];
    for (const fid in folders) {
      const f = folders[fid];
      if (f.devices) {
        for (const d of f.devices) {
          if (d.deviceID === device.deviceID) {
            const comp = completion[device.deviceID]?.[fid];
            if (comp && (comp.needItems + (comp.needDeletes || 0)) > 0) {
              flds.push(fid);
              pages[fid] = 1;
              perpages[fid] = 10;
              loadRemoteNeed(fid, 1, 10);
            }
            break;
          }
        }
      }
    }
    needFolders = flds;
  });

  async function loadRemoteNeed(folder, page = 1, perpage = 10) {
    try {
      const data = await api.getRemoteNeed(device.deviceID, folder, page, perpage);
      data.files = [...(data.progress || []), ...(data.queued || []), ...(data.rest || [])];
      remoteNeed = { ...remoteNeed, [folder]: data };
    } catch (e) {
      console.error('Error loading remote need:', e);
    }
  }

  function changePage(fid, newPage) {
    const tp = totalPages(fid);
    if (newPage < 1 || newPage > tp) return;
    pages = { ...pages, [fid]: newPage };
    loadRemoteNeed(fid, newPage, perpages[fid] || 10);
  }

  function changePerpage(fid, newPerpage) {
    perpages = { ...perpages, [fid]: newPerpage };
    pages = { ...pages, [fid]: 1 };
    loadRemoteNeed(fid, 1, newPerpage);
  }

  function totalItems(fid) {
    return completion[device.deviceID]?.[fid]?.needItems || 0;
  }

  function totalPages(fid) {
    const total = totalItems(fid);
    const pp = perpages[fid] || 10;
    if (total <= 0) return 1;
    return Math.ceil(total / pp);
  }


  function friendlyNameFromShort(shortID) {
    if (!devices || !shortID) return t('Unknown');
    for (const devID in devices) {
      if (devID.substring(0, shortID.length) === shortID) {
        return utils.deviceName(devices[devID]);
      }
    }
    return shortID;
  }
</script>

<Modal title="{t('Out of Sync Items')} - {utils.deviceName(device)}" status="info" icon="fas fa-exchange-alt" large={true} {onclose}>
  <div class="modal-body">
    {#if needFolders.length === 0}
      <p class="text-muted text-center">{$translations, t('Loading data...')}</p>
    {:else}
      {#each needFolders as fid, idx}
        <div class="panel panel-default">
          <button class="btn panel-heading" data-toggle="collapse" aria-expanded="false" onclick={(e) => {
            const target = e.currentTarget.nextElementSibling;
            if (target) target.classList.toggle('collapse');
          }}>
            <h4 class="panel-title">
              <span>{utils.folderLabel(folders, fid)}</span>
            </h4>
          </button>
          <div class:collapse={needFolders.length > 1} class="panel-collapse">
            <div class="panel-body less-padding">
              {#if !remoteNeed[fid]}
                <p><span class="fas fa-spinner fa-spin"></span> {t('Loading...')}</p>
              {:else}
                <table class="table table-striped">
                  <thead>
                    <tr>
                      <th>{$translations, t('Path')}</th>
                      <th>{$translations, t('Size')}</th>
                      <th><span use:tooltip={t('Time the item was last modified')}>{$translations, t('Mod. Time')}</span></th>
                      <th><span use:tooltip={t('Device that last modified the item')}>{$translations, t('Mod. Device')}</span></th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each (remoteNeed[fid].files || []) as file}
                      <tr>
                        <td class="word-break-all">{file.name}</td>
                        <td>{#if file.type !== 'DIRECTORY'}{utils.binaryFilter(file.size)}B{/if}</td>
                        <td>{utils.formatDate(file.modified)}</td>
                        <td>{file.modifiedBy ? friendlyNameFromShort(file.modifiedBy) : t('Unknown')}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>

                <!-- Pagination -->
                {#if totalPages(fid) > 1}
                  <ul class="pagination">
                    <li class:disabled={(pages[fid] || 1) <= 1}>
                      <!-- svelte-ignore a11y_invalid_attribute -->
                      <a href="" onclick={(e) => { e.preventDefault(); changePage(fid, (pages[fid] || 1) - 1); }}>&lsaquo;</a>
                    </li>
                    {#each generatePagesArray(pages[fid] || 1, totalItems(fid), perpages[fid] || 10) as p}
                      {#if p === '...'}
                        <li class="disabled"><a>...</a></li>
                      {:else}
                        <li class:active={(pages[fid] || 1) === p}>
                          <!-- svelte-ignore a11y_invalid_attribute -->
                          <a href="" onclick={(e) => { e.preventDefault(); changePage(fid, p); }}>{p}</a>
                        </li>
                      {/if}
                    {/each}
                    <li class:disabled={(pages[fid] || 1) >= totalPages(fid)}>
                      <!-- svelte-ignore a11y_invalid_attribute -->
                      <a href="" onclick={(e) => { e.preventDefault(); changePage(fid, (pages[fid] || 1) + 1); }}>&rsaquo;</a>
                    </li>
                  </ul>
                {/if}
                <ul class="pagination pull-right">
                  {#each [10, 25, 50] as opt}
                    <li class:active={(perpages[fid] || 10) === opt}>
                      <!-- svelte-ignore a11y_invalid_attribute -->
                      <a href="#" onclick={(e) => { e.preventDefault(); changePerpage(fid, opt); }}>{opt}</a>
                    </li>
                  {/each}
                </ul>
                <div class="clearfix"></div>
              {/if}
            </div>
          </div>
        </div>
      {/each}
    {/if}
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
  </div>
</Modal>
