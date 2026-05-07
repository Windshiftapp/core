<script>
  import { onMount } from 'svelte';
  import { Shield, Calendar, Loader2, Terminal, Key, Users, AlertTriangle, ChevronDown, ChevronUp } from 'lucide-svelte';
  import { getSecuritySettings, updateSecuritySettings, authPolicy } from '../api.js';
  import Toggle from '../components/Toggle.svelte';
  import Input from '../components/Input.svelte';
  import Panel from '../components/Panel.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import PageHeader from '../layout/PageHeader.svelte';
  import Select from '../components/Select.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';

  let loading = $state(true);
  let saving = $state(false);

  let calendarFeedEnabled = $state(true);
  let pluginCliExecEnabled = $state(false);
  let allowUserManagedAgents = $state(false);
  let maxAgentsPerUser = $state(5);
  let apiKeyCreationPolicy = $state('all_users');
  let apiKeyAllowedGroupIds = $state([]);

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
    try {
      const settings = await getSecuritySettings();
      calendarFeedEnabled = settings.calendar_feed_enabled ?? true;
      pluginCliExecEnabled = settings.plugin_cli_exec_enabled ?? false;
      allowUserManagedAgents = settings.allow_user_managed_agents ?? false;
      maxAgentsPerUser = settings.max_agents_per_user ?? 5;
      apiKeyCreationPolicy = settings.api_key_creation_policy ?? 'all_users';
      apiKeyAllowedGroupIds = settings.api_key_allowed_group_ids ?? [];
    } catch (err) {
      errorToast(t('settings.security.failedToLoad'));
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
    try {
      await updateSecuritySettings({
        calendar_feed_enabled: calendarFeedEnabled,
        plugin_cli_exec_enabled: pluginCliExecEnabled,
        allow_user_managed_agents: allowUserManagedAgents,
        max_agents_per_user: maxAgentsPerUser,
        api_key_creation_policy: apiKeyCreationPolicy,
        api_key_allowed_group_ids: apiKeyAllowedGroupIds
      });
    } catch (err) {
      errorToast(t('settings.security.failedToSave'));
      console.error('Failed to save settings:', err);
    } finally {
      saving = false;
    }
  }

  async function saveAuthPolicy() {
    savingPolicy = true;
    try {
      await authPolicy.update({
        policy: authPolicyConfig.policy,
        preview_mode: authPolicyConfig.preview_mode
      });
      // Reload to get updated state
      await loadAuthPolicy();
    } catch (err) {
      errorToast(err.message || 'Failed to save authentication policy');
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

  async function handleUserManagedAgentsToggle(newValue) {
    allowUserManagedAgents = newValue;
    await saveSettings();
  }

  async function handleMaxAgentsBlur() {
    // Clamp in the UI too; the backend also clamps 0..1000.
    const n = Number(maxAgentsPerUser);
    if (!Number.isFinite(n) || n < 0) maxAgentsPerUser = 0;
    else if (n > 1000) maxAgentsPerUser = 1000;
    else maxAgentsPerUser = Math.floor(n);
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
  <PageHeader title={t('settings.security.title')} subtitle={t('settings.security.subtitle')} icon={Shield} />

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-icon-subtle);" />
    </div>
  {:else}
    <!-- Calendar Feed Settings -->
    <Panel padding="spacious">
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
            <div class="mt-3">
              <AlertBox variant="warning" message={t('settings.security.calendarFeedsWarning')} />
            </div>
          {/if}
        </div>
      </div>
    </Panel>

    <!-- Plugin CLI Execution Settings -->
    <div class="mt-4">
      <Panel padding="spacious">
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
              <div class="mt-3">
                <AlertBox variant="error" message={t('settings.security.pluginExecutionWarning')} />
              </div>
            {/if}
          </div>
        </div>
      </Panel>
    </div>

    <!-- User-Managed Agents Settings -->
    <div class="mt-4">
      <Panel padding="spacious">
        <div class="flex items-start gap-4">
          <div class="p-2 rounded-lg" style="background-color: var(--ds-background-neutral);">
            <Users class="w-5 h-5" style="color: var(--ds-icon);" />
          </div>
          <div class="flex-1">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="text-base font-medium" style="color: var(--ds-text);">User-Managed Agents</h3>
                <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                  Allow non-admin users to create their own agent users from their profile and mint API tokens for them. Agents inherit their owner's permissions at all times.
                </p>
              </div>
              <Toggle
                bind:checked={allowUserManagedAgents}
                disabled={saving}
                onchange={handleUserManagedAgentsToggle}
              />
            </div>

            {#if allowUserManagedAgents}
              <div class="mt-4">
                <label for="max-agents-per-user" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                  Max agents per user
                </label>
                <div class="w-32">
                  <Input
                    id="max-agents-per-user"
                    type="number"
                    min="0"
                    max="1000"
                    step="1"
                    bind:value={maxAgentsPerUser}
                    onblur={handleMaxAgentsBlur}
                    disabled={saving}
                  />
                </div>
                <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
                  Caps the number of agents each non-admin may own. Admin-created service users are not subject to this limit.
                </p>
              </div>
            {/if}
          </div>
        </div>
      </Panel>
    </div>

    <!-- Authentication Policy Settings -->
    <div class="mt-4">
      <Panel padding="spacious">
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
              <label for="auth-policy-select" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                Authentication Method
              </label>
              <Select
                id="auth-policy-select"
                value={authPolicyConfig.policy}
                onchange={(v) => handlePolicyChange({ target: { value: v } })}
                disabled={savingPolicy}
                options={policyOptions.map(o => ({
                  value: o.value,
                  label: isPolicyDisabled(o) ? `${o.label} (${getPolicyDisabledReason(o)})` : o.label,
                  disabled: isPolicyDisabled(o)
                }))}
              />
              <DescriptionText>
                {policyOptions.find(o => o.value === authPolicyConfig.policy)?.description}
              </DescriptionText>
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
              <div class="mt-4">
                <AlertBox variant="warning">
                  <strong>Policy Active:</strong> Users without the required authentication method will be prompted to enroll on their next login.
                  {#if authPolicyConfig.fallback_enabled}
                    System administrators have password fallback access (rate limited).
                  {/if}
                </AlertBox>
              </div>
            {/if}

            <!-- Admin Fallback Notice -->
            {#if authPolicyConfig.fallback_enabled}
              <div class="mt-4">
                <AlertBox variant="warning">
                  <strong>Fallback Enabled:</strong> System administrators can use password login as fallback (rate limited: 5/hour).
                  To disable, restart the server without <code class="px-1 py-0.5 rounded" style="background: var(--ds-background-neutral);">--enable-fallback</code>.
                </AlertBox>
              </div>
            {:else}
              <div class="mt-4">
                <AlertBox variant="success">
                  <strong>Fallback Disabled:</strong> System administrators must comply with the authentication policy.
                  To enable emergency fallback, restart the server with <code class="px-1 py-0.5 rounded" style="background: var(--ds-background-neutral);">--enable-fallback</code> or <code class="px-1 py-0.5 rounded" style="background: var(--ds-background-neutral);">ENABLE_ADMIN_FALLBACK=true</code>.
                </AlertBox>
              </div>
            {/if}
          {/if}
        </div>
      </div>
      </Panel>
    </div>
  {/if}
</div>
