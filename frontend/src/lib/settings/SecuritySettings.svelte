<script>
  import { onMount } from 'svelte';
  import { Shield, Calendar, Loader2, Terminal, Key, Users, AlertTriangle, ChevronDown, ChevronUp } from 'lucide-svelte';
  import { getSecuritySettings, updateSecuritySettings, authPolicy } from '../api.js';
  import Toggle from '../components/Toggle.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');

  let calendarFeedEnabled = $state(true);
  let pluginCliExecEnabled = $state(false);

  // Auth policy state
  let authPolicyConfig = $state({
    policy: 'password',
    preview_mode: false,
    sso_configured: false,
    fallback_enabled: false,
    hide_password_form: false
  });
  let authPolicyStats = $state(null);
  let affectedUsers = $state([]);
  let showAffectedUsers = $state(false);
  let loadingPolicy = $state(false);
  let savingPolicy = $state(false);

  const policyOptions = [
    { value: 'password', label: 'Password Only', description: 'Standard password authentication (default)' },
    { value: 'password_passkey_2fa', label: 'Password + Passkey 2FA', description: 'Password login followed by passkey verification', requiresNoSSO: true },
    { value: 'passkey_only', label: 'Passkey Only', description: 'Passkey authentication only (password for initial enrollment)' },
    { value: 'sso_primary', label: 'SSO Required', description: 'Single Sign-On only (password disabled)', requiresSSO: true }
  ];

  onMount(async () => {
    await Promise.all([loadSettings(), loadAuthPolicy()]);
  });

  async function loadSettings() {
    loading = true;
    error = '';
    try {
      const settings = await getSecuritySettings();
      calendarFeedEnabled = settings.calendar_feed_enabled ?? true;
      pluginCliExecEnabled = settings.plugin_cli_exec_enabled ?? false;
    } catch (err) {
      error = t('settings.security.failedToLoad');
      console.error('Failed to load security settings:', err);
    } finally {
      loading = false;
    }
  }

  async function loadAuthPolicy() {
    loadingPolicy = true;
    try {
      const [config, stats] = await Promise.all([
        authPolicy.get(),
        authPolicy.getStats()
      ]);
      authPolicyConfig = config;
      authPolicyStats = stats;

      // Load affected users if not password policy
      if (config.policy !== 'password') {
        affectedUsers = await authPolicy.getAffected();
      }
    } catch (err) {
      console.error('Failed to load auth policy:', err);
    } finally {
      loadingPolicy = false;
    }
  }

  async function saveSettings() {
    saving = true;
    error = '';
    try {
      await updateSecuritySettings({
        calendar_feed_enabled: calendarFeedEnabled,
        plugin_cli_exec_enabled: pluginCliExecEnabled
      });
    } catch (err) {
      error = t('settings.security.failedToSave');
      console.error('Failed to save settings:', err);
    } finally {
      saving = false;
    }
  }

  async function saveAuthPolicy() {
    savingPolicy = true;
    error = '';
    try {
      await authPolicy.update({
        policy: authPolicyConfig.policy,
        preview_mode: authPolicyConfig.preview_mode
      });
      // Reload to get updated state
      await loadAuthPolicy();
    } catch (err) {
      error = err.message || 'Failed to save authentication policy';
      console.error('Failed to save auth policy:', err);
    } finally {
      savingPolicy = false;
    }
  }

  async function handleCalendarToggle(newValue) {
    calendarFeedEnabled = newValue;
    await saveSettings();
  }

  async function handleCliExecToggle(newValue) {
    pluginCliExecEnabled = newValue;
    await saveSettings();
  }

  async function handlePolicyChange(event) {
    authPolicyConfig.policy = event.target.value;
    await saveAuthPolicy();
  }

  async function handlePreviewToggle(newValue) {
    authPolicyConfig.preview_mode = newValue;
    await saveAuthPolicy();
  }

  function isPolicyDisabled(option) {
    if (option.requiresSSO && !authPolicyConfig.sso_configured) return true;
    if (option.requiresNoSSO && authPolicyConfig.sso_configured) return true;
    return false;
  }

  function getPolicyDisabledReason(option) {
    if (option.requiresSSO && !authPolicyConfig.sso_configured) {
      return 'Requires SSO to be configured';
    }
    if (option.requiresNoSSO && authPolicyConfig.sso_configured) {
      return 'Not recommended when SSO is configured';
    }
    return '';
  }
</script>

