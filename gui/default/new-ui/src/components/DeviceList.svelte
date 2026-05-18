<script>
  import DeviceItem from './DeviceItem.svelte';
  import ThisDevice from './ThisDevice.svelte';
  import * as utils from '../lib/utils.js';
  import { t, translations } from '../lib/i18n.js';

  let { devicesGrouped, devices, connections, connectionsTotal, completion, deviceStats,
    discoveryCache, config, model, folders, myID, system, version, localStateTotal,
    metricRates, listenersFailed, listenersRunning, listenersTotal,
    discoveryFailed, discoveryTotal, actions } = $props();

  function otherDevicesCount() {
    return Object.values(devices).filter(d => d.deviceID !== myID).length;
  }

  function isAtleastOneDevicePausedStateSetTo(pause) {
    for (const d of Object.values(devices)) {
      if (d.paused === pause) return true;
    }
    return false;
  }
</script>

<!-- This Device -->
<h3>{$translations, t('This Device')}</h3>
{#if devices[myID]}
  <ThisDevice
    device={devices[myID]}
    {connections}
    {connectionsTotal}
    {config}
    {system}
    {version}
    {localStateTotal}
    {metricRates}
    {listenersFailed}
    {listenersRunning}
    {listenersTotal}
    {discoveryFailed}
    {discoveryTotal}
    {actions}
  />
{/if}

<!-- Remote Devices -->
<h3>
  {$translations, t('Remote Devices')}
  {#if otherDevicesCount() > 1}
    ({otherDevicesCount()})
  {/if}
</h3>

{#each Object.entries(devicesGrouped) as [groupName, groupedDevices], groupIdx}
  {#if groupName}
    <h4>
      {groupName}
      {#if groupedDevices.length > 1 && groupName.length > 0}
        ({groupedDevices.length})
      {/if}
    </h4>
  {/if}

  <div class="panel-group" id="devices-{groupIdx}">
    {#each groupedDevices as deviceCfg, deviceIdx (deviceCfg.deviceID)}
      <DeviceItem
        {deviceCfg}
        {connections}
        {completion}
        {deviceStats}
        {discoveryCache}
        {devices}
        {folders}
        {myID}
        {system}
        {metricRates}
        {groupIdx}
        {deviceIdx}
        {actions}
      />
    {/each}
  </div>
{/each}

<div class="form-group">
  <span class="pull-right">
    {#if isAtleastOneDevicePausedStateSetTo(false)}
      <button type="button" class="btn btn-sm btn-default" onclick={() => actions.setAllDevicesPause(true)}>
        <span class="fas fa-pause"></span>&nbsp;{$translations, t('Pause All')}
      </button>
    {/if}
    {#if isAtleastOneDevicePausedStateSetTo(true)}
      <button type="button" class="btn btn-sm btn-default" onclick={() => actions.setAllDevicesPause(false)}>
        <span class="fas fa-play"></span>&nbsp;{$translations, t('Resume All')}
      </button>
    {/if}
    <button type="button" class="btn btn-sm btn-default" onclick={() => actions.openGlobalChanges()}>
      <span class="fas fa-fw fa-info-circle"></span>&nbsp;{$translations, t('Recent Changes')}
    </button>
    <button type="button" class="btn btn-sm btn-default" onclick={() => actions.addDevice()}>
      <span class="fas fa-plus"></span>&nbsp;{$translations, t('Add Remote Device')}
    </button>
  </span>
  <div class="clearfix"></div>
</div>
