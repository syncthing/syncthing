<script>
  import { slide } from 'svelte/transition';
  import * as utils from '../lib/utils.js';
  import { t, translations } from '../lib/i18n.js';
  import { tooltip } from '../lib/tooltip.js';

  let { device, connections, connectionsTotal, config, system, version,
    localStateTotal, metricRates, listenersFailed, listenersRunning, listenersTotal,
    discoveryFailed, discoveryTotal, actions } = $props();

  let expanded = $state(true);
</script>

<div class="panel panel-default">
  <button class="btn panel-heading" onclick={() => expanded = !expanded} aria-expanded={expanded}>
    <h4 class="panel-title">
      <span class="panel-icon identicon">{@html utils.generateIdenticon(device.deviceID)}</span>
      <div class="panel-title-text">{utils.deviceName(device)}</div>
    </h4>
  </button>

  {#if expanded}
    <div class="panel-collapse collapse in" transition:slide={{ duration: 200 }}>
      <div class="panel-body less-padding">
        <table class="table table-condensed table-striped table-auto">
          <tbody>
            <tr>
              <th><span class="fas fa-fw fa-cloud-download-alt"></span>&nbsp;{$translations, t('Download Rate')}</th>
              <td class="text-right">
                <!-- svelte-ignore a11y_invalid_attribute -->
                <a href="#" class="toggler" onclick={(e) => { e.preventDefault(); actions.toggleUnits(); }}>
                  {#if !metricRates}
                    {utils.binaryFilter(connectionsTotal.inbps)}B/s
                  {:else}
                    {utils.metricFilter(connectionsTotal.inbps * 8)}bps
                  {/if}
                  ({utils.binaryFilter(connectionsTotal.inBytesTotal)}B)
                  {#if config?.options?.maxRecvKbps > 0}
                    <br/><small><i class="text-muted">{t('Limit')}:
                      {#if !metricRates}
                        {utils.binaryFilter(config.options.maxRecvKbps * 1024)}B/s
                      {:else}
                        {utils.metricFilter(config.options.maxRecvKbps * 1024 * 8)}bps
                      {/if}
                      {#if config.options.limitBandwidthInLan}
                        ({t('Applied to LAN')})
                      {/if}
                    </i></small>
                  {/if}
                </a>
              </td>
            </tr>
            <tr>
              <th><span class="fas fa-fw fa-cloud-upload-alt"></span>&nbsp;{$translations, t('Upload Rate')}</th>
              <td class="text-right">
                <!-- svelte-ignore a11y_invalid_attribute -->
                <a href="#" class="toggler" onclick={(e) => { e.preventDefault(); actions.toggleUnits(); }}>
                  {#if !metricRates}
                    {utils.binaryFilter(connectionsTotal.outbps)}B/s
                  {:else}
                    {utils.metricFilter(connectionsTotal.outbps * 8)}bps
                  {/if}
                  ({utils.binaryFilter(connectionsTotal.outBytesTotal)}B)
                  {#if config?.options?.maxSendKbps > 0}
                    <br/><small><i class="text-muted">{t('Limit')}:
                      {#if !metricRates}
                        {utils.binaryFilter(config.options.maxSendKbps * 1024)}B/s
                      {:else}
                        {utils.metricFilter(config.options.maxSendKbps * 1024 * 8)}bps
                      {/if}
                      {#if config.options.limitBandwidthInLan}
                        ({t('Applied to LAN')})
                      {/if}
                    </i></small>
                  {/if}
                </a>
              </td>
            </tr>
            <tr>
              <th><span class="fas fa-fw fa-home"></span>&nbsp;{$translations, t('Local State (Total)')}</th>
              <td class="text-right">
                <span use:tooltip={utils.localeNumber(utils.alwaysNumber(localStateTotal.files)) + ' ' + t('files') + ', ' + utils.localeNumber(utils.alwaysNumber(localStateTotal.directories)) + ' ' + t('directories') + ', ~' + utils.binaryFilter(localStateTotal.bytes) + 'B'}>
                  <span class="far fa-copy"></span>&nbsp;{utils.localeNumber(utils.alwaysNumber(localStateTotal.files))}&ensp;
                  <span class="far fa-folder"></span>&nbsp;{utils.localeNumber(utils.alwaysNumber(localStateTotal.directories))}&ensp;
                  <span class="far fa-hdd"></span>&nbsp;~{utils.binaryFilter(localStateTotal.bytes)}B
                </span>
              </td>
            </tr>
            <tr>
              <th><span class="fas fa-fw fa-sitemap"></span>&nbsp;{$translations, t('Listeners')}</th>
              <td class="text-right">
                <span use:tooltip={t('Show detailed listener status.')}>
                  <!-- svelte-ignore a11y_invalid_attribute -->
                  <a href="#" class:text-success={listenersTotal > 0 && listenersFailed.length === 0}
                    class:text-danger={listenersTotal > 0 && listenersFailed.length === listenersTotal}
                    onclick={(e) => { e.preventDefault(); actions.showListenersStatus(); }}>
                    {listenersTotal - listenersFailed.length}/{listenersTotal}
                  </a>
                </span>
              </td>
            </tr>
            {#if system?.discoveryEnabled}
              <tr>
                <th><span class="fas fa-fw fa-map-signs"></span>&nbsp;{$translations, t('Discovery')}</th>
                <td class="text-right">
                  <span use:tooltip={t('Show detailed discovery status.')}>
                    <!-- svelte-ignore a11y_invalid_attribute -->
                    <a href="#" class:text-success={discoveryFailed.length === 0}
                      class:text-danger={discoveryFailed.length === discoveryTotal}
                      onclick={(e) => { e.preventDefault(); actions.showDiscoveryStatus(); }}>
                      {discoveryTotal - discoveryFailed.length}/{discoveryTotal}
                    </a>
                  </span>
                </td>
              </tr>
            {/if}
            <tr>
              <th><span class="far fa-fw fa-clock"></span>&nbsp;{$translations, t('Uptime')}</th>
              <td class="text-right">{utils.durationFilter(system?.uptime, 'm')}</td>
            </tr>
            <tr>
              <th><span class="fas fa-fw fa-qrcode"></span>&nbsp;{$translations, t('Identification')}</th>
              <td class="text-right">
                <span use:tooltip={t('Click to see full identification string and QR code.')}>
                  <!-- svelte-ignore a11y_invalid_attribute -->
                  <a href="#" onclick={(e) => { e.preventDefault(); actions.showDeviceIdentification(device); }}>
                    {utils.deviceShortID(device.deviceID)}
                  </a>
                </span>
              </td>
            </tr>
            <tr>
              <th><span class="fas fa-fw fa-tag"></span>&nbsp;{$translations, t('Version')}</th>
              <td class="text-right no-overflow-ellipse">{utils.versionString(version)}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>
