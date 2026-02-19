<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import {
    GitBranch, Plus, Edit, Trash2, X, Check, RefreshCw,
    AlertCircle, Settings, Power, PowerOff, Link, ExternalLink,
    TestTube, CheckCircle, XCircle, Github, Copy
  } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Input from '../components/Input.svelte';
  import FormField from '../components/FormField.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import WorkspacePicker from '../pickers/WorkspacePicker.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import PageHeader from '../layout/PageHeader.svelte';
  import Card from '../components/Card.svelte';

  // Workspace access options
  const workspaceAccessOptions = $derived([
    { value: 'unrestricted', label: t('settings.scmProviders.allWorkspaces') },
    { value: 'restricted', label: t('settings.scmProviders.restrictToWorkspaces') }
  ]);

  let providers = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let showDeleteModal = $state(false);
  let editingProvider = $state(null);
  let deletingProvider = $state(null);
  let testResult = $state(null);
  let testLoading = $state(false);
  let saving = $state(false);
  let oauthLoading = $state(false);

  // Workspace restriction state
  let allowedWorkspaceIds = $state([]);


  // Check for OAuth callback status in URL
  let oauthStatus = $state(null);
  let oauthMessage = $state(null);

  // Provider types (only GitHub and Gitea supported)
  const providerTypes = [
    { value: 'github', label: 'GitHub', icon: Github },
    { value: 'gitea', label: 'Gitea / Forgejo', icon: GitBranch },
  ];

  // Auth methods
  const authMethods = [
    { value: 'oauth', label: 'OAuth App', description: 'Users authorize via OAuth flow' },
    { value: 'pat', label: 'Personal Access Token', description: 'Use a PAT for all operations' },
    { value: 'github_app', label: 'GitHub App', description: 'Fine-grained permissions (GitHub only)' },
  ];

  // Filter auth methods based on provider type (GitHub App only for GitHub)
  const availableAuthMethods = $derived(authMethods.filter(m =>
    m.value !== 'github_app' || formData.provider_type === 'github'
  ));

  // Reset auth method if it becomes invalid for the selected provider
  $effect(() => {
    if (formData.auth_method === 'github_app' && formData.provider_type !== 'github') {
      formData.auth_method = 'oauth';
    }
  });

  // Default OAuth scopes per provider type
  const defaultScopes = {
    github: 'repo read:user user:email',
    gitea: 'read:user read:repository write:repository',
  };

  // Form state
  let formData = $state({
    slug: '',
    name: '',
    provider_type: 'github',
    auth_method: 'oauth',
    enabled: true,
    is_default: false,
    base_url: '',
    oauth_client_id: '',
    oauth_client_secret: '',
    personal_access_token: '',
    github_app_id: '',
    github_app_private_key: '',
    github_app_installation_id: '',
    github_org_id: null,
    scopes: defaultScopes.github,
    workspace_restriction_mode: 'unrestricted',
  });

  let formErrors = $state({});

  // GitHub App discovery state
  let discoveredInstallations = $state([]);
  let discoveringInstallations = $state(false);
  let discoveryError = $state(null);

  // Update scopes when provider type changes (only if scopes is a default value)
  $effect(() => {
    if (formData.provider_type) {
      const currentIsDefault = Object.values(defaultScopes).includes(formData.scopes);
      if (currentIsDefault) {
        formData.scopes = defaultScopes[formData.provider_type] || defaultScopes.github;
      }
    }
  });

  onMount(async () => {
    // Check for OAuth callback status
    const urlParams = new URLSearchParams(window.location.search);
    if (urlParams.get('oauth') === 'success') {
      oauthStatus = 'success';
      oauthMessage = `Successfully connected to ${urlParams.get('provider') || 'provider'}`;
      // Clean up URL
      window.history.replaceState({}, '', window.location.pathname);
    } else if (urlParams.get('oauth') === 'error') {
      oauthStatus = 'error';
      oauthMessage = urlParams.get('message') || 'OAuth connection failed';
      window.history.replaceState({}, '', window.location.pathname);
    }

    await loadProviders();
  });

  async function loadProviders() {
    try {
      loading = true;
      error = null;
      const response = await api.scmProviders.getAll();
      providers = response || [];
    } catch (err) {
      console.error('Failed to load SCM providers:', err);
      error = t('settings.scmProviders.failedToLoad');
    } finally {
      loading = false;
    }
  }

  async function openCreateModal() {
    formData = {
      slug: '',
      name: '',
      provider_type: 'github',
      auth_method: 'oauth',
      enabled: true,
      is_default: false,
      base_url: '',
      oauth_client_id: '',
      oauth_client_secret: '',
      personal_access_token: '',
      github_app_id: '',
      github_app_private_key: '',
      github_app_installation_id: '',
      github_org_id: null,
      scopes: defaultScopes.github,
      workspace_restriction_mode: 'unrestricted',
    };
    formErrors = {};
    testResult = null;
    allowedWorkspaceIds = [];
    // Reset discovery state
    discoveredInstallations = [];
    discoveringInstallations = false;
    discoveryError = null;
    showCreateModal = true;
  }

  async function openEditModal(provider) {
    editingProvider = provider;
    formData = {
      slug: provider.slug,
      name: provider.name,
      provider_type: provider.provider_type,
      auth_method: provider.auth_method,
      enabled: provider.enabled,
      is_default: provider.is_default,
      base_url: provider.base_url || '',
      oauth_client_id: provider.oauth_client_id || '',
      oauth_client_secret: '', // Never pre-fill secrets
      personal_access_token: '',
      github_app_id: provider.github_app_id || '',
      github_app_private_key: '',
      github_app_installation_id: provider.github_app_installation_id || '',
      github_org_id: provider.github_org_id || null,
      scopes: provider.scopes || 'repo',
      workspace_restriction_mode: provider.workspace_restriction_mode || 'unrestricted',
    };
    formErrors = {};
    testResult = null;
    // Reset discovery state
    discoveredInstallations = [];
    discoveringInstallations = false;
    discoveryError = null;
    showEditModal = true;
    await loadAllowedWorkspaces(provider.id);
  }

  function openDeleteModal(provider) {
    deletingProvider = provider;
    showDeleteModal = true;
  }

  function closeModals() {
    showCreateModal = false;
    showEditModal = false;
    showDeleteModal = false;
    editingProvider = null;
    deletingProvider = null;
    testResult = null;
    formErrors = {};
  }

  function validateForm() {
    formErrors = {};

    if (!formData.slug.trim()) {
      formErrors.slug = 'Slug is required';
    } else if (!/^[a-z0-9-]+$/.test(formData.slug)) {
      formErrors.slug = 'Slug must be lowercase alphanumeric with hyphens only';
    }

    if (!formData.name.trim()) {
      formErrors.name = 'Name is required';
    }

    // Validate based on auth method
    if (formData.auth_method === 'oauth') {
      if (!editingProvider && !formData.oauth_client_id) {
        formErrors.oauth_client_id = 'Client ID is required for OAuth';
      }
      if (!editingProvider && !formData.oauth_client_secret) {
        formErrors.oauth_client_secret = 'Client Secret is required for OAuth';
      }
    } else if (formData.auth_method === 'pat') {
      if (!editingProvider && !formData.personal_access_token) {
        formErrors.personal_access_token = 'Personal Access Token is required';
      }
    } else if (formData.auth_method === 'github_app') {
      if (!formData.github_app_id) {
        formErrors.github_app_id = 'App ID is required';
      }
      if (!editingProvider && !formData.github_app_private_key) {
        formErrors.github_app_private_key = 'Private Key is required';
      }
    }

    // Require base_url for Gitea
    if (formData.provider_type === 'gitea' && !formData.base_url) {
      formErrors.base_url = 'Base URL is required for self-hosted providers';
    }

    return Object.keys(formErrors).length === 0;
  }

  async function handleSubmit() {
    if (!validateForm()) return;

    saving = true;
    try {
      if (editingProvider) {
        await api.scmProviders.update(editingProvider.id, formData);
        // Update workspace allowlist if restriction mode is restricted
        if (formData.workspace_restriction_mode === 'restricted') {
          await api.scmProviders.updateAllowedWorkspaces(editingProvider.id, allowedWorkspaceIds);
        }
      } else {
        const newProvider = await api.scmProviders.create(formData);
        // Set workspace allowlist if restriction mode is restricted
        if (formData.workspace_restriction_mode === 'restricted' && allowedWorkspaceIds.length > 0) {
          await api.scmProviders.updateAllowedWorkspaces(newProvider.id, allowedWorkspaceIds);
        }
      }
      await loadProviders();
      closeModals();
    } catch (err) {
      console.error('Failed to save provider:', err);
      formErrors.submit = err.message || t('settings.scmProviders.failedToSave');
    } finally {
      saving = false;
    }
  }

  async function loadAllowedWorkspaces(providerId) {
    if (!providerId) {
      allowedWorkspaceIds = [];
      return;
    }
    try {
      const response = await api.scmProviders.getAllowedWorkspaces(providerId);
      allowedWorkspaceIds = (response || []).map(w => w.workspace_id);
    } catch (err) {
      console.error('Failed to load allowed workspaces:', err);
      allowedWorkspaceIds = [];
    }
  }

  async function handleDelete() {
    if (!deletingProvider) return;

    saving = true;
    try {
      await api.scmProviders.delete(deletingProvider.id);
      await loadProviders();
      closeModals();
    } catch (err) {
      console.error('Failed to delete provider:', err);
      error = t('settings.scmProviders.failedToDelete');
    } finally {
      saving = false;
    }
  }

  async function testConnection(providerId) {
    testLoading = true;
    testResult = null;
    try {
      const result = await api.scmProviders.test(providerId || editingProvider?.id);
      testResult = result;
      // If testing from list (not modal), show result as toast
      if (providerId && !showEditModal) {
        if (result.success) {
          successToast(result.message || t('settings.scmProviders.connectionSuccessful'), 'Test Result');
        } else {
          errorToast(result.error || t('settings.scmProviders.connectionFailed'), 'Test Result');
        }
      }
    } catch (err) {
      testResult = { success: false, error: err.message || 'Test failed' };
      if (providerId && !showEditModal) {
        errorToast(err.message || 'Connection test failed', 'Test Result');
      }
    } finally {
      testLoading = false;
    }
  }

  async function startOAuth(provider) {
    oauthLoading = true;
    try {
      const result = await api.scmProviders.startOAuth(provider.slug);
      if (result.auth_url) {
        window.location.href = result.auth_url;
      } else {
        error = 'Failed to get OAuth URL';
      }
    } catch (err) {
      error = err.message || 'Failed to start OAuth flow';
      oauthLoading = false;
    }
  }

  async function discoverGitHubAppInstallations() {
    if (!formData.github_app_id || !formData.github_app_private_key) {
      discoveryError = 'App ID and Private Key are required for discovery';
      return;
    }

    discoveringInstallations = true;
    discoveryError = null;
    discoveredInstallations = [];

    try {
      const result = await api.post('/admin/scm-providers/github-app/discover-installations', {
        app_id: formData.github_app_id,
        private_key: formData.github_app_private_key
      });

      if (result.success) {
        discoveredInstallations = result.installations || [];
        if (discoveredInstallations.length === 0) {
          discoveryError = 'No installations found. Make sure the GitHub App is installed on at least one organization.';
        } else if (discoveredInstallations.length === 1) {
          // Auto-select if only one installation
          selectInstallation(discoveredInstallations[0]);
        }
      } else {
        discoveryError = result.error || 'Failed to discover installations';
      }
    } catch (err) {
      discoveryError = err.message || 'Failed to discover installations';
    } finally {
      discoveringInstallations = false;
    }
  }

  function selectInstallation(installation) {
    formData.github_app_installation_id = String(installation.id);
    formData.github_org_id = installation.account_id;
    // Clear discovery state after selection
    discoveredInstallations = [];
    discoveryError = null;
  }

  function getProviderIcon(type) {
    const providerType = providerTypes.find(p => p.value === type);
    return providerType?.icon || GitBranch;
  }

  function getProviderLabel(type) {
    const providerType = providerTypes.find(p => p.value === type);
    return providerType?.label || type;
  }

  function dismissOAuthStatus() {
    oauthStatus = null;
    oauthMessage = null;
  }

  // Compute OAuth callback URL based on slug
  const oauthCallbackUrl = $derived(formData.slug
    ? `${window.location.origin}/api/scm/oauth/${formData.slug}/callback`
    : '');

  let callbackCopied = $state(false);
  async function copyCallbackUrl() {
    if (!oauthCallbackUrl) return;
    try {
      await navigator.clipboard.writeText(oauthCallbackUrl);
      callbackCopied = true;
      setTimeout(() => callbackCopied = false, 2000);
    } catch (err) {
      errorToast('Failed to copy to clipboard');
    }
  }

