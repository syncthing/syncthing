<script>
  import Modal from '../Modal.svelte';
  import { api } from '../../lib/api.js';
  import * as utils from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';

  let { config, onclose } = $props();

  let advancedConfig = $state(JSON.parse(JSON.stringify(config)));

  // Sort for display
  advancedConfig.devices.sort((a, b) => (utils.deviceName(a) || '').localeCompare(utils.deviceName(b) || ''));
  advancedConfig.folders.sort((a, b) => (a.label || a.id).localeCompare(b.label || b.id));

  let expandedSections = $state({});

  function toggleSection(id) {
    expandedSections = { ...expandedSections, [id]: !expandedSections[id] };
  }

  function inputTypeFor(key, value) {
    if (key.startsWith('_')) return 'skip';
    if (typeof value === 'object' && value !== null) return 'skip';
    if (typeof value === 'boolean') return 'checkbox';
    if (typeof value === 'number') return 'number';
    if (Array.isArray(value)) return 'list';
    return 'text';
  }

  function uncamel(str) {
    return str.replace(/([A-Z])/g, ' $1').replace(/^./, s => s.toUpperCase());
  }

  function getEntries(obj) {
    if (!obj) return [];
    return Object.entries(obj).filter(([k, v]) => inputTypeFor(k, v) !== 'skip');
  }

  async function saveAdvanced() {
    try {
      await api.postConfig(advancedConfig);
      onclose();
    } catch (e) {
      console.error('Error saving advanced config:', e);
    }
  }
</script>

