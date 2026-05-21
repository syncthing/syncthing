<script>
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import * as utils from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';
  import { folders } from '../../lib/stores.js';
  import { get } from 'svelte/store';
  import { onMount } from 'svelte';

  let { folderID, onclose } = $props();

  let versions = $state(null);
  let loading = $state(true);
  let errors = $state(null);

  // Selections: { filePath: versionTime (Date), ... }
  let selections = $state({});

  // Filters
  let filterText = $state('');
  let filterStart = $state(null);
  let filterEnd = $state(null);
  let minDate = $state(null);
  let maxDate = $state(null);

  // Tree
  let tree = $state([]);
  let expanded = $state({});

  // Confirmation dialog
  let showConfirmation = $state(false);

  // Date range dropdown
  let showDateDropdown = $state(false);

  onMount(async () => {
    try {
      const data = await api.getFolderVersions(folderID);
      if (data) {
        // Parse dates and sort versions (newest first)
        for (const key of Object.keys(data)) {
          for (const ver of data[key]) {
            ver.modTime = new Date(ver.modTime);
            ver.versionTime = new Date(ver.versionTime);
          }
          data[key].sort((a, b) => b.versionTime - a.versionTime);
        }
        versions = data;

        // Build tree
        tree = buildTree(versions);

        // Find date window
        let mn = new Date();
        let mx = new Date(0);
        for (const key of Object.keys(versions)) {
          for (const ver of versions[key]) {
            if (ver.versionTime < mn) mn = ver.versionTime;
            if (ver.versionTime > mx) mx = ver.versionTime;
          }
        }
        minDate = mn;
        maxDate = mx;
        filterStart = mn;
        filterEnd = mx;

        // Expand all top-level folders
        for (const node of tree) {
          if (node.folder) {
            expanded[node.key] = true;
          }
        }
        expanded = { ...expanded };
      }
    } catch (e) {
      console.error('Error loading versions:', e);
    }
    loading = false;
  });

  function folderLabel() {
    const f = get(folders);
    return f[folderID]?.label || folderID;
  }

  // Build a tree structure from flat file paths (exact port from Angular's buildTree)
  function buildTree(children) {
    const root = { children: [] };
    for (const [path, data] of Object.entries(children)) {
      const parts = path.split('/');
      const name = parts.pop();
      let keySoFar = [];
      let parent = root;
      for (const part of parts) {
        keySoFar.push(part);
        let found = null;
        for (const child of parent.children) {
          if (child.title === part && child.folder) {
            found = child;
            break;
          }
        }
        if (!found) {
          found = { title: part, key: keySoFar.join('/'), folder: true, children: [] };
          parent.children.push(found);
        }
        parent = found;
      }
      parent.children.push({ title: name, key: path, folder: false, versions: data });
    }
    // Sort: folders first, then alphabetically case-insensitive
    sortTree(root.children);
    return root.children;
  }

  function sortTree(nodes) {
    nodes.sort((a, b) => {
      const ax = (a.folder ? '0' : '1') + a.title.toLowerCase();
      const bx = (b.folder ? '0' : '1') + b.title.toLowerCase();
      return ax < bx ? -1 : ax > bx ? 1 : 0;
    });
    for (const node of nodes) {
      if (node.folder && node.children) sortTree(node.children);
    }
  }

  // Filter versions by date range
  function filterVersions(vers) {
    if (!filterStart || !filterEnd) return vers;
    return vers.filter(v => v.versionTime >= filterStart && v.versionTime <= filterEnd);
  }

  // Check if a node should be visible (matches text filter + has versions in date range)
  function nodeVisible(node) {
    if (node.folder) {
      return node.children.some(c => nodeVisible(c));
    }
    // Text filter
    if (filterText) {
      const ft = filterText.toLowerCase().replace(/\\/g, '/');
      const vp = node.key.toLowerCase().replace(/\\/g, '/');
      if (!vp.includes(ft)) return false;
    }
    // Date filter
    if (filterVersions(node.versions).length === 0) return false;
    return true;
  }

  // Selection count
  function selectionCount() {
    let count = 0;
    for (const v of Object.values(selections)) {
      if (v) count++;
    }
    return count;
  }

  // Mass actions
  function massAction(folderKey, action) {
    for (const [key, vers] of Object.entries(versions)) {
      if (!key.startsWith(folderKey + '/')) continue;
      // Check text filter
      if (filterText) {
        const ft = filterText.toLowerCase().replace(/\\/g, '/');
        if (!key.toLowerCase().replace(/\\/g, '/').includes(ft)) continue;
      }
      if (action === 'unset') {
        delete selections[key];
        continue;
      }
      const available = filterVersions(vers).map(v => v.versionTime).sort((a, b) => a - b);
      if (available.length) {
        if (action === 'latest') {
          selections[key] = available[available.length - 1];
        } else if (action === 'oldest') {
          selections[key] = available[0];
        }
      }
    }
    selections = { ...selections };
  }

  // Restore
  async function doRestore() {
    showConfirmation = false;
    const sel = {};
    for (const [key, value] of Object.entries(selections)) {
      if (value) sel[key] = value;
    }
    versions = null;
    selections = {};
    try {
      const result = await api.postFolderVersions(folderID, sel);
      if (result && Object.keys(result).length > 0) {
        errors = result;
      } else {
        onclose();
      }
    } catch (e) {
      console.error('Error restoring versions:', e);
    }
  }

  // Date range presets
  function dateRanges() {
    if (!minDate || !maxDate) return [];
    const now = new Date();
    const startOfDay = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const yesterday = new Date(startOfDay);
    yesterday.setDate(yesterday.getDate() - 1);
    const last7 = new Date(startOfDay);
    last7.setDate(last7.getDate() - 6);
    const last30 = new Date(startOfDay);
    last30.setDate(last30.getDate() - 29);
    const startOfMonth = new Date(now.getFullYear(), now.getMonth(), 1);
    const startOfLastMonth = new Date(now.getFullYear(), now.getMonth() - 1, 1);
    const endOfLastMonth = new Date(now.getFullYear(), now.getMonth(), 0, 23, 59, 59);

    const ranges = [];
    const tryAdd = (label, start, end) => {
      // Include range if it overlaps with available data
      if (start <= maxDate && end >= minDate) {
        ranges.push({ label, start, end });
      }
    };
    tryAdd(t('All Time'), minDate, maxDate);
    tryAdd(t('Today'), startOfDay, now);
    tryAdd(t('Yesterday'), yesterday, new Date(startOfDay.getTime() - 1));
    tryAdd(t('Last 7 Days'), last7, now);
    tryAdd(t('Last 30 Days'), last30, now);
    tryAdd(t('This Month'), startOfMonth, now);
    tryAdd(t('Last Month'), startOfLastMonth, endOfLastMonth);
    return ranges;
  }

  function applyDateRange(start, end) {
    filterStart = start;
    filterEnd = end;
    showDateDropdown = false;
  }

  function formatDateForInput(d) {
    if (!d) return '';
    const pad = n => String(n).padStart(2, '0');
    return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + 'T' + pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
  }

  function formatDateShort(d) {
    if (!d) return '';
    return utils.formatDate(d, 'yyyy/MM/dd HH:mm:ss');
  }

  function handleStartChange(e) {
    const v = e.target.value;
    if (v) filterStart = new Date(v);
  }

  function handleEndChange(e) {
    const v = e.target.value;
    if (v) filterEnd = new Date(v);
  }

  // Toggle folder expand
  function toggleExpand(key) {
    expanded[key] = !expanded[key];
    expanded = { ...expanded };
  }

  // Close dropdown on outside click
  function handleGlobalClick(e) {
    if (showDateDropdown) {
      showDateDropdown = false;
    }
  }
