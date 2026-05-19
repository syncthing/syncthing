<script>
  import { onMount, onDestroy } from 'svelte';
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import { t, translations } from '../../lib/i18n.js';

  let { onclose } = $props();

  let entries = $state([]);
  let paused = $state(false);
  let facilities = $state({});
  let activeTab = $state('log');
  let timer = null;
  let textArea;

  onMount(() => {
    loadFacilities();
    fetchLogs();
  });

  onDestroy(() => {
    if (timer) clearTimeout(timer);
  });

  let facilityPackages = $state({});

  async function loadFacilities() {
    try {
      const data = await api.getLogLevels();
      // API returns either {facilities: {levels, packages}} or flat {key: level}
      if (data.facilities) {
        facilities = data.facilities.levels || {};
        facilityPackages = data.facilities.packages || {};
      } else {
        facilities = data;
      }
    } catch (e) {
      console.error('Error loading log levels:', e);
    }
  }

  async function fetchLogs() {
    if (paused) {
      timer = setTimeout(fetchLogs, 500);
      return;
    }

    const last = entries.length > 0 ? entries[entries.length - 1].when : null;
    try {
      const data = await api.getSystemLog(last);
      if (data && data.messages) {
        entries = [...entries, ...data.messages];
        if (textArea) {
          requestAnimationFrame(() => {
            textArea.scrollTop = textArea.scrollHeight;
          });
        }
      }
    } catch (e) {
      console.error('Error fetching logs:', e);
    }

    timer = setTimeout(fetchLogs, 2000);
  }

  function logContent() {
    return entries.map(e =>
      e.when.split('.')[0].replace('T', ' ') + ' ' + (e.level || '') + ' ' + e.message
    ).join('\n');
  }

  function handleScroll() {
    if (textArea) {
      paused = textArea.scrollHeight > (textArea.scrollTop + textArea.offsetHeight + 5);
    }
  }

  function scrollToBottom() {
    if (textArea) {
      textArea.scrollTop = textArea.scrollHeight;
      paused = false;
    }
  }

  let facilitiesUpdating = $state(false);

  async function setFacilityLevel(facility, newLevel) {
    facilitiesUpdating = true;
    try {
      await api.post('system/log?facility=' + encodeURIComponent(facility) + '&level=' + newLevel);
      facilities = { ...facilities, [facility]: newLevel };
    } catch (e) {
      console.error('Error setting facility level:', e);
    }
    facilitiesUpdating = false;
  }
</script>

<Modal title={t('Logs')} icon="fas fa-wrench" large={true} {onclose}>
  <div class="modal-body">
    <ul class="nav nav-tabs">
      <li class:active={activeTab === 'log'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'log'; }}>{$translations, t('Log')}</a>
      </li>
      <li class:active={activeTab === 'facilities'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'facilities'; }}>{$translations, t('Debugging Facilities')}</a>
      </li>
    </ul>

    <div class="tab-content">
      {#if activeTab === 'log'}
        <textarea
          bind:this={textArea}
          class="form-control"
          rows="20"
          readonly
          value={logContent()}
          onscroll={handleScroll}
          style="font-family: Consolas, monospace; font-size: 11px;"
        ></textarea>
        {#if paused}
          <div class="text-center" style="margin-top: 5px;">
            <button class="btn btn-sm btn-default" onclick={scrollToBottom}>
              <span class="fas fa-arrow-down"></span>&nbsp;{$translations, t('Scroll to bottom')}
            </button>
          </div>
        {/if}
      {/if}

      {#if activeTab === 'facilities'}
        <label>{$translations, t('Available debug logging facilities:')}</label>
        <table class="table table-condensed table-striped">
          <tbody>
            {#each Object.entries(facilities).sort(([a], [b]) => a.localeCompare(b)) as [facility, level]}
              <tr>
                <td>{facilityPackages[facility] || facility} (<code>{facility}</code>)</td>
                <td class="form-group">
                  <select class="form-control" value={level.toUpperCase()} onchange={(e) => setFacilityLevel(facility, e.target.value)} disabled={facilitiesUpdating}>
                    <option value="DEBUG">{t('Debug')}</option>
                    <option value="INFO">{t('Info')}</option>
                    <option value="WARN">{t('Warning')}</option>
                    <option value="ERROR">{t('Error')}</option>
                  </select>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-default" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
  </div>
</Modal>
