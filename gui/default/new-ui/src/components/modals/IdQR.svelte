<script>
  import Modal from '../Modal.svelte';
  import * as utils from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';

  let { device, onclose } = $props();

  let copied = $state(false);

  async function copyID() {
    const success = await utils.copyToClipboard(device.deviceID);
    if (success) {
      copied = true;
      setTimeout(() => { copied = false; }, 2000);
    }
  }
</script>

<Modal title={t('Device Identification')} status="info" icon="fas fa-qrcode" {onclose}>
  <div class="modal-body">
    <div class="text-center" style="margin-bottom: 15px;">
      <span class="identicon" style="display:inline-block; width:64px; height:64px;">
        {@html utils.generateIdenticon(device.deviceID)}
      </span>
    </div>

    <div class="form-group">
      <label>{$translations, t('Device ID')}</label>
      <div class="input-group">
        <input type="text" class="form-control" value={device.deviceID} readonly
          style="font-family: monospace; font-size: 11px;" />
        <span class="input-group-btn">
          <button type="button" class="btn btn-default" onclick={copyID} title="{t('Copy')}">
            <span class="far fa-copy"></span>
          </button>
        </span>
      </div>
    </div>

    {#if device.name}
      <div class="form-group">
        <label>{$translations, t('Device Name')}</label>
        <input type="text" class="form-control" value={device.name} readonly />
      </div>
    {/if}

    <div class="text-center" style="margin-top: 15px;">
      <p class="text-muted">
        {copied ? t('Copied!') : t('Share this device ID with others so they can connect to you.')}
      </p>
      <div class="btn-group">
        <button type="button" class="btn btn-default" onclick={copyID}>
          <span class="far fa-copy"></span>&nbsp;{$translations, t('Copy')}
        </button>
      </div>
    </div>
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-default" onclick={onclose}>{$translations, t('Close')}</button>
  </div>
</Modal>
