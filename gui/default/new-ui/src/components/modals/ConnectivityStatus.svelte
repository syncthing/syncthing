<script>
  import Modal from '../Modal.svelte';
  import { t, translations } from '../../lib/i18n.js';

  let { type, listenersRunning = [], listenersFailed = [], discoveryRunning = [], discoveryFailed = [], onclose } = $props();

  let heading = $derived(
    type === 'listeners'
      ? (listenersFailed.length > 0 ? t('Listener Failures') : t('Listener Status'))
      : (discoveryFailed.length > 0 ? t('Discovery Failures') : t('Discovery Status'))
  );
  let status = $derived(
    type === 'listeners'
      ? (listenersFailed.length > 0 ? 'danger' : 'default')
      : (discoveryFailed.length > 0 ? 'danger' : 'default')
  );
  let icon = $derived(type === 'listeners' ? 'fas fa-fw fa-sitemap' : 'fas fa-fw fa-map-signs');
</script>

<Modal {heading} {status} {icon} large={true} closeable={true} {onclose}>
  <div class="modal-body">
    {#if type === 'listeners'}
      {#if listenersRunning.length === 0}
        <p>{$translations, t('Syncthing is not listening for connection attempts from other devices on any address. Only outgoing connections from this device may work.')}</p>
      {/if}
      {#if listenersRunning.length > 0}
        <p>{$translations, t('Syncthing is listening on the following network addresses for connection attempts from other devices:')}</p>
        <ul>
          {#each listenersRunning as listener}
            <li>{listener}</li>
          {/each}
        </ul>
      {/if}
      {#if listenersFailed.length > 0}
        <p>{$translations, t('Some listening addresses could not be enabled to accept connections:')}</p>
        <ul>
          {#each listenersFailed as listener}
            <li>{listener}</li>
          {/each}
        </ul>
      {/if}
    {:else}
      {#if discoveryRunning.length === 0}
        <p>{$translations, t('This device cannot automatically discover other devices or announce its own address to be found by others. Only devices with statically configured addresses can connect.')}</p>
      {/if}
      {#if discoveryRunning.length > 0}
        <p>{$translations, t('The following methods are used to discover other devices on the network and announce this device to be found by others:')}</p>
        <ul>
          {#each discoveryRunning as discovery}
            <li>{discovery}</li>
          {/each}
        </ul>
      {/if}
      {#if discoveryFailed.length > 0}
        <p>{$translations, t('Some discovery methods could not be established for finding other devices or announcing this device:')}</p>
        <ul>
          {#each discoveryFailed as discovery}
            <li>{discovery}</li>
          {/each}
        </ul>
      {/if}
      <div class="row">
        <div class="col-md-offset-2 col-md-8">
          <div class="panel panel-default">
            <div class="panel-body">{$translations, t('Failure to connect to IPv6 servers is expected if there is no IPv6 connectivity.')}</div>
          </div>
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
