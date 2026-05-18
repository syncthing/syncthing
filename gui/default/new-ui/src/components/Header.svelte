<script>
  import { myID, devices } from '../lib/stores.js';
  import { deviceName as getDeviceName, docsURL } from '../lib/utils.js';
  import { t, translations, currentLocale, getAvailableLocales, useLocale, langPrettyprint } from '../lib/i18n.js';

  let { authenticated, version, upgradeInfo, actions } = $props();

  // Reactive derivation using $derived - properly reads from stores
  let thisDevName = $derived.by(() => {
    const id = $myID;
    const devs = $devices;
    const device = devs[id];
    if (!device) return 'Syncthing';
    return device.name || device.deviceID?.substr(0, 7) || 'Syncthing';
  });

  let actionsOpen = $state(false);
  let helpOpen = $state(false);
  let langOpen = $state(false);

  function toggleDropdown(which) {
    if (which === 'actions') {
      actionsOpen = !actionsOpen;
      helpOpen = false;
      langOpen = false;
    } else if (which === 'lang') {
      langOpen = !langOpen;
      actionsOpen = false;
      helpOpen = false;
    } else {
      helpOpen = !helpOpen;
      actionsOpen = false;
      langOpen = false;
    }
  }

  function closeDropdowns() {
    actionsOpen = false;
    helpOpen = false;
    langOpen = false;
  }

  function getThisDevice() {
    const id = $myID;
    const devs = $devices;
    return devs[id];
  }

  let availableLocales = getAvailableLocales();

  // Update document title reactively
  $effect(() => {
    document.title = thisDevName + ' | Syncthing';
  });
</script>

<svelte:window onclick={closeDropdowns} />