<Modal title="{t('Advanced Configuration')}" status="danger" icon="fas fa-cogs" large={true} {onclose}>
  <div class="modal-body">
    <p class="text-danger">
      <b>{$translations, t('Be careful!')}</b>
      {t('Incorrect configuration may damage your folder contents and render Syncthing inoperable.')}
    </p>

    <div class="panel-group">
      <!-- GUI -->
      <div class="panel panel-default">
        <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('gui')}>
          <h4 class="panel-title">GUI</h4>
        </div>
        {#if expandedSections.gui}
          <div class="panel-body less-padding">
            <form class="form-horizontal" role="form">
              {#each getEntries(advancedConfig.gui) as [key, value]}
                <div class="form-group">
                  <label class="col-sm-4 control-label">
                    {uncamel(key)}&nbsp;<a href="{utils.docsURL(null, 'users/config#config-option-gui.' + key.toLowerCase())}" target="_blank"><span class="fas fa-question-circle"></span></a>
                  </label>
                  <div class="col-sm-8">
                    {#if typeof value === 'boolean'}
                      <input class="form-control" type="checkbox" bind:checked={advancedConfig.gui[key]} />
                    {:else if typeof value === 'number'}
                      <input class="form-control" type="number" bind:value={advancedConfig.gui[key]} />
                    {:else if Array.isArray(value)}
                      <input class="form-control" type="text" value={value.join(', ')} onchange={(e) => advancedConfig.gui[key] = e.target.value.split(',').map(s => s.trim())} />
                    {:else}
                      <input class="form-control" type="text" bind:value={advancedConfig.gui[key]} />
                    {/if}
                  </div>
                </div>
              {/each}
            </form>
          </div>
        {/if}
      </div>

      <!-- Options -->
      <div class="panel panel-default">
        <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('options')}>
          <h4 class="panel-title">{$translations, t('Options')}</h4>
        </div>
        {#if expandedSections.options}
          <div class="panel-body less-padding">
            <form class="form-horizontal" role="form">
              {#each getEntries(advancedConfig.options) as [key, value]}
                <div class="form-group">
                  <label class="col-sm-4 control-label">
                    {uncamel(key)}&nbsp;<a href="{utils.docsURL(null, 'users/config#config-option-options.' + key.toLowerCase())}" target="_blank"><span class="fas fa-question-circle"></span></a>
                  </label>
                  <div class="col-sm-8">
                    {#if typeof value === 'boolean'}
                      <input class="form-control" type="checkbox" bind:checked={advancedConfig.options[key]} />
                    {:else if typeof value === 'number'}
                      <input class="form-control" type="number" bind:value={advancedConfig.options[key]} />
                    {:else if Array.isArray(value)}
                      <input class="form-control" type="text" value={value.join(', ')} onchange={(e) => advancedConfig.options[key] = e.target.value.split(',').map(s => s.trim())} />
                    {:else}
                      <input class="form-control" type="text" bind:value={advancedConfig.options[key]} />
                    {/if}
                  </div>
                </div>
              {/each}
            </form>
          </div>
        {/if}
      </div>

      <!-- LDAP -->
      {#if advancedConfig.ldap}
        <div class="panel panel-default">
          <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('ldap')}>
            <h4 class="panel-title">LDAP</h4>
          </div>
          {#if expandedSections.ldap}
            <div class="panel-body less-padding">
              <form class="form-horizontal" role="form">
                {#each getEntries(advancedConfig.ldap) as [key, value]}
                  <div class="form-group">
                    <label class="col-sm-4 control-label">
                      {uncamel(key)}&nbsp;<a href="{utils.docsURL(null, 'users/config#config-option-ldap.' + key.toLowerCase())}" target="_blank"><span class="fas fa-question-circle"></span></a>
                    </label>
                    <div class="col-sm-8">
                      {#if typeof value === 'boolean'}
                        <input class="form-control" type="checkbox" bind:checked={advancedConfig.ldap[key]} />
                      {:else if typeof value === 'number'}
                        <input class="form-control" type="number" bind:value={advancedConfig.ldap[key]} />
                      {:else}
                        <input class="form-control" type="text" bind:value={advancedConfig.ldap[key]} />
                      {/if}
                    </div>
                  </div>
                {/each}
              </form>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Folders -->
      <div class="panel panel-default">
        <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('folders')}>
          <h4 class="panel-title">{$translations, t('Folders')}</h4>
        </div>
        {#if expandedSections.folders}
          <div class="panel-body less-padding">
            {#each advancedConfig.folders as folder, fi}
              <div class="panel panel-default">
                <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('folder-' + fi)}>
                  <h4 class="panel-title">
                    {t('Folder')} "{folder.label || folder.id}"{#if folder.label} ({folder.id}){/if}
                  </h4>
                </div>
                {#if expandedSections['folder-' + fi]}
                  <div class="panel-body less-padding">
                    <form class="form-horizontal" role="form">
                      {#each getEntries(folder) as [key, value]}
                        <div class="form-group">
                          <label class="col-sm-4 control-label">
                            {uncamel(key)}&nbsp;<a href="{utils.docsURL(null, 'users/config#config-option-folder.' + key.toLowerCase())}" target="_blank"><span class="fas fa-question-circle"></span></a>
                          </label>
                          <div class="col-sm-8">
                            {#if typeof value === 'boolean'}
                              <input class="form-control" type="checkbox" bind:checked={advancedConfig.folders[fi][key]} />
                            {:else if typeof value === 'number'}
                              <input class="form-control" type="number" bind:value={advancedConfig.folders[fi][key]} />
                            {:else if Array.isArray(value)}
                              <input class="form-control" type="text" value={value.join(', ')} onchange={(e) => advancedConfig.folders[fi][key] = e.target.value.split(',').map(s => s.trim())} />
                            {:else}
                              <input class="form-control" type="text" bind:value={advancedConfig.folders[fi][key]} />
                            {/if}
                          </div>
                        </div>
                      {/each}
                    </form>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Devices -->
      <div class="panel panel-default">
        <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('devices')}>
          <h4 class="panel-title">{$translations, t('Devices')}</h4>
        </div>
        {#if expandedSections.devices}
          <div class="panel-body less-padding">
            {#each advancedConfig.devices as dev, di}
              <div class="panel panel-default">
                <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('device-' + di)}>
                  <h4 class="panel-title">
                    {t('Device')} "{utils.deviceName(dev)}"
                  </h4>
                </div>
                {#if expandedSections['device-' + di]}
                  <div class="panel-body less-padding">
                    <form class="form-horizontal" role="form">
                      {#each getEntries(dev) as [key, value]}
                        <div class="form-group">
                          <label class="col-sm-4 control-label">
                            {uncamel(key)}&nbsp;<a href="{utils.docsURL(null, 'users/config#config-option-device.' + key.toLowerCase())}" target="_blank"><span class="fas fa-question-circle"></span></a>
                          </label>
                          <div class="col-sm-8">
                            {#if typeof value === 'boolean'}
                              <input class="form-control" type="checkbox" bind:checked={advancedConfig.devices[di][key]} />
                            {:else if typeof value === 'number'}
                              <input class="form-control" type="number" bind:value={advancedConfig.devices[di][key]} />
                            {:else if Array.isArray(value)}
                              <input class="form-control" type="text" value={value.join(', ')} onchange={(e) => advancedConfig.devices[di][key] = e.target.value.split(',').map(s => s.trim())} />
                            {:else}
                              <input class="form-control" type="text" bind:value={advancedConfig.devices[di][key]} />
                            {/if}
                          </div>
                        </div>
                      {/each}
                    </form>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Defaults -->
      {#if advancedConfig.defaults}
        <div class="panel panel-default">
          <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('defaults')}>
            <h4 class="panel-title">{$translations, t('Defaults')}</h4>
          </div>
          {#if expandedSections.defaults}
            <div class="panel-body less-padding">
              {#if advancedConfig.defaults.folder}
                <div class="panel panel-default">
                  <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('default-folder')}>
                    <h4 class="panel-title">{$translations, t('Default Folder')}</h4>
                  </div>
                  {#if expandedSections['default-folder']}
                    <form class="form-horizontal" role="form">
                      {#each getEntries(advancedConfig.defaults.folder) as [key, value]}
                        <div class="form-group">
                          <label class="col-sm-4 control-label">{uncamel(key)}</label>
                          <div class="col-sm-8">
                            {#if typeof value === 'boolean'}
                              <input class="form-control" type="checkbox" bind:checked={advancedConfig.defaults.folder[key]} />
                            {:else if typeof value === 'number'}
                              <input class="form-control" type="number" bind:value={advancedConfig.defaults.folder[key]} />
                            {:else}
                              <input class="form-control" type="text" bind:value={advancedConfig.defaults.folder[key]} />
                            {/if}
                          </div>
                        </div>
                      {/each}
                    </form>
                  {/if}
                </div>
              {/if}
              {#if advancedConfig.defaults.device}
                <div class="panel panel-default">
                  <div class="panel-heading" style="cursor: pointer;" onclick={() => toggleSection('default-device')}>
                    <h4 class="panel-title">{$translations, t('Default Device')}</h4>
                  </div>
                  {#if expandedSections['default-device']}
                    <form class="form-horizontal" role="form">
                      {#each getEntries(advancedConfig.defaults.device) as [key, value]}
                        <div class="form-group">
                          <label class="col-sm-4 control-label">{uncamel(key)}</label>
                          <div class="col-sm-8">
                            {#if typeof value === 'boolean'}
                              <input class="form-control" type="checkbox" bind:checked={advancedConfig.defaults.device[key]} />
                            {:else if typeof value === 'number'}
                              <input class="form-control" type="number" bind:value={advancedConfig.defaults.device[key]} />
                            {:else}
                              <input class="form-control" type="text" bind:value={advancedConfig.defaults.device[key]} />
                            {/if}
                          </div>
                        </div>
                      {/each}
                    </form>
                  {/if}
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  <div class="modal-footer">
    <button type="button" class="btn btn-primary btn-sm" onclick={saveAdvanced}>
      <span class="fas fa-check"></span>&nbsp;{$translations, t('Save')}
    </button>
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
  </div>
</Modal>
