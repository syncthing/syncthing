<script>
  import { onMount } from 'svelte';
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import * as utils from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';

  let { version, system, onclose } = $props();
  let paths = $state({});
  let activeTab = $state('authors');
  let authors = $state('');
  let includedSW = $state('');

  onMount(async () => {
    try {
      paths = await api.getSystemPaths();
    } catch (e) {
      console.error('Error loading paths:', e);
    }
  });

  function buildDate() {
    if (!version?.date) return '';
    try {
      const d = new Date(version.date);
      return d.toISOString().split('T')[0];
    } catch (e) {
      return version.date;
    }
  }

  function upgradeTag() {
    if (version?.tags?.includes('noupgrade')) return '(noupgrade)';
    return '';
  }
</script>

<Modal title={t('About')} status="info" icon="fas fa-heart" {onclose}>
  <div class="modal-body">
    <div class="text-center" style="margin-bottom: 20px;">
      <img src="/assets/img/logo-horizontal.svg" alt="Syncthing" style="max-height: 80px;" />
    </div>

    <p class="text-center text-muted" style="font-size: 1.5em;">
      {utils.versionString(version)}
    </p>

    {#if version?.codename}
      <p class="text-center" style="font-size: 1.2em; font-style: italic;">
        "{version.codename}"
      </p>
    {/if}

    <p class="text-center text-muted">
      {$translations, t('Build')} {buildDate()} {upgradeTag()}
    </p>

    <p class="text-center text-muted">
      Copyright &copy; 2014-{new Date().getFullYear()} the Syncthing Authors.
    </p>

    <p class="text-center text-muted">
      Syncthing is Free and Open Source Software licensed as MPL v2.0.
    </p>

    <!-- Tabs -->
    <ul class="nav nav-tabs" style="margin-top: 20px;">
      <li class:active={activeTab === 'authors'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'authors'; }}>{$translations, t('Authors')}</a>
      </li>
      <li class:active={activeTab === 'software'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'software'; }}>{$translations, t('Included Software')}</a>
      </li>
      <li class:active={activeTab === 'paths'}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" onclick={(e) => { e.preventDefault(); activeTab = 'paths'; }}>{$translations, t('Paths')}</a>
      </li>
    </ul>

    <div class="tab-content">
      {#if activeTab === 'authors'}
        <div style="max-height: 250px; overflow-y: auto; padding: 10px 0;">
          <h5>{$translations, t('The Syncthing Authors')}</h5>
          <p class="text-muted">
            <a href="https://github.com/syncthing/syncthing/blob/main/AUTHORS" target="_blank">{$translations, t('View on GitHub')}</a>
          </p>
        </div>
      {/if}
      {#if activeTab === 'software'}
        <div style="max-height: 250px; overflow-y: auto; padding: 10px 0;">
          <p class="text-muted">
            <a href="https://github.com/syncthing/syncthing/blob/main/go.sum" target="_blank">{$translations, t('View dependencies on GitHub')}</a>
          </p>
        </div>
      {/if}
      {#if activeTab === 'paths'}
        <div style="padding: 10px 0;">
          <table class="table table-striped table-condensed">
            <tbody>
              {#if paths.baseDir}
                <tr>
                  <th>{$translations, t('Configuration Directory')}</th>
                  <td>{paths.baseDir}</td>
                </tr>
              {/if}
              {#if paths.dataDir}
                <tr>
                  <th>{$translations, t('Database Location')}</th>
                  <td>{paths.dataDir}</td>
                </tr>
              {/if}
              {#if system?.myID}
                <tr>
                  <th>{$translations, t('Device ID')}</th>
                  <td style="word-break: break-all; font-family: monospace; font-size: 11px;">{system.myID}</td>
                </tr>
              {/if}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-default" onclick={onclose}>{$translations, t('Close')}</button>
  </div>
</Modal>
