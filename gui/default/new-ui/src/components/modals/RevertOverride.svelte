<script>
  import Modal from '../Modal.svelte';
  import { t, translations } from '../../lib/i18n.js';

  let { type = 'revert', onconfirm, onclose } = $props();

  function heading() {
    if (type === 'deleteEnc') return t('Delete Unexpected Items');
    if (type === 'override') return t('Send Local Changes');
    return t('Revert Local Changes');
  }

  function icon() {
    if (type === 'override') return 'fas fa-arrow-circle-up';
    return 'fas fa-arrow-circle-down';
  }

  function confirmLabel() {
    if (type === 'deleteEnc') return t('Delete');
    if (type === 'override') return t('Override');
    return t('Revert');
  }
</script>

<Modal title={heading()} status="danger" icon={icon()} large={false} {onclose}>
  <div class="modal-body">
    <p><span>{$translations, t('Warning')}!</span></p>
    {#if type === 'deleteEnc'}
      <p>
        {t('Unexpected items have been found in this folder.')}
        {t('You should never add or change anything locally in a "Receive Encrypted" folder.')}
      </p>
      <p>{t('Are you sure you want to permanently delete all these files?')}</p>
    {:else if type === 'override'}
      <p>{t('The folder content on other devices will be overwritten to become identical with this device. Files not present here will be deleted on other devices.')}</p>
      <p>{t('Are you sure you want to override all remote changes?')}</p>
    {:else}
      <p>{t('The folder content on this device will be overwritten to become identical with other devices. Files newly added here will be deleted.')}</p>
      <p>{t('Are you sure you want to revert all local changes?')}</p>
    {/if}
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-danger pull-left btn-sm" onclick={() => { if (onconfirm) onconfirm(); if (onclose) onclose(); }}>
      <span class="fas fa-check"></span>&nbsp;{$translations, confirmLabel()}
    </button>
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Cancel')}
    </button>
  </div>
</Modal>