</script>

<Modal title="{t('Restore Versions')} ({folderLabel()})" icon="fas fa-undo" large={true} {onclose}>
  <div class="modal-body">
    {#if loading && !versions}
      <div>{$translations, t('Loading data...')}</div>
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
      <div>{$translations, t('There are no file versions to restore.')}</div>
    {:else}
      <!-- Tree table -->
      <div id="restoreTree-container">
        <table id="restoreTree" style="width: 100%; cursor: pointer;">
          <tbody>
            {#each tree as node}
              {#if nodeVisible(node)}
                {@render treeNode(node, 0)}
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
      <hr />
      <!-- Filters row -->
      <div class="row form-inline">
        <div class="col-md-6">
          <div class="form-group">
            <label for="restoreVersionSearch"><span>{$translations, t('Filter by name')}</span>:&nbsp;</label>
            <input id="restoreVersionSearch" class="form-control" type="text" bind:value={filterText} />
          </div>
        </div>
        <div class="col-md-6">
          <div class="form-group">
            <label for="restoreVersionDateRange"><span>{$translations, t('Filter by date')}</span>:&nbsp;</label>
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="dropdown" style="display: inline-block;" onclick={(e) => e.stopPropagation()}>
              <input id="restoreVersionDateRange" class="form-control" readonly
                value="{formatDateShort(filterStart)} - {formatDateShort(filterEnd)}"
                onclick={() => showDateDropdown = !showDateDropdown}
                style="cursor: pointer; min-width: 300px;" />
              {#if showDateDropdown}
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div class="dropdown-menu" style="display: block; padding: 10px; min-width: 320px; right: 0; left: auto;" onclick={(e) => e.stopPropagation()}>
                  <div style="margin-bottom: 8px;">
                    {#each dateRanges() as range}
                      <button type="button" class="btn btn-default btn-xs btn-block text-left" style="margin-bottom: 2px;"
                        onclick={() => applyDateRange(range.start, range.end)}>
                        {range.label}
                      </button>
                    {/each}
                  </div>
                  <hr style="margin: 5px 0;" />
                  <div style="margin-bottom: 4px;">
                    <label style="font-weight: normal; font-size: 12px;">{$translations, t('Custom Range')}</label>
                  </div>
                  <div style="margin-bottom: 4px;">
                    <input type="datetime-local" class="form-control input-sm" value={formatDateForInput(filterStart)} onchange={handleStartChange} step="1" />
                  </div>
                  <div style="margin-bottom: 4px;">
                    <input type="datetime-local" class="form-control input-sm" value={formatDateForInput(filterEnd)} onchange={handleEndChange} step="1" />
                  </div>
                  <button type="button" class="btn btn-primary btn-xs btn-block" onclick={() => showDateDropdown = false}>
                    {$translations, t('Apply')}
                  </button>
                </div>
              {/if}
            </div>
          </div>
        </div>
      </div>
    {/if}
  </div>
  <div class="modal-footer">
    {#if versions && Object.keys(versions).length > 0 && !errors}
      <button type="button" class="btn btn-primary btn-sm" disabled={selectionCount() < 1}
        onclick={() => showConfirmation = true}>
        <span class="fas fa-check"></span>&nbsp;{$translations, t('Restore')}
      </button>
    {/if}
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
  </div>
</Modal>

<!-- Confirmation modal -->
{#if showConfirmation}
  <Modal title="{t('Restore Versions')}" icon="fas fa-question-circle" status="warning" onclose={() => showConfirmation = false}>
    <div class="modal-body">
      <p style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
        {$translations, t('Are you sure you want to restore {%count%} files?', { count: selectionCount() })}
      </p>
    </div>
    <div class="modal-footer">
      <button type="button" class="btn btn-warning pull-left btn-sm" onclick={doRestore}>
        <span class="fas fa-check"></span>&nbsp;{$translations, t('Yes')}
      </button>
      <button type="button" class="btn btn-default btn-sm" onclick={() => showConfirmation = false}>
        <span class="fas fa-times"></span>&nbsp;{$translations, t('No')}
      </button>
    </div>
  </Modal>
{/if}

{#snippet treeNode(node, depth)}
  {#if node.folder}
    <!-- Folder row -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <tr class="fancytree-folder" style="cursor: pointer;" onclick={() => toggleExpand(node.key)}>
      <td style="white-space: nowrap;">
        <span style="display: inline-block; width: {depth * 24}px;"></span>
        <span class="fa fa-fw {expanded[node.key] ? 'fa-caret-down' : 'fa-caret-right'}" style="width: 16px; margin-right: 4px;"></span>
        <span class="fa fa-fw {expanded[node.key] ? 'fa-folder-open' : 'fa-folder'}" style="color: #337ab7; margin-right: 4px;"></span>
        <span style="padding-left: 4px; white-space: normal; word-break: break-all;">{node.title}</span>
      </td>
      <td class="text-right" style="white-space: nowrap;">
        <!-- Mass actions dropdown -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="dropdown pull-right" onclick={(e) => e.stopPropagation()}>
          <button class="btn btn-default btn-xs dropdown-toggle" type="button" data-toggle="dropdown"
            onclick={(e) => { e.stopPropagation(); const ul = e.currentTarget.nextElementSibling; ul.style.display = ul.style.display === 'block' ? 'none' : 'block'; }}>
            <span>{$translations, t('Mass actions')}</span>
            <span class="caret"></span>
          </button>
          <ul class="dropdown-menu dropdown-menu-right" style="display: none;">
            <li><a href="#" onclick={(e) => { e.preventDefault(); e.stopPropagation(); massAction(node.key, 'unset'); e.currentTarget.closest('.dropdown-menu').style.display = 'none'; }}>{$translations, t('Do not restore all')}</a></li>
            <li><a href="#" onclick={(e) => { e.preventDefault(); e.stopPropagation(); massAction(node.key, 'latest'); e.currentTarget.closest('.dropdown-menu').style.display = 'none'; }}>{$translations, t('Select latest version')}</a></li>
            <li><a href="#" onclick={(e) => { e.preventDefault(); e.stopPropagation(); massAction(node.key, 'oldest'); e.currentTarget.closest('.dropdown-menu').style.display = 'none'; }}>{$translations, t('Select oldest version')}</a></li>
          </ul>
        </div>
      </td>
    </tr>
    <!-- Children -->
    {#if expanded[node.key]}
      {#each node.children as child}
        {#if nodeVisible(child)}
          {@render treeNode(child, depth + 1)}
        {/if}
      {/each}
    {/if}
  {:else}
    <!-- File row -->
    {#if nodeVisible(node)}
      <tr>
        <td style="white-space: nowrap;">
          <span style="display: inline-block; width: {(depth) * 24 + 20}px;"></span>
          <span class="fa fa-fw fa-file-o" style="color: #999; margin-right: 4px;"></span>
          <span style="padding-left: 4px; white-space: normal; word-break: break-all;">{node.title}</span>
        </td>
        <td class="text-right" style="white-space: nowrap;">
          <!-- Version selector dropdown -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div class="dropdown pull-right" onclick={(e) => e.stopPropagation()}>
            <button class="btn btn-default btn-xs dropdown-toggle" type="button"
              onclick={(e) => { e.stopPropagation(); const ul = e.currentTarget.nextElementSibling; ul.style.display = ul.style.display === 'block' ? 'none' : 'block'; }}>
              {#if selections[node.key]}
                <span>{formatDateShort(selections[node.key])}</span>
              {:else}
                <span>{$translations, t('Do not restore')}</span>
              {/if}
              <span class="caret"></span>
            </button>
            <ul class="dropdown-menu dropdown-menu-right" style="display: none;">
              <li><a href="#" onclick={(e) => { e.preventDefault(); delete selections[node.key]; selections = { ...selections }; e.currentTarget.closest('.dropdown-menu').style.display = 'none'; }}>{$translations, t('Do not restore')}</a></li>
              {#each filterVersions(node.versions) as ver}
                <li><a href="#" onclick={(e) => { e.preventDefault(); selections[node.key] = ver.versionTime; selections = { ...selections }; e.currentTarget.closest('.dropdown-menu').style.display = 'none'; }}>
                  {formatDateShort(ver.versionTime)} {utils.binaryFilter(ver.size)}B
                </a></li>
              {/each}
            </ul>
          </div>
        </td>
      </tr>
    {/if}
  {/if}
{/snippet}

<style>
  #restoreTree-container {
    max-height: 400px;
    overflow-y: auto;
  }
  #restoreTree {
    width: 100%;
  }
  #restoreTree tr:hover {
    background-color: #f5f5f5;
  }
  .fancytree-folder > td:first-child {
    font-weight: bold;
  }
</style>
