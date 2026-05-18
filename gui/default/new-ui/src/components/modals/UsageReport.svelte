<script>
  import Modal from '../Modal.svelte';
  import { config as configStore, saveConfig } from '../../lib/stores.js';
  import { t, translations } from '../../lib/i18n.js';

  let { system, config, onclose, actions } = $props();

  async function acceptUR() {
    configStore.update(c => {
      c.options.urAccepted = system?.urVersionMax || 3;
      c.options.urSeen = system?.urVersionMax || 3;
      return { ...c };
    });
    await saveConfig();
    onclose();
  }

  async function declineUR() {
    configStore.update(c => {
      if (c.options.urAccepted === 0) c.options.urAccepted = -1;
      c.options.urSeen = system?.urVersionMax || 3;
      return { ...c };
    });
    await saveConfig();
    onclose();
  }
</script>

<Modal title={t('Allow Anonymous Usage Reporting?')} status="info" icon="fas fa-bar-chart" {onclose}>
  <div class="modal-body">
    <p>
      {$translations, t('Syncthing can report anonymous usage data to the developers. This helps them understand how Syncthing is used and can guide development priorities.')}
    </p>
    <p>
      {$translations, t('No personally identifiable information is collected.')}
    </p>
    <p>
      {$translations, t('Would you like to help by allowing Syncthing to send anonymous usage reports?')}
    </p>
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-success btn-sm" onclick={acceptUR}>
      <span class="fas fa-check"></span>&nbsp;{$translations, t('Yes')}
    </button>
    <button type="button" class="btn btn-danger btn-sm" onclick={declineUR}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('No')}
    </button>
  </div>
</Modal>
