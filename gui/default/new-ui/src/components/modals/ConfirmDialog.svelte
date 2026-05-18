<script>
  import Modal from '../Modal.svelte';
  import { t, translations } from '../../lib/i18n.js';

  let { title = '', status = 'warning', icon = 'fas fa-question-circle', message = '', message2 = '', confirmText = '', confirmClass = 'btn-warning', onconfirm, onclose } = $props();
</script>

<Modal {title} {status} {icon} large={false} {onclose}>
  <div class="modal-body">
    {#if message}
      <p>{message}</p>
    {/if}
    {#if message2}
      <p>{message2}</p>
    {/if}
  </div>
  <div class="modal-footer">
    {#if confirmText}
      <button type="button" class="btn {confirmClass} pull-left btn-sm" onclick={() => { if (onconfirm) onconfirm(); if (onclose) onclose(); }}>
        <span class="fas fa-check"></span>&nbsp;{$translations, confirmText}
      </button>
    {/if}
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, confirmText ? t('No') : t('Close')}
    </button>
  </div>
</Modal>
