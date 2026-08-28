<script>
  import { onMount } from 'svelte';
  import { Shield, Calendar, Image as ImageIcon, Loader2, Terminal, Key, Users, UserCog, AlertTriangle, ChevronDown, ChevronUp } from '@lucide/svelte';
  import { agentSecurity, getSecuritySettings, updateSecuritySettings, authPolicy } from '../api.js';
  import AgentSecurityAllowlistEditor from './AgentSecurityAllowlistEditor.svelte';
  import Toggle from '../components/Toggle.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Panel from '../components/Panel.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import ConfirmWithReasonDialog from '../dialogs/ConfirmWithReasonDialog.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import PageHeader from '../layout/PageHeader.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';

  let loading = $state(true);
  let saving = $state(false);

  let calendarFeedEnabled = $state(true);
  let pluginCliExecEnabled = $state(false);
  let allowExternalImages = $state(false);
  let allowUserManagedAgents = $state(false);
  let maxAgentsPerUser = $state(5);
  let workspaceManagedAgents = $state(true);
  let apiKeyCreationPolicy = $state('all_users');
  let apiKeyAllowedGroupIds = $state([]);

  // Coding-agent harness: allow workspace admins to bind runs to
  // centralized service users (WI-87). Flag changes need a reason —
  // the backend audit-logs every flip; the reason dialog enforces a
  // non-empty value before the PUT goes out.
  let allowCentralizedAgentUsers = $state(false);
  let savingAgentCentralized = $state(false);
  let loadingAgentCentralized = $state(true);
  let agentReasonDialogOpen = $state(false);
  let pendingAgentCentralizedValue = $state(false);

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

  const policyOptions = $derived([
    { value: 'password', label: t('securitySettings.policy.password'), description: t('securitySettings.policy.passwordDescription') },
    { value: 'password_passkey_2fa', label: t('securitySettings.policy.passwordPasskey'), description: t('securitySettings.policy.passwordPasskeyDescription'), requiresNoSSO: true },
    { value: 'passkey_only', label: t('securitySettings.policy.passkeyOnly'), description: t('securitySettings.policy.passkeyOnlyDescription') },
    { value: 'sso_primary', label: t('securitySettings.policy.ssoRequired'), description: t('securitySettings.policy.ssoRequiredDescription'), requiresSSO: true }
  ]);

  onMount(async () => {
    await Promise.all([loadSettings(), loadAuthPolicy(), loadAgentCentralized()]);
  });

  async function loadAgentCentralized() {
    loadingAgentCentralized = true;
    try {
      const settings = await agentSecurity.getSettings();
      allowCentralizedAgentUsers = !!settings.allow_centralized_service_users;
    } catch (err) {
      console.error('Failed to load agent security settings:', err);
    } finally {
      loadingAgentCentralized = false;
    }
  }

  function handleAgentCentralizedToggle(newValue) {
    // The Toggle has already updated allowCentralizedAgentUsers via
    // bind:checked. Snapshot the desired value, open the reason dialog;
    // cancel-path reverts the bound state to whatever the server still
    // has on file.
    pendingAgentCentralizedValue = newValue;
    agentReasonDialogOpen = true;
  }

  async function applyAgentCentralized(reason) {
    savingAgentCentralized = true;
    try {
      const result = await agentSecurity.updateSettings({
        allow_centralized_service_users: pendingAgentCentralizedValue,
        reason,
      });
      allowCentralizedAgentUsers = !!result.allow_centralized_service_users;
    } catch (err) {
      errorToast(err?.message || t('securitySettings.agentUpdateFailed'));
      console.error('Failed to update agent security:', err);
      // Force-refresh so the toggle reflects server truth.
      await loadAgentCentralized();
    } finally {
      savingAgentCentralized = false;
    }
  }

  async function cancelAgentCentralized() {
    // Toggle bound state has already flipped; reload so the UI shows
    // the server's truth instead of the un-confirmed user intent.
    await loadAgentCentralized();
  }

  async function loadSettings() {
    loading = true;
    try {
      const settings = await getSecuritySettings();
      calendarFeedEnabled = settings.calendar_feed_enabled ?? true;
      pluginCliExecEnabled = settings.plugin_cli_exec_enabled ?? false;
      allowExternalImages = settings.allow_external_images ?? false;
      allowUserManagedAgents = settings.allow_user_managed_agents ?? false;
      maxAgentsPerUser = settings.max_agents_per_user ?? 5;
      workspaceManagedAgents = settings.workspace_managed_agents ?? true;
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
        allow_external_images: allowExternalImages,
        allow_user_managed_agents: allowUserManagedAgents,
        max_agents_per_user: maxAgentsPerUser,
        workspace_managed_agents: workspaceManagedAgents,
        api_key_creation_policy: apiKeyCreationPolicy,
        api_key_allowed_group_ids: apiKeyAllowedGroupIds
      });
      return true;
    } catch (err) {
      errorToast(t('settings.security.failedToSave'));
      console.error('Failed to save settings:', err);
      return false;
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
      errorToast(err.message || t('securitySettings.policySaveFailed'));
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

  async function handleExternalImagesToggle(newValue) {
    allowExternalImages = newValue;
    if (!(await saveSettings())) {
      allowExternalImages = !newValue;
    }
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

  async function handleWorkspaceManagedAgentsToggle(newValue) {
    workspaceManagedAgents = newValue;
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
      return t('securitySettings.requiresSso');
    }
    if (option.requiresNoSSO && authPolicyConfig.sso_configured) {
      return t('securitySettings.notRecommendedWithSso');
    }
    return '';
  }

  function getPolicyLabel(option) {
    if (option.value === 'password' && authPolicyConfig.sso_configured) {
      return t('securitySettings.policy.passwordOrSso');
    }
    return option.label;
  }

  function getPolicyDescription(option) {
    if (option?.value === 'password' && authPolicyConfig.sso_configured) {
      return t('securitySettings.policy.passwordOrSsoDescription');
    }
    return option?.description || '';
  }
</script>

<div class="security-settings">
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
        <div class="min-w-0 flex-1">
          <div class="flex items-center justify-between gap-4">
            <div class="min-w-0">
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

    <!-- External Markdown Images -->
    <div class="mt-4">
      <Panel padding="spacious">
        <div class="flex items-start gap-4">
          <div class="p-2 rounded-lg" style="background-color: var(--ds-background-neutral);">
            <ImageIcon class="w-5 h-5" style="color: var(--ds-icon);" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center justify-between gap-4">
              <div class="min-w-0">
                <h3 class="text-base font-medium" style="color: var(--ds-text);">
                  {t('settings.security.externalImages')}
                </h3>
                <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                  {t('settings.security.externalImagesDesc')}
                </p>
                <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
                  {t('settings.security.externalImagesRefresh')}
                </p>
              </div>
              <Toggle
                bind:checked={allowExternalImages}
                ariaLabel={t('settings.security.externalImages')}
                dataTestid="external-images-toggle"
                disabled={saving}
                onchange={handleExternalImagesToggle}
              />
            </div>

            {#if allowExternalImages}
              <div class="mt-3">
                <AlertBox variant="warning" message={t('settings.security.externalImagesWarning')} />
              </div>
            {/if}
          </div>
        </div>
      </Panel>
    </div>

    <!-- Plugin CLI Execution Settings -->
    <div class="mt-4">
      <Panel padding="spacious">
        <div class="flex items-start gap-4">
          <div class="p-2 rounded-lg" style="background-color: var(--ds-background-neutral);">
            <Terminal class="w-5 h-5" style="color: var(--ds-icon);" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center justify-between gap-4">
              <div class="min-w-0">
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
          <div class="min-w-0 flex-1">
            <div class="flex items-center justify-between gap-4">
              <div class="min-w-0">
                <h3 class="text-base font-medium" style="color: var(--ds-text);">{t('securitySettings.workspaceAgents')}</h3>
                <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                  {t('securitySettings.workspaceAgentsDescription')}
                </p>
              </div>
              <Toggle
                bind:checked={workspaceManagedAgents}
                dataTestid="workspace-managed-agents-toggle"
                disabled={saving}
                onchange={handleWorkspaceManagedAgentsToggle}
              />
            </div>
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
          <div class="min-w-0 flex-1">
            <div class="flex items-center justify-between gap-4">
              <div class="min-w-0">
                <h3 class="text-base font-medium" style="color: var(--ds-text);">{t('securitySettings.userAgents')}</h3>
                <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                  {t('securitySettings.userAgentsDescription')}
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
                  {t('securitySettings.maxAgentsPerUser')}
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
                  {t('securitySettings.maxAgentsDescription')}
                </p>
              </div>
            {/if}
          </div>
        </div>
      </Panel>
    </div>

    <!-- Coding-Agent Harness: Centralized Service Users (WI-87) -->
    <div class="mt-4">
      <Panel padding="spacious">
        <div class="flex items-start gap-4">
          <div class="p-2 rounded-lg" style="background-color: var(--ds-background-neutral);">
            <UserCog class="w-5 h-5" style="color: var(--ds-icon);" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center justify-between gap-4">
              <div class="min-w-0">
                <h3 class="text-base font-medium" style="color: var(--ds-text);">{t('securitySettings.centralizedUsers')}</h3>
                <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                  {t('securitySettings.centralizedUsersDescription')}
                </p>
              </div>
              {#if loadingAgentCentralized}
                <Loader2 class="w-5 h-5 animate-spin" style="color: var(--ds-icon-subtle);" />
              {:else}
                <Toggle
                  bind:checked={allowCentralizedAgentUsers}
                  disabled={savingAgentCentralized}
                  onchange={handleAgentCentralizedToggle}
                />
              {/if}
            </div>

            {#if allowCentralizedAgentUsers}
              <div class="mt-3">
                <AlertBox variant="warning" message={t('securitySettings.centralizedUsersWarning')} />
              </div>
            {/if}

            <!-- Allowlist editor -->
            <div class="mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
              <AgentSecurityAllowlistEditor flagEnabled={allowCentralizedAgentUsers} />
            </div>
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
        <div class="min-w-0 flex-1">
          <div>
            <h3 class="text-base font-medium" style="color: var(--ds-text);">{t('securitySettings.authenticationPolicy')}</h3>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              {t('securitySettings.authenticationPolicyDescription')}
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
                {t('securitySettings.authenticationMethod')}
              </label>
              <Select
                id="auth-policy-select"
                value={authPolicyConfig.policy}
                onchange={(v) => handlePolicyChange({ target: { value: v } })}
                disabled={savingPolicy}
                options={policyOptions.map(o => ({
                  value: o.value,
                  label: isPolicyDisabled(o) ? `${getPolicyLabel(o)} (${getPolicyDisabledReason(o)})` : getPolicyLabel(o),
                  disabled: isPolicyDisabled(o)
                }))}
              />
              <DescriptionText>
                {getPolicyDescription(policyOptions.find(o => o.value === authPolicyConfig.policy))}
              </DescriptionText>
            </div>

            <!-- Preview Mode Toggle -->
            {#if authPolicyConfig.policy !== 'password'}
              <div class="mt-4 flex items-center justify-between">
                <div>
                  <span class="text-sm font-medium" style="color: var(--ds-text);">{t('securitySettings.previewMode')}</span>
                  <p class="text-xs" style="color: var(--ds-text-subtle);">
                    {t('securitySettings.previewModeDescription')}
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
                  <span class="text-sm font-medium" style="color: var(--ds-text);">{t('securitySettings.userStatistics')}</span>
                </div>
                <div class="grid grid-cols-2 gap-2 text-sm">
                  <div style="color: var(--ds-text-subtle);">{t('securitySettings.totalUsers')}:</div>
                  <div style="color: var(--ds-text);">{authPolicyStats.total_users}</div>
                  <div style="color: var(--ds-text-subtle);">{t('securitySettings.withPasskey')}:</div>
                  <div style="color: var(--ds-text);">{authPolicyStats.users_with_passkey}</div>
                  <div style="color: var(--ds-text-subtle);">{t('securitySettings.withoutPasskey')}:</div>
                  <div style="color: var(--ds-text);">{authPolicyStats.users_without_passkey}</div>
                  {#if authPolicyConfig.sso_configured}
                    <div style="color: var(--ds-text-subtle);">{t('securitySettings.ssoUsers')}:</div>
                    <div style="color: var(--ds-text);">{authPolicyStats.sso_users}</div>
                  {/if}
                  <div style="color: var(--ds-text-subtle);">{t('securitySettings.systemAdmins')}:</div>
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
                  {t('securitySettings.affectedUsers', { count: affectedUsers.length })}
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
                            <span class="text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-brand-bold); color: white;">{t('securitySettings.admin')}</span>
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
                <AlertBox variant="warning" class="security-fallback-alert">
                  <strong>{t('securitySettings.policyActive')}:</strong> {t('securitySettings.policyActiveDescription')}
                  {#if authPolicyConfig.fallback_enabled}
                    {t('securitySettings.adminFallbackShort')}
                  {/if}
                </AlertBox>
              </div>
            {/if}

            <!-- Admin Fallback Notice -->
            {#if authPolicyConfig.fallback_enabled}
              <div class="mt-4">
                <AlertBox variant="warning">
                  <strong>{t('securitySettings.fallbackEnabled')}:</strong> {t('securitySettings.fallbackEnabledDescription')}
                  {t('securitySettings.disableFallback')} <code class="px-1 py-0.5 rounded" style="background: var(--ds-background-neutral);">--enable-fallback</code>.
                </AlertBox>
              </div>
            {:else}
              <div class="mt-4">
                <AlertBox variant="success" class="security-fallback-alert">
                  <strong>{t('securitySettings.fallbackDisabled')}:</strong> {t('securitySettings.fallbackDisabledDescription')}
                  {t('securitySettings.enableFallback')} <code class="px-1 py-0.5 rounded" style="background: var(--ds-background-neutral);">--enable-fallback</code> {t('common.or')} <code class="px-1 py-0.5 rounded" style="background: var(--ds-background-neutral);">ENABLE_ADMIN_FALLBACK=true</code>.
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

<ConfirmWithReasonDialog
  bind:show={agentReasonDialogOpen}
  variant="warning"
  title={pendingAgentCentralizedValue ? t('securitySettings.enableCentralizedTitle') : t('securitySettings.disableCentralizedTitle')}
  message={pendingAgentCentralizedValue
    ? t('securitySettings.enableCentralizedMessage')
    : t('securitySettings.disableCentralizedMessage')}
  reasonLabel={t('securitySettings.reasonLabel')}
  reasonPlaceholder={t('securitySettings.reasonPlaceholder')}
  confirmText={pendingAgentCentralizedValue ? t('common.enable') : t('common.disable')}
  onconfirm={applyAgentCentralized}
  oncancel={cancelAgentCentralized}
/>

<style>
  .security-settings :global(.security-fallback-alert > .text-sm.flex-1) {
    min-width: 0;
    overflow-wrap: anywhere;
  }
</style>