<div>
  <div class="mb-6">
    <div class="flex items-center gap-3 mb-2">
      <Shield class="w-6 h-6" style="color: var(--ds-icon);" />
      <h2 class="text-2xl font-semibold" style="color: var(--ds-text);">{t('settings.security.title')}</h2>
    </div>
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      {t('settings.security.subtitle')}
    </p>
  </div>

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-icon-subtle);" />
    </div>
  {:else}
    {#if error}
      <div class="mb-4 p-3 rounded-md" style="background-color: var(--ds-background-danger-bold); color: white;">
        {error}
      </div>
    {/if}

    <!-- Calendar Feed Settings -->
    <div class="border rounded-lg p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <div class="flex items-start gap-4">
        <div class="p-2 rounded-lg" style="background-color: var(--ds-background-neutral);">
          <Calendar class="w-5 h-5" style="color: var(--ds-icon);" />
        </div>
        <div class="flex-1">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-base font-medium" style="color: var(--ds-text);">{t('settings.security.calendarFeeds')}</h3>
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                {t('settings.security.calendarFeedsDesc')}
              </p>
            </div>
            <Toggle
              bind:checked={calendarFeedEnabled}
              disabled={saving}
              onchange={handleCalendarToggle}
            />
          </div>

          {#if !calendarFeedEnabled}
            <div class="mt-3 px-4 py-3 rounded flex items-start gap-3" style="background: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-left: 4px solid var(--ds-icon-warning);">
              <AlertTriangle class="w-4 h-4 flex-shrink-0 mt-0.5" style="color: var(--ds-icon-warning);" />
              <span class="text-sm" style="color: var(--ds-text);">{t('settings.security.calendarFeedsWarning')}</span>
            </div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Plugin CLI Execution Settings -->
    <div class="border rounded-lg p-6 mt-4" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <div class="flex items-start gap-4">
        <div class="p-2 rounded-lg" style="background-color: var(--ds-background-neutral);">
          <Terminal class="w-5 h-5" style="color: var(--ds-icon);" />
        </div>
        <div class="flex-1">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-base font-medium" style="color: var(--ds-text);">{t('settings.security.pluginExecution')}</h3>
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                {t('settings.security.pluginExecutionDesc')}
              </p>
            </div>
            <Toggle
              bind:checked={pluginCliExecEnabled}
              disabled={saving}
              onchange={handleCliExecToggle}
            />
          </div>

          {#if pluginCliExecEnabled}
            <div class="mt-3 px-4 py-3 rounded flex items-start gap-3" style="background: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-left: 4px solid var(--ds-icon-danger);">
              <AlertTriangle class="w-4 h-4 flex-shrink-0 mt-0.5" style="color: var(--ds-icon-danger);" />
              <span class="text-sm" style="color: var(--ds-text);">{t('settings.security.pluginExecutionWarning')}</span>
            </div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Authentication Policy Settings -->
    <div class="border rounded-lg p-6 mt-4" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <div class="flex items-start gap-4">
        <div class="p-2 rounded-lg" style="background-color: var(--ds-background-neutral);">
          <Key class="w-5 h-5" style="color: var(--ds-icon);" />
        </div>
        <div class="flex-1">
          <div>
            <h3 class="text-base font-medium" style="color: var(--ds-text);">Authentication Policy</h3>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              Control how users authenticate to the application.
            </p>
          </div>

          {#if loadingPolicy}
            <div class="flex items-center justify-center py-4">
              <Loader2 class="w-5 h-5 animate-spin" style="color: var(--ds-icon-subtle);" />
            </div>
          {:else}
            <!-- Policy Selector -->
            <div class="mt-4">
              <label class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                Authentication Method
              </label>
              <select
                value={authPolicyConfig.policy}
                onchange={handlePolicyChange}
                disabled={savingPolicy}
                class="w-full px-3 py-2 border rounded-md text-sm"
                style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
              >
                {#each policyOptions as option}
                  <option
                    value={option.value}
                    disabled={isPolicyDisabled(option)}
                  >
                    {option.label}
                    {#if isPolicyDisabled(option)}
                      ({getPolicyDisabledReason(option)})
                    {/if}
                  </option>
                {/each}
              </select>
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                {policyOptions.find(o => o.value === authPolicyConfig.policy)?.description}
              </p>
            </div>

            <!-- Preview Mode Toggle -->
            {#if authPolicyConfig.policy !== 'password'}
              <div class="mt-4 flex items-center justify-between">
                <div>
                  <span class="text-sm font-medium" style="color: var(--ds-text);">Preview Mode</span>
                  <p class="text-xs" style="color: var(--ds-text-subtle);">
                    See affected users without enforcing the policy
                  </p>
                </div>
                <Toggle
                  bind:checked={authPolicyConfig.preview_mode}
                  disabled={savingPolicy}
                  onchange={handlePreviewToggle}
                />
              </div>
            {/if}

            <!-- Statistics -->
            {#if authPolicyStats}
              <div class="mt-4 p-3 rounded-md" style="background-color: var(--ds-background-neutral);">
                <div class="flex items-center gap-2 mb-2">
                  <Users class="w-4 h-4" style="color: var(--ds-icon);" />
                  <span class="text-sm font-medium" style="color: var(--ds-text);">User Statistics</span>
                </div>
                <div class="grid grid-cols-2 gap-2 text-sm">
                  <div style="color: var(--ds-text-subtle);">Total users:</div>
                  <div style="color: var(--ds-text);">{authPolicyStats.total_users}</div>
                  <div style="color: var(--ds-text-subtle);">With passkey:</div>
                  <div style="color: var(--ds-text);">{authPolicyStats.users_with_passkey}</div>
                  <div style="color: var(--ds-text-subtle);">Without passkey:</div>
                  <div style="color: var(--ds-text);">{authPolicyStats.users_without_passkey}</div>
                  {#if authPolicyConfig.sso_configured}
                    <div style="color: var(--ds-text-subtle);">SSO users:</div>
                    <div style="color: var(--ds-text);">{authPolicyStats.sso_users}</div>
                  {/if}
                  <div style="color: var(--ds-text-subtle);">System admins:</div>
                  <div style="color: var(--ds-text);">{authPolicyStats.system_admins}</div>
                </div>
              </div>
            {/if}

            <!-- Affected Users (when not password policy) -->
            {#if authPolicyConfig.policy !== 'password' && affectedUsers.length > 0}
              <div class="mt-4">
                <button
                  type="button"
                  onclick={() => showAffectedUsers = !showAffectedUsers}
                  class="flex items-center gap-2 text-sm font-medium"
                  style="color: var(--ds-text);"
                >
                  <AlertTriangle class="w-4 h-4" style="color: var(--ds-icon-warning);" />
                  {affectedUsers.length} users will need to enroll
                  {#if showAffectedUsers}
                    <ChevronUp class="w-4 h-4" />
                  {:else}
                    <ChevronDown class="w-4 h-4" />
                  {/if}
                </button>

                {#if showAffectedUsers}
                  <div class="mt-2 max-h-48 overflow-y-auto border rounded-md" style="border-color: var(--ds-border);">
                    {#each affectedUsers as user}
                      <div class="px-3 py-2 border-b last:border-b-0 text-sm" style="border-color: var(--ds-border);">
                        <div style="color: var(--ds-text);">{user.full_name || user.username}</div>
                        <div style="color: var(--ds-text-subtle);" class="text-xs">{user.email}</div>
                        <div class="flex gap-2 mt-1">
                          {#if user.is_admin}
                            <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-brand-bold); color: white;">Admin</span>
                          {/if}
                          {#if user.has_sso}
                            <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">SSO</span>
                          {/if}
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Policy Enforcement Notice -->
            {#if authPolicyConfig.policy !== 'password' && !authPolicyConfig.preview_mode}
              <div class="mt-4 px-4 py-3 rounded flex items-start gap-3" style="background: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-left: 4px solid var(--ds-icon-warning);">
                <AlertTriangle class="w-4 h-4 flex-shrink-0 mt-0.5" style="color: var(--ds-icon-warning);" />
                <div class="text-sm" style="color: var(--ds-text);">
                  <strong>Policy Active:</strong> Users without the required authentication method will be prompted to enroll on their next login.
                  {#if authPolicyConfig.fallback_enabled}
                    System administrators have password fallback access (rate limited).
                  {/if}
                </div>
              </div>
            {/if}

            <!-- Admin Fallback Notice -->
            {#if authPolicyConfig.fallback_enabled}
              <div class="mt-4 px-4 py-3 rounded flex items-start gap-3" style="background: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-left: 4px solid var(--ds-icon-warning);">
                <Key class="w-4 h-4 flex-shrink-0 mt-0.5" style="color: var(--ds-icon-warning);" />
                <div class="text-sm" style="color: var(--ds-text);">
                  <strong>Fallback Enabled:</strong> System administrators can use password login as fallback (rate limited: 5/hour).
                  To disable, restart the server without <code class="px-1 py-0.5 rounded" style="background: var(--ds-background-neutral);">--enable-fallback</code>.
                </div>
              </div>
            {:else}
              <div class="mt-4 px-4 py-3 rounded flex items-start gap-3" style="background: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-left: 4px solid var(--ds-icon-success);">
                <Key class="w-4 h-4 flex-shrink-0 mt-0.5" style="color: var(--ds-icon-success);" />
                <div class="text-sm" style="color: var(--ds-text);">
                  <strong>Fallback Disabled:</strong> System administrators must comply with the authentication policy.
                  To enable emergency fallback, restart the server with <code class="px-1 py-0.5 rounded" style="background: var(--ds-background-neutral);">--enable-fallback</code> or <code class="px-1 py-0.5 rounded" style="background: var(--ds-background-neutral);">ENABLE_ADMIN_FALLBACK=true</code>.
                </div>
              </div>
            {/if}
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>