<nav class="navbar navbar-top navbar-default" role="navigation">
  <div class="container">
    <span class="navbar-brand" aria-hidden="true">
      <img class="logo hidden-xs" src="/assets/img/logo-horizontal.svg" height="32" width="117" alt=""/>
      <img class="logo hidden visible-xs" src="/assets/img/favicon-default.png" height="32" alt=""/>
    </span>

    {#if authenticated}
      <p class="navbar-text hidden-xs">{thisDevName}</p>
    {/if}

    <ul class="nav navbar-nav navbar-right">
      {#if upgradeInfo?.majorNewer}
        <li class="upgrade-newer-major">
          <button type="button" class="btn navbar-btn btn-danger btn-sm" onclick={() => actions.showMajorUpgrade()}>
            <span class="fas fa-arrow-circle-up"></span>
            <span class="hidden-xs">{$translations, t('Upgrade To {%version%}', { version: upgradeInfo.latest })}</span>
          </button>
        </li>
      {:else if upgradeInfo?.newer}
        <li class="upgrade-newer">
          <button type="button" class="btn navbar-btn btn-primary btn-sm" onclick={() => actions.showUpgrade()}>
            <span class="fas fa-arrow-circle-up"></span>
            <span class="hidden-xs">{$translations, t('Upgrade To {%version%}', { version: upgradeInfo.latest })}</span>
          </button>
        </li>
      {/if}

      <!-- Language selector dropdown -->
      <li class="dropdown" class:open={langOpen}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" class="dropdown-toggle" onclick={(e) => { e.stopPropagation(); toggleDropdown('lang'); }}>
          <span class="fas fa-globe"></span>
          <span class="hidden-xs">&nbsp;{langPrettyprint[$currentLocale] || 'English'}</span>
          <span class="caret"></span>
        </a>
        <ul class="dropdown-menu">
          {#each availableLocales as loc}
            <li class:active={$currentLocale === loc.code}>
              <!-- svelte-ignore a11y_invalid_attribute -->
              <a href="#" onclick={(e) => { e.preventDefault(); useLocale(loc.code, true); closeDropdowns(); }}>{loc.name}</a>
            </li>
          {/each}
        </ul>
      </li>

      <!-- Help dropdown -->
      <li class="dropdown action-menu" class:open={helpOpen}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" class="dropdown-toggle" onclick={(e) => { e.stopPropagation(); toggleDropdown('help'); }}>
          <span class="fa fa-question-circle"></span>
          <span class="hidden-xs">{$translations, t('Help')}</span>
          <span class="caret"></span>
        </a>
        <ul class="dropdown-menu">
          <li><a class="navbar-link" href="{docsURL(version, 'intro/gui')}" target="_blank"><span class="fa fa-fw fa-info-circle"></span>&nbsp;{$translations, t('Introduction')}</a></li>
          <li class="divider"></li>
          <li><a class="navbar-link" href="https://syncthing.net/" target="_blank"><span class="fa fa-fw fa-home"></span>&nbsp;{$translations, t('Home page')}</a></li>
          <li><a class="navbar-link" href="{docsURL(version)}" target="_blank"><span class="fa fa-fw fa-book"></span>&nbsp;{$translations, t('Documentation')}</a></li>
          <li><a class="navbar-link" href="https://forum.syncthing.net" target="_blank"><span class="fa fa-fw fa-users"></span>&nbsp;{$translations, t('Support')}</a></li>
          <li class="divider"></li>
          <li><a class="navbar-link" href="https://github.com/syncthing/syncthing/releases" target="_blank"><span class="fa fa-fw fa-file-text"></span>&nbsp;{$translations, t('Changelog')}</a></li>
          <li><a class="navbar-link" href="https://data.syncthing.net/" target="_blank"><span class="fa fa-fw fa-bar-chart"></span>&nbsp;{$translations, t('Statistics')}</a></li>
          <li class="divider"></li>
          <li><a class="navbar-link" href="https://github.com/syncthing/syncthing/issues" target="_blank"><span class="fa fa-fw fa-bug"></span>&nbsp;{$translations, t('Bugs')}</a></li>
          <li><a class="navbar-link" href="https://github.com/syncthing/syncthing" target="_blank"><span class="fa fa-fw fa-file-code-o"></span>&nbsp;{$translations, t('Source Code')}</a></li>
          <li class="divider"></li>
          <!-- svelte-ignore a11y_invalid_attribute -->
          <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.openAbout(); }}><span class="fa fa-fw fa-heart"></span>&nbsp;{$translations, t('About')}</a></li>
        </ul>
      </li>

      {#if authenticated}
        <!-- Actions dropdown -->
        <li class="dropdown action-menu" class:open={actionsOpen}>
          <!-- svelte-ignore a11y_invalid_attribute -->
          <a href="#" class="dropdown-toggle" onclick={(e) => { e.stopPropagation(); toggleDropdown('actions'); }}>
            <span class="fa fa-cog"></span>
            <span class="hidden-xs">{$translations, t('Actions')}</span>
            <span class="caret"></span>
          </a>
          <ul class="dropdown-menu">
            <!-- svelte-ignore a11y_invalid_attribute -->
            <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.openSettings(); }}><span class="fa fa-fw fa-cog"></span>&nbsp;{$translations, t('Settings')}</a></li>
            <!-- svelte-ignore a11y_invalid_attribute -->
            <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.openAdvancedSettings(); }}><span class="fa fa-fw fa-cogs"></span>&nbsp;{$translations, t('Advanced')}</a></li>
            <li class="divider"></li>
            <!-- svelte-ignore a11y_invalid_attribute -->
            <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.showDeviceIdentification(getThisDevice()); }}><span class="fa fa-fw fa-qrcode"></span>&nbsp;{$translations, t('Show ID')}</a></li>
            <!-- svelte-ignore a11y_invalid_attribute -->
            <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.openLogViewer(); }}><span class="fa fa-fw fa-wrench"></span>&nbsp;{$translations, t('Logs')}</a></li>
            <li><a href="/rest/debug/support" target="_blank"><span class="fa fa-fw fa-user-md"></span>&nbsp;{$translations, t('Support Bundle')}</a></li>
            <li class="divider"></li>
            {#if actions.isAuthEnabled()}
              <!-- svelte-ignore a11y_invalid_attribute -->
              <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.doLogout(); }}><span class="far fa-fw fa-sign-out"></span>&nbsp;{$translations, t('Log Out')}</a></li>
            {/if}
            <!-- svelte-ignore a11y_invalid_attribute -->
            <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.doRestart(); }}><span class="fa fa-fw fa-refresh"></span>&nbsp;{$translations, t('Restart')}</a></li>
            <!-- svelte-ignore a11y_invalid_attribute -->
            <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.doShutdown(); }}><span class="fa fa-fw fa-power-off"></span>&nbsp;{$translations, t('Shut Down')}</a></li>
          </ul>
        </li>
      {/if}
    </ul>
  </div>
</nav>
