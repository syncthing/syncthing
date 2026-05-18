<script>
  import { t, translations } from '../lib/i18n.js';

  let { config, actions } = $props();
</script>

<!-- Placeholder for notification panels - unacked notification IDs from config -->
{#if config?.options?.unackedNotificationIDs?.length > 0}
  {#each config.options.unackedNotificationIDs as notifID}
    <div class="row">
      <div class="col-md-12">
        <div class="panel panel-info">
          <div class="panel-heading">
            <h3 class="panel-title">
              <div class="panel-icon">
                <span class="fas fa-info-circle"></span>
              </div>
              {$translations, t('Notification')}
            </h3>
          </div>
          <div class="panel-body">
            <p>{t('Notification')}: {notifID}</p>
          </div>
        </div>
      </div>
    </div>
  {/each}
{/if}
