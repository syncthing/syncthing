<script>
  import { api } from '../lib/api.js';
  import { t, translations } from '../lib/i18n.js';

  let username = $state('');
  let password = $state('');
  let stayLoggedIn = $state(false);
  let inProgress = $state(false);
  let badLogin = $state(false);
  let failed = $state(false);

  async function authenticatePassword() {
    inProgress = true;
    badLogin = false;
    failed = false;
    try {
      await api.login(username, password, stayLoggedIn);
      location.reload();
    } catch (err) {
      if (err.status === 403) {
        badLogin = true;
      } else {
        failed = true;
        console.log('Password authentication failed:', err);
      }
    } finally {
      inProgress = false;
    }
  }
</script>

<div class="center-block">
  <h3>{$translations, t('Authentication Required')}</h3>

  <form onsubmit={(e) => { e.preventDefault(); authenticatePassword(); }}>
    <div class="form-group">
      <label for="user">{$translations, t('User')}</label>
      <input id="user" class="form-control" type="text" name="user" bind:value={username} autofocus required autocomplete="username" />
    </div>

    <div class="form-group">
      <label for="password">{$translations, t('Password')}</label>
      <input id="password" class="form-control" type="password" name="password" bind:value={password} autocomplete="current-password" />
    </div>

    <div class="form-group">
      <label>
        <input type="checkbox" id="stayLoggedIn" name="stayLoggedIn" bind:checked={stayLoggedIn} />&nbsp;{$translations, t('Stay logged in')}
      </label>
    </div>

    <div class="row">
      <div class="col-md-9 login-form-messages">
        {#if badLogin}
          <p class="text-danger">{$translations, t('Incorrect user name or password.')}</p>
        {/if}
        {#if failed}
          <p class="text-danger">{$translations, t('Login failed, see Syncthing logs for details.')}</p>
        {/if}
      </div>
      <div class="col-md-3 text-right">
        <button type="submit" class="btn btn-default" disabled={inProgress}>{$translations, t('Log In')}</button>
      </div>
    </div>
  </form>
</div>