</script>

<div class="space-y-6">
  <!-- Header -->
  <PageHeader title={t('settings.scmProviders.title')} subtitle={t('settings.scmProviders.subtitle')} icon={GitBranch}>
    {#snippet actions()}
      <Button variant="primary" onclick={openCreateModal} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('scmProviders', 'addProvider'), guard: () => !showCreateModal && !showEditModal && !showDeleteModal }}>
        <Plus class="w-4 h-4 mr-2" />
        {t('settings.scmProviders.addProvider')}
      </Button>
    {/snippet}
  </PageHeader>

  <!-- OAuth Status Messages -->
  {#if oauthStatus}
    <AlertBox
      type={oauthStatus === 'success' ? 'success' : 'error'}
      dismissible
      ondismiss={dismissOAuthStatus}
    >
      {oauthMessage}
    </AlertBox>
  {/if}

  <!-- Error -->
  {#if error}
    <AlertBox type="error" dismissible ondismiss={() => error = null}>
      {error}
    </AlertBox>
  {/if}

  <!-- Loading -->
  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Spinner size="lg" />
    </div>
  {:else if providers.length === 0}
    <!-- Empty State -->
    <Card variant="dashed" class="text-center py-12">
      <GitBranch class="mx-auto h-12 w-12" style="color: var(--ds-text-subtlest);" />
      <h3 class="mt-2 text-sm font-medium" style="color: var(--ds-text);">{t('settings.scmProviders.noProviders')}</h3>
      <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
        {t('settings.scmProviders.getStarted')}
      </p>
      <div class="mt-4">
        <Button variant="primary" onclick={openCreateModal} keyboardHint="A">
          <Plus class="w-4 h-4 mr-2" />
          {t('settings.scmProviders.addProvider')}
        </Button>
      </div>
    </Card>
  {:else}
    <!-- Providers List -->
    <Card shadow padding="none" class="divide-y">
      {#each providers as provider}
        <div class="p-4 flex items-center justify-between" style="border-color: var(--ds-border);">
          <div class="flex items-center space-x-4">
            <div class="flex-shrink-0">
              <svelte:component this={getProviderIcon(provider.provider_type)} class="h-8 w-8" style="color: var(--ds-text-subtle);" />
            </div>
            <div>
              <div class="flex items-center space-x-2">
                <h3 class="text-sm font-medium" style="color: var(--ds-text);">{provider.name}</h3>
                {#if provider.enabled}
                  <Lozenge color="green">{t('settings.scmProviders.enabled')}</Lozenge>
                {:else}
                  <Lozenge color="gray">{t('settings.scmProviders.disabled')}</Lozenge>
                {/if}
                {#if provider.is_default}
                  <Lozenge color="blue">{t('settings.scmProviders.default')}</Lozenge>
                {/if}
                {#if provider.workspace_restriction_mode === 'restricted'}
                  <Lozenge color="purple">{t('settings.scmProviders.restricted')}</Lozenge>
                {/if}
              </div>
              <div class="flex items-center space-x-2 mt-1 text-sm" style="color: var(--ds-text-subtle);">
                <span>{getProviderLabel(provider.provider_type)}</span>
                <span>•</span>
                <span class="capitalize">{provider.auth_method}</span>
                {#if provider.base_url}
                  <span>•</span>
                  <span class="truncate max-w-xs">{provider.base_url}</span>
                {/if}
              </div>
              <!-- Connection Status -->
              <div class="mt-1 flex items-center space-x-2 text-xs">
                {#if provider.auth_method === 'oauth'}
                  {#if provider.has_oauth_token}
                    <span class="flex items-center" style="color: var(--ds-text-success);">
                      <CheckCircle class="w-3 h-3 mr-1" />
                      {t('settings.scmProviders.oauthConnected')}
                    </span>
                  {:else}
                    <span class="flex items-center" style="color: var(--ds-text-warning);">
                      <AlertCircle class="w-3 h-3 mr-1" />
                      {t('settings.scmProviders.oauthNotConnected')}
                    </span>
                  {/if}
                {:else if provider.auth_method === 'pat'}
                  {#if provider.has_pat}
                    <span class="flex items-center" style="color: var(--ds-text-success);">
                      <CheckCircle class="w-3 h-3 mr-1" />
                      {t('settings.scmProviders.patConfigured')}
                    </span>
                  {:else}
                    <span class="flex items-center" style="color: var(--ds-text-danger);">
                      <XCircle class="w-3 h-3 mr-1" />
                      {t('settings.scmProviders.patNotConfigured')}
                    </span>
                  {/if}
                {/if}
              </div>
            </div>
          </div>
          <div class="flex items-center space-x-2">
            {#if provider.auth_method === 'oauth' && !provider.has_oauth_token && provider.oauth_client_id}
              <Button
                variant="secondary"
                size="sm"
                onclick={() => startOAuth(provider)}
                disabled={oauthLoading}
              >
                {#if oauthLoading}
                  <Spinner size="sm" class="mr-1" />
                {:else}
                  <Link class="w-4 h-4 mr-1" />
                {/if}
                {t('settings.scmProviders.connect')}
              </Button>
            {/if}
            <Button variant="ghost" size="sm" onclick={() => testConnection(provider.id)} disabled={testLoading}>
              <TestTube class="w-4 h-4" />
            </Button>
            <Button variant="ghost" size="sm" onclick={() => openEditModal(provider)}>
              <Edit class="w-4 h-4" />
            </Button>
            <Button variant="ghost" size="sm" onclick={() => openDeleteModal(provider)}>
              <Trash2 class="w-4 h-4 text-red-500" />
            </Button>
          </div>
        </div>
      {/each}
    </Card>
  {/if}
</div>

<!-- Create/Edit Modal -->
<Modal isOpen={showCreateModal || showEditModal} onclose={closeModals} maxWidth="max-w-2xl">
    <ModalHeader title={showCreateModal ? t('settings.scmProviders.addSCMProvider') : t('settings.scmProviders.editSCMProvider')} onClose={closeModals} />

    <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="p-4 space-y-4">
      {#if formErrors.submit}
        <AlertBox type="error">{formErrors.submit}</AlertBox>
      {/if}

      <!-- Basic Info -->
      <div class="grid grid-cols-2 gap-4">
        <FormField label={t('settings.scmProviders.slug')} error={formErrors.slug} helper={t('settings.scmProviders.slugHelp')}>
          <Input
            bind:value={formData.slug}
            placeholder="github-main"
            disabled={!!editingProvider}
          />
        </FormField>
        <FormField label={t('settings.scmProviders.name')} error={formErrors.name}>
          <Input
            bind:value={formData.name}
            placeholder={t('settings.scmProviders.namePlaceholder')}
          />
        </FormField>
      </div>

      <!-- Provider Type & Auth Method -->
      <div class="grid grid-cols-2 gap-4">
        <div>
          <Label color="default" class="mb-1">{t('settings.scmProviders.providerType')}</Label>
          <BasePicker
            bind:value={formData.provider_type}
            items={providerTypes}
            placeholder="Select provider type"
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
          />
        </div>
        <div>
          <Label color="default" class="mb-1">{t('settings.scmProviders.authMethod')}</Label>
          <BasePicker
            bind:value={formData.auth_method}
            items={availableAuthMethods}
            placeholder="Select auth method"
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
          />
        </div>
      </div>

      <!-- Base URL (for self-hosted Gitea/Forgejo) -->
      {#if formData.provider_type === 'gitea'}
        <FormField label={t('settings.scmProviders.baseUrl')} error={formErrors.base_url} helper={t('settings.scmProviders.baseUrlPlaceholder')}>
          <Input
            bind:value={formData.base_url}
            placeholder="https://gitea.example.com"
          />
        </FormField>
      {/if}

      <!-- OAuth Settings -->
      {#if formData.auth_method === 'oauth'}
        <!-- Callback URL -->
        <div class="p-3 rounded-lg border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
          <div class="flex items-center justify-between mb-1">
            <label class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.scmProviders.callbackUrl')}</label>
            {#if oauthCallbackUrl}
              <button
                type="button"
                class="flex items-center text-xs px-2 py-1 rounded hover:bg-opacity-80 transition-colors"
                style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
                onclick={copyCallbackUrl}
              >
                {#if callbackCopied}
                  <Check class="w-3 h-3 mr-1" style="color: var(--ds-text-success);" />
                  {t('settings.scmProviders.copied')}
                {:else}
                  <Copy class="w-3 h-3 mr-1" />
                  {t('settings.scmProviders.copy')}
                {/if}
              </button>
            {/if}
          </div>
          {#if oauthCallbackUrl}
            <code class="block text-xs p-2 rounded break-all" style="background-color: var(--ds-background-input); color: var(--ds-text);">
              {oauthCallbackUrl}
            </code>
          {:else}
            <p class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.scmProviders.enterSlugForCallback')}</p>
          {/if}
          <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">
            {t('settings.scmProviders.useThisUrl')} {formData.provider_type === 'github' ? 'GitHub' : 'Gitea'}
          </p>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <FormField label={t('settings.scmProviders.oauthClientId')} error={formErrors.oauth_client_id}>
            <Input bind:value={formData.oauth_client_id} />
          </FormField>
          <FormField label={t('settings.scmProviders.oauthClientSecret')} error={formErrors.oauth_client_secret} helper={editingProvider ? t('settings.scmProviders.leaveEmptyToKeep') : ''}>
            <Input type="password" bind:value={formData.oauth_client_secret} />
          </FormField>
        </div>
      {/if}

      <!-- PAT Settings -->
      {#if formData.auth_method === 'pat'}
        <FormField label={t('settings.scmProviders.personalAccessToken')} error={formErrors.personal_access_token} helper={editingProvider ? t('settings.scmProviders.leaveEmptyToKeep') : ''}>
          <Input type="password" bind:value={formData.personal_access_token} />
        </FormField>
      {/if}

      <!-- GitHub App Settings -->
      {#if formData.auth_method === 'github_app'}
        <!-- Callback URL -->
        <div class="p-3 rounded-lg border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
          <div class="flex items-center justify-between mb-1">
            <label class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.scmProviders.callbackUrl')}</label>
            {#if oauthCallbackUrl}
              <button
                type="button"
                class="flex items-center text-xs px-2 py-1 rounded hover:bg-opacity-80 transition-colors"
                style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
                onclick={copyCallbackUrl}
              >
                {#if callbackCopied}
                  <Check class="w-3 h-3 mr-1" style="color: var(--ds-text-success);" />
                  {t('settings.scmProviders.copied')}
                {:else}
                  <Copy class="w-3 h-3 mr-1" />
                  {t('settings.scmProviders.copy')}
                {/if}
              </button>
            {/if}
          </div>
          {#if oauthCallbackUrl}
            <code class="block text-xs p-2 rounded break-all" style="background-color: var(--ds-background-input); color: var(--ds-text);">
              {oauthCallbackUrl}
            </code>
          {:else}
            <p class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.scmProviders.enterSlugForCallback')}</p>
          {/if}
          <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">
            {t('settings.scmProviders.useThisUrl')} {formData.provider_type === 'github' ? 'GitHub' : 'Gitea'}
          </p>
        </div>

        <FormField label={t('settings.scmProviders.githubAppId')} error={formErrors.github_app_id}>
          <Input bind:value={formData.github_app_id} />
        </FormField>

        <FormField label={t('settings.scmProviders.privateKeyPem')} error={formErrors.github_app_private_key} helper={editingProvider ? t('settings.scmProviders.leaveEmptyToKeep') : ''}>
          <textarea
            bind:value={formData.github_app_private_key}
            rows="4"
            class="shadow-sm focus:ring-blue-500 focus:border-blue-500 block w-full sm:text-sm rounded-md font-mono text-xs border"
            style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border);"
            placeholder="-----BEGIN RSA PRIVATE KEY-----"
          />
        </FormField>

        <!-- Installation Discovery -->
        <div class="space-y-3 p-3 rounded-lg border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
          <div class="flex items-center justify-between">
            <div>
              <h4 class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.scmProviders.orgInstallation')}</h4>
              <p class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">
                {t('settings.scmProviders.discoverInstallations')}
              </p>
            </div>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onclick={discoverGitHubAppInstallations}
              disabled={discoveringInstallations || !formData.github_app_id || (!formData.github_app_private_key && !editingProvider)}
            >
              {#if discoveringInstallations}
                <Spinner size="sm" class="mr-2" />
              {:else}
                <RefreshCw class="w-4 h-4 mr-2" />
              {/if}
              {t('settings.scmProviders.discoverButton')}
            </Button>
          </div>

          {#if discoveryError}
            <AlertBox type="error" size="sm">{discoveryError}</AlertBox>
          {/if}

          {#if discoveredInstallations.length > 0}
            <div class="space-y-2">
              <p class="text-xs" style="color: var(--ds-text-subtle);">Select an organization:</p>
              <div class="grid gap-2">
                {#each discoveredInstallations as installation}
                  <button
                    type="button"
                    class="flex items-center p-2 rounded border hover:border-blue-500 transition-colors text-left w-full"
                    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
                    onclick={() => selectInstallation(installation)}
                  >
                    {#if installation.account_avatar_url}
                      <img src={installation.account_avatar_url} alt="" class="w-8 h-8 rounded mr-3" />
                    {:else}
                      <div class="w-8 h-8 rounded mr-3 flex items-center justify-center" style="background-color: var(--ds-surface);">
                        <Github class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                      </div>
                    {/if}
                    <div>
                      <p class="text-sm font-medium" style="color: var(--ds-text);">{installation.account_login}</p>
                      <p class="text-xs" style="color: var(--ds-text-subtle);">{installation.account_type} • ID: {installation.id}</p>
                    </div>
                  </button>
                {/each}
              </div>
            </div>
          {/if}

          {#if formData.github_app_installation_id}
            <div class="flex items-center p-2 rounded" style="background-color: var(--ds-background-success-subtle);">
              <CheckCircle class="w-4 h-4 mr-2" style="color: var(--ds-text-success);" />
              <span class="text-sm" style="color: var(--ds-text);">
                {t('settings.scmProviders.orgInstallationId')} {formData.github_app_installation_id})
              </span>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Scopes (only for OAuth/PAT - GitHub Apps use permissions configured in GitHub) -->
      {#if formData.auth_method !== 'github_app'}
        <FormField label={t('settings.scmProviders.scopes')} helper={t('settings.scmProviders.scopesHelp')}>
          <Input bind:value={formData.scopes} placeholder="repo read:user user:email" />
        </FormField>
      {/if}

      <!-- Workspace Restrictions -->
      <div class="space-y-3">
        <div>
          <Label color="default" class="mb-1">{t('settings.scmProviders.workspaceAccess')}</Label>
          <BasePicker
            bind:value={formData.workspace_restriction_mode}
            items={workspaceAccessOptions}
            placeholder="Select access mode"
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
          />
        </div>

        {#if formData.workspace_restriction_mode === 'restricted'}
          <div class="space-y-2">
            <WorkspacePicker
              bind:value={allowedWorkspaceIds}
              placeholder="Select allowed workspaces..."
              label={t('settings.scmProviders.allowedWorkspaces')}
            />
            {#if allowedWorkspaceIds.length === 0}
              <div class="text-xs p-2 rounded" style="background-color: var(--ds-background-warning-subtle); color: var(--ds-text-warning);">
                {t('settings.scmProviders.noWorkspacesWarning')}
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Enabled & Default -->
      <div class="flex items-center space-x-6">
        <Checkbox
          bind:checked={formData.enabled}
          label={t('settings.scmProviders.enabled')}
          size="small"
        />
        <Checkbox
          bind:checked={formData.is_default}
          label={t('settings.scmProviders.default')}
          size="small"
        />
      </div>

      <!-- Test Result -->
      {#if testResult}
        <AlertBox type={testResult.success ? 'success' : 'error'}>
          {testResult.success ? testResult.message || t('settings.scmProviders.connectionSuccessful') : testResult.error}
        </AlertBox>
      {/if}

      <!-- Actions -->
      <div class="flex justify-between pt-4 border-t" style="border-color: var(--ds-border);">
        <div>
          {#if editingProvider}
            <Button
              type="button"
              variant="secondary"
              onclick={() => testConnection()}
              disabled={testLoading}
            >
              {#if testLoading}
                <Spinner size="sm" class="mr-2" />
              {:else}
                <TestTube class="w-4 h-4 mr-2" />
              {/if}
              Test Connection
            </Button>
          {/if}
        </div>
        <div class="flex space-x-2">
          <Button type="button" variant="secondary" onclick={closeModals}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" variant="primary" disabled={saving} keyboardHint="⏎">
            {#if saving}
              <Spinner size="sm" class="mr-2" />
            {/if}
            {showCreateModal ? t('common.create') : t('common.save')}
          </Button>
        </div>
      </div>
    </form>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal isOpen={showDeleteModal && deletingProvider} onclose={closeModals} maxWidth="max-w-md">
    <ModalHeader title={t('common.delete')} onClose={closeModals} />
    <div class="p-4">
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        {t('common.confirmDelete')} <strong style="color: var(--ds-text);">{deletingProvider?.name}</strong>?
      </p>
      <div class="mt-4 flex justify-end space-x-2">
        <Button variant="secondary" onclick={closeModals}>
          {t('common.cancel')}
        </Button>
        <Button variant="danger" onclick={handleDelete} disabled={saving}>
          {#if saving}
            <Spinner size="sm" class="mr-2" />
          {/if}
          {t('common.delete')}
        </Button>
      </div>
    </div>
</Modal>

