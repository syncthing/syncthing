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
          <svg width="14" height="16" viewBox="0 0 107 128" style="vertical-align: -2px; filter: drop-shadow(0 0 2px rgba(255,255,255,0.5));"><path fill="#ff3e00" d="M94.1566,22.8189c-10.4-14.8851-30.94-19.2971-45.7914-9.8348L22.2825,29.6078A29.9234,29.9234,0,0,0,8.7639,49.6506a31.5136,31.5136,0,0,0,3.1076,20.2318A30.0061,30.0061,0,0,0,7.3953,81.0653a31.8886,31.8886,0,0,0,5.4473,24.1157c10.4022,14.8865,30.9423,19.2966,45.7914,9.8348L84.7167,98.3921A29.9177,29.9177,0,0,0,98.2353,78.3493,31.5263,31.5263,0,0,0,95.13,58.117a30,30,0,0,0,4.4743-11.1824,31.88,31.88,0,0,0-5.4473-24.1157"/><path fill="#fff" d="M45.8171,106.5815A20.7182,20.7182,0,0,1,23.58,98.3389a19.1739,19.1739,0,0,1-3.2766-14.5025,18.1886,18.1886,0,0,1,.6233-2.4357l.4912-1.4978,1.3363.9815a33.6443,33.6443,0,0,0,10.203,5.0978l.9694.2941-.0893.9675a5.8474,5.8474,0,0,0,1.052,3.8781,6.2389,6.2389,0,0,0,6.6952,2.485,5.7449,5.7449,0,0,0,1.6021-.7041L69.27,76.281a5.4306,5.4306,0,0,0,2.4506-3.631,5.7948,5.7948,0,0,0-.9875-4.3712,6.2436,6.2436,0,0,0-6.6978-2.4864,5.7427,5.7427,0,0,0-1.6.7036l-9.9532,6.3449a19.0329,19.0329,0,0,1-5.2965,2.3259,20.7181,20.7181,0,0,1-22.2368-8.2427,19.1725,19.1725,0,0,1-3.2766-14.5024,17.9885,17.9885,0,0,1,8.13-12.0513L55.8833,23.7472a19.0038,19.0038,0,0,1,5.3-2.3287A20.7182,20.7182,0,0,1,83.42,29.6611a19.1739,19.1739,0,0,1,3.2766,14.5025,18.4,18.4,0,0,1-.6233,2.4357l-.4912,1.4978-1.3356-.98a33.6175,33.6175,0,0,0-10.2037-5.1l-.9694-.2942.0893-.9675a5.8588,5.8588,0,0,0-1.052-3.878,6.2389,6.2389,0,0,0-6.6952-2.485,5.7449,5.7449,0,0,0-1.6021.7041L37.73,51.719a5.4218,5.4218,0,0,0-2.4487,3.63,5.7862,5.7862,0,0,0,.9856,4.3717,6.2437,6.2437,0,0,0,6.6978,2.4864,5.7652,5.7652,0,0,0,1.602-.7041l9.9519-6.3425a18.978,18.978,0,0,1,5.2959-2.3278,20.7181,20.7181,0,0,1,22.2368,8.2427,19.1725,19.1725,0,0,1,3.2766,14.5024,17.9977,17.9977,0,0,1-8.13,12.0532L51.1167,104.2528a19.0038,19.0038,0,0,1-5.3,2.3287"/></svg>
          <span class="caret"></span>
        </a>
        <ul class="dropdown-menu">
          <li>
            <a href="/">
              <svg width="14" height="14" viewBox="0 0 256 272" style="vertical-align: -2px; filter: drop-shadow(0 0 2px rgba(255,255,255,0.5));"><path fill="#dd0031" d="M.1 45.522L125.908.697l129.196 44.028-20.919 166.45-108.277 59.966-106.583-59.169z"/><path fill="#c3002f" d="M255.104 44.725L125.908.697v270.444l108.277-59.866z"/><path fill="#fff" d="M126.107 32.274L47.714 206.693l29.285-.498 15.739-39.828h70.325l17.233 40.164 27.79.498zm.2 55.882l26.496 55.383h-49.806z"/></svg>
              &nbsp;Angular (Default)
            </a>
          </li>
          <li>
            <a href="/new-ui/">
              <svg width="14" height="16" viewBox="0 0 107 128" style="vertical-align: -2px; filter: drop-shadow(0 0 2px rgba(255,255,255,0.5));"><path fill="#ff3e00" d="M94.1566,22.8189c-10.4-14.8851-30.94-19.2971-45.7914-9.8348L22.2825,29.6078A29.9234,29.9234,0,0,0,8.7639,49.6506a31.5136,31.5136,0,0,0,3.1076,20.2318A30.0061,30.0061,0,0,0,7.3953,81.0653a31.8886,31.8886,0,0,0,5.4473,24.1157c10.4022,14.8865,30.9423,19.2966,45.7914,9.8348L84.7167,98.3921A29.9177,29.9177,0,0,0,98.2353,78.3493,31.5263,31.5263,0,0,0,95.13,58.117a30,30,0,0,0,4.4743-11.1824,31.88,31.88,0,0,0-5.4473-24.1157"/><path fill="#fff" d="M45.8171,106.5815A20.7182,20.7182,0,0,1,23.58,98.3389a19.1739,19.1739,0,0,1-3.2766-14.5025,18.1886,18.1886,0,0,1,.6233-2.4357l.4912-1.4978,1.3363.9815a33.6443,33.6443,0,0,0,10.203,5.0978l.9694.2941-.0893.9675a5.8474,5.8474,0,0,0,1.052,3.8781,6.2389,6.2389,0,0,0,6.6952,2.485,5.7449,5.7449,0,0,0,1.6021-.7041L69.27,76.281a5.4306,5.4306,0,0,0,2.4506-3.631,5.7948,5.7948,0,0,0-.9875-4.3712,6.2436,6.2436,0,0,0-6.6978-2.4864,5.7427,5.7427,0,0,0-1.6.7036l-9.9532,6.3449a19.0329,19.0329,0,0,1-5.2965,2.3259,20.7181,20.7181,0,0,1-22.2368-8.2427,19.1725,19.1725,0,0,1-3.2766-14.5024,17.9885,17.9885,0,0,1,8.13-12.0513L55.8833,23.7472a19.0038,19.0038,0,0,1,5.3-2.3287A20.7182,20.7182,0,0,1,83.42,29.6611a19.1739,19.1739,0,0,1,3.2766,14.5025,18.4,18.4,0,0,1-.6233,2.4357l-.4912,1.4978-1.3356-.98a33.6175,33.6175,0,0,0-10.2037-5.1l-.9694-.2942.0893-.9675a5.8588,5.8588,0,0,0-1.052-3.878,6.2389,6.2389,0,0,0-6.6952-2.485,5.7449,5.7449,0,0,0-1.6021.7041L37.73,51.719a5.4218,5.4218,0,0,0-2.4487,3.63,5.7862,5.7862,0,0,0,.9856,4.3717,6.2437,6.2437,0,0,0,6.6978,2.4864,5.7652,5.7652,0,0,0,1.602-.7041l9.9519-6.3425a18.978,18.978,0,0,1,5.2959-2.3278,20.7181,20.7181,0,0,1,22.2368,8.2427,19.1725,19.1725,0,0,1,3.2766,14.5024,17.9977,17.9977,0,0,1-8.13,12.0532L51.1167,104.2528a19.0038,19.0038,0,0,1-5.3,2.3287"/></svg>
              &nbsp;Svelte (Experimental)
              <span class="fa fa-check" style="margin-left: 5px; opacity: 0.6;"></span>
            </a>
          </li>
        </ul>
      </li>

      <!-- Language selector dropdown -->
      <li class="dropdown" language-select class:open={langOpen}>
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a href="#" class="dropdown-toggle" onclick={(e) => { e.stopPropagation(); toggleDropdown('lang'); }}><span class="fas fa-globe"></span><span class="hidden-xs">&nbsp;{langPrettyprint[$currentLocale] || 'English'}</span> <span class="caret"></span></a>
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
