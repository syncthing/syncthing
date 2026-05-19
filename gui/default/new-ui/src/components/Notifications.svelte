<script>
  import { t, translations } from '../lib/i18n.js';
  import * as utils from '../lib/utils.js';
  import { api } from '../lib/api.js';
  import { config as configStore, saveConfig } from '../lib/stores.js';

  let { config, actions } = $props();

  function hasNotification(id) {
    return config?.options?.unackedNotificationIDs?.includes(id);
  }

  async function dismissNotification(id) {
    configStore.update(c => {
      c.options.unackedNotificationIDs = (c.options.unackedNotificationIDs || []).filter(n => n !== id);
      return { ...c };
    });
    await saveConfig();
  }

  async function setCrashReportingEnabled(enabled) {
    configStore.update(c => {
      c.options.crashReportingEnabled = enabled;
      return { ...c };
    });
  }
</script>

{#if hasNotification('channelNotification')}
  <div class="panel panel-success">
    <div class="panel-heading">
      <h3 class="panel-title"><span class="fas fa-bolt"></span>&nbsp;{$translations, t('Automatic upgrades')}</h3>
    </div>
    <div class="panel-body">
      <p>{t('Automatic upgrade now offers the choice between stable releases and release candidates.')}</p>
      <p>
        {t('Release candidates contain the latest features and fixes. They are similar to the traditional bi-weekly Syncthing releases.')}
        {t('Stable releases are delayed by about two weeks. During this time they go through testing as release candidates.')}
        {t('You can read more about the two release channels at the link below.')}
      </p>
      <p>{t('You can change your choice at any time in the Settings dialog.')}</p>
      <p><a href="{utils.docsURL(null, 'users/releases')}"><span class="fas fa-info-circle"></span>&nbsp;{t('Learn more')}</a></p>
    </div>
    <div class="panel-footer">
      <button type="button" class="btn btn-sm btn-default pull-right" onclick={() => { actions.openSettings(); dismissNotification('channelNotification'); }}>
        <span class="fas fa-cog"></span>&nbsp;{$translations, t('Settings')}
      </button>
      <button type="button" class="btn btn-sm btn-default" onclick={() => dismissNotification('channelNotification')}>
        <span class="fas fa-check"></span>&nbsp;{$translations, t('OK')}
      </button>
      <div class="clearfix"></div>
    </div>
  </div>
{/if}

{#if hasNotification('fsWatcherNotification')}
  <div class="panel panel-success">
    <div class="panel-heading">
      <h3 class="panel-title"><span class="fas fa-bolt"></span>&ensp;{$translations, t('Watching for Changes')}</h3>
    </div>
    <div class="panel-body">
      <p>{t('Continuously watching for changes is now available within Syncthing. This will detect changes on disk and issue a scan on only the modified paths. The benefits are that changes are propagated quicker and that less full scans are required.')}</p>
      <p><a href="{utils.docsURL(null, 'users/syncing#scanning')}"><span class="fas fa-info-circle"></span>&nbsp;{t('Learn more')}</a></p>
      <p>{t('Do you want to enable watching for changes for all your folders?')}<br />{t('Additionally the full rescan interval will be increased (times 60, i.e. new default of 1h). You can also configure it manually for every folder later after choosing No.')}</p>
    </div>
    <div class="panel-footer clearfix">
      <div class="pull-right">
        <button type="button" class="btn btn-primary btn-sm" onclick={() => dismissNotification('fsWatcherNotification')}>
          <span class="fas fa-check"></span>&nbsp;{$translations, t('Yes')}
        </button>
        <button type="button" class="btn btn-default btn-sm" onclick={() => dismissNotification('fsWatcherNotification')}>
          <span class="fas fa-times"></span>&nbsp;{$translations, t('No')}
        </button>
      </div>
      <div class="clearfix"></div>
    </div>
  </div>
{/if}

{#if hasNotification('crAutoEnabled')}
  <div class="panel panel-success">
    <div class="panel-heading">
      <h3 class="panel-title"><span class="fas fa-bolt"></span>&ensp;{$translations, t('Automatic Crash Reporting')}</h3>
    </div>
    <div class="panel-body">
      <p>{t('Syncthing now supports automatically reporting crashes to the developers. This feature is enabled by default.')}</p>
      <p><a href="{utils.docsURL(null, 'users/crashrep')}"><span class="fas fa-info-circle"></span>&nbsp;{t('Learn more')}</a></p>
    </div>
    <div class="panel-footer clearfix">
      <div class="pull-right">
        <button type="button" class="btn btn-danger" onclick={() => { setCrashReportingEnabled(false); dismissNotification('crAutoEnabled'); }}>
          <span class="fas fa-times"></span>&nbsp;{$translations, t('Disable Crash Reporting')}
        </button>
        <button type="button" class="btn btn-default" onclick={() => dismissNotification('crAutoEnabled')}>
          <span class="fas fa-check"></span>&nbsp;{$translations, t('OK')}
        </button>
      </div>
      <div class="clearfix"></div>
    </div>
  </div>
{/if}

{#if hasNotification('crAutoDisabled')}
  <div class="panel panel-success">
    <div class="panel-heading">
      <h3 class="panel-title"><span class="fas fa-bolt"></span>&ensp;{$translations, t('Automatic Crash Reporting')}</h3>
    </div>
    <div class="panel-body">
      <p>{t('Syncthing now supports automatically reporting crashes to the developers. This feature is enabled by default.')}</p>
      <p>{t('However, your current settings indicate you might not want it enabled. We have disabled automatic crash reporting for you.')}</p>
      <p><a href="{utils.docsURL(null, 'users/crashrep')}"><span class="fas fa-info-circle"></span>&nbsp;{t('Learn more')}</a></p>
    </div>
    <div class="panel-footer clearfix">
      <div class="pull-right">
        <button type="button" class="btn btn-success" onclick={() => { setCrashReportingEnabled(true); dismissNotification('crAutoDisabled'); }}>
          <span class="fas fa-check"></span>&nbsp;{$translations, t('Enable Crash Reporting')}
        </button>
        <button type="button" class="btn btn-default" onclick={() => dismissNotification('crAutoDisabled')}>
          <span class="fas fa-check"></span>&nbsp;{$translations, t('OK')}
        </button>
      </div>
      <div class="clearfix"></div>
    </div>
  </div>
{/if}

{#if hasNotification('authenticationUserAndPassword')}
  <div class="panel panel-success">
    <div class="panel-heading">
      <h3 class="panel-title"><span class="fas fa-bolt"></span>&nbsp;{$translations, t('GUI Authentication: Set User and Password')}</h3>
    </div>
    <div class="panel-body">
      <p>{t('Username/Password has not been set for the GUI authentication. Please consider setting it up.')}</p>
      <p>{t('If you want to prevent other users on this computer from accessing Syncthing and through it your files, consider setting up authentication.')}</p>
    </div>
    <div class="panel-footer">
      <button type="button" class="btn btn-sm btn-default pull-right" onclick={() => actions.openSettings()}>
        <span class="fas fa-cog"></span>&nbsp;{$translations, t('Settings')}
      </button>
      <button type="button" class="btn btn-sm btn-default pull-left" onclick={() => dismissNotification('authenticationUserAndPassword')}>
        <span class="fa fa-check-circle"></span>&nbsp;{$translations, t('OK')}
      </button>
      <div class="clearfix"></div>
    </div>
  </div>
{/if}
