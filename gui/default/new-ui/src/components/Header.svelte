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
  let uiOpen = $state(false);

  function toggleDropdown(which) {
    actionsOpen = which === 'actions' ? !actionsOpen : false;
    helpOpen = which === 'help' ? !helpOpen : false;
    langOpen = which === 'lang' ? !langOpen : false;
    uiOpen = which === 'ui' ? !uiOpen : false;
  }

  function closeDropdowns() {
    actionsOpen = false;
    helpOpen = false;
    langOpen = false;
    uiOpen = false;
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

      <!-- UI switcher dropdown -->
      <li class="dropdown" class:open={uiOpen}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" class="dropdown-toggle" onclick={(e) => { e.stopPropagation(); toggleDropdown('ui'); }}>
          <svg width="14" height="14" viewBox="0 0 256 308" style="vertical-align: -2px; fill: currentColor;"><path d="M239.682 40.707C211.113-.182 156.674-12.3 115.129 16.076L44.104 59.76c-21.144 14.453-33.963 37.515-34.96 62.883-.692 17.607 5.149 34.73 16.58 48.674a57.874 57.874 0 0 0-7.558 27.282c-.838 25.367 10.507 49.585 30.28 64.647a73.588 73.588 0 0 0 7.559 27.283c28.57 40.889 83.009 53.007 124.554 24.63l71.024-43.682c21.145-14.453 33.964-37.515 34.96-62.883.693-17.607-5.148-34.73-16.58-48.674a57.878 57.878 0 0 0 7.56-27.282c.837-25.368-10.508-49.586-30.281-64.647z"/></svg>
          <span class="caret"></span>
        </a>
        <ul class="dropdown-menu">
          <li>
            <a href="/">
              <svg width="14" height="14" viewBox="0 0 256 272" style="vertical-align: -2px; fill: #dd0031;"><path d="M.1 45.522L125.908.697l129.196 44.028-20.919 166.45-108.277 59.966-106.583-59.169z"/><path d="M255.104 44.725L125.908.697v270.444l108.277-59.866z" fill="#c3002f"/><path d="M126.107 32.274L47.714 206.693l29.285-.498 15.739-39.828h70.325l17.233 40.164 27.79.498zm.2 55.882l26.496 55.383h-49.806z" fill="#fff"/></svg>
              &nbsp;{$translations, t('Angular (Default)')}
            </a>
          </li>
          <li class="active">
            <a href="/new-ui/">
              <svg width="14" height="14" viewBox="0 0 256 308" style="vertical-align: -2px; fill: #ff3e00;"><path d="M239.682 40.707C211.113-.182 156.674-12.3 115.129 16.076L44.104 59.76c-21.144 14.453-33.963 37.515-34.96 62.883-.692 17.607 5.149 34.73 16.58 48.674a57.874 57.874 0 0 0-7.558 27.282c-.838 25.367 10.507 49.585 30.28 64.647a73.588 73.588 0 0 0 7.559 27.283c28.57 40.889 83.009 53.007 124.554 24.63l71.024-43.682c21.145-14.453 33.964-37.515 34.96-62.883.693-17.607-5.148-34.73-16.58-48.674a57.878 57.878 0 0 0 7.56-27.282c.837-25.368-10.508-49.586-30.281-64.647z"/></svg>
              &nbsp;{$translations, t('Svelte (Experimental)')}
              &nbsp;<span class="badge">aktiv</span>
            </a>
          </li>
        </ul>
      </li>

      <!-- Language selector dropdown -->
      <li class="dropdown" language-select class:open={langOpen}>
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
          <li class="divider" aria-hidden="true"></li>
          <li><a class="navbar-link" href="https://syncthing.net/" target="_blank"><span class="fa fa-fw fa-home"></span>&nbsp;{$translations, t('Home page')}</a></li>
          <li><a class="navbar-link" href="{docsURL(version)}" target="_blank"><span class="fa fa-fw fa-book"></span>&nbsp;{$translations, t('Documentation')}</a></li>
          <li><a class="navbar-link" href="https://forum.syncthing.net" target="_blank"><span class="fa fa-fw fa-users"></span>&nbsp;{$translations, t('Support')}</a></li>
          <li class="divider" aria-hidden="true"></li>
          <li><a class="navbar-link" href="https://github.com/syncthing/syncthing/releases" target="_blank"><span class="fa fa-fw fa-file-text"></span>&nbsp;{$translations, t('Changelog')}</a></li>
          <li><a class="navbar-link" href="https://data.syncthing.net/" target="_blank"><span class="fa fa-fw fa-bar-chart"></span>&nbsp;{$translations, t('Statistics')}</a></li>
          <li class="divider" aria-hidden="true"></li>
          <li><a class="navbar-link" href="https://github.com/syncthing/syncthing/issues" target="_blank"><span class="fa fa-fw fa-bug"></span>&nbsp;{$translations, t('Bugs')}</a></li>
          <li><a class="navbar-link" href="https://github.com/syncthing/syncthing" target="_blank"><span class="fa fa-fw fa-file-code-o"></span>&nbsp;{$translations, t('Source Code')}</a></li>
          <li class="divider" aria-hidden="true"></li>
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
            <li class="divider" aria-hidden="true"></li>
            <!-- svelte-ignore a11y_invalid_attribute -->
            <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.showDeviceIdentification(getThisDevice()); }}><span class="fa fa-fw fa-qrcode"></span>&nbsp;{$translations, t('Show ID')}</a></li>
            <!-- svelte-ignore a11y_invalid_attribute -->
            <li><a href="#" onclick={(e) => { e.preventDefault(); closeDropdowns(); actions.openLogViewer(); }}><span class="fa fa-fw fa-wrench"></span>&nbsp;{$translations, t('Logs')}</a></li>
            <li><a href="/rest/debug/support" target="_blank"><span class="fa fa-fw fa-user-md"></span>&nbsp;{$translations, t('Support Bundle')}</a></li>
            <li class="divider" aria-hidden="true"></li>
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
