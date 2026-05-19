<script>
  import Modal from '../Modal.svelte';
  import * as utils from '../../lib/utils.js';
  import { t, translations } from '../../lib/i18n.js';

  let { device, onclose } = $props();

  function copyID(e) {
    utils.copyToClipboard(device.deviceID);
  }
</script>

<Modal title="{t('Device Identification')} - {utils.deviceName(device)}" status="info" icon="fas fa-qrcode" large={true} {onclose}>
  <div class="modal-body text-center">
    <div class="well well-sm text-monospace select-on-click"><strong>{device.deviceID}</strong></div>
    {#if device.deviceID}
      <div>
        <img class="img-thumbnail" src="/rest/qr/?text={device.deviceID}" height="328" width="328" alt="{t('QR code')}" />
        <div class="btn-group-vertical" style="vertical-align: top;">
          <button type="button" class="btn btn-default" onclick={copyID}>
            <span class="fa fa-lg fa-clone text-left"></span>&nbsp;{$translations, t('Copy')}
          </button>
        </div>
      </div>
    {/if}
  </div>
  <div class="modal-footer">
    <button type="button" class="btn btn-default btn-sm" onclick={onclose}>
      <span class="fas fa-times"></span>&nbsp;{$translations, t('Close')}
    </button>
  </div>
</Modal>
