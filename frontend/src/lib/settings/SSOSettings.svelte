<script>
  import { onMount } from 'svelte';
  import { ssoStore } from '../stores';
  import { api } from '../api.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import {
    KeyRound, Plus, Edit, Trash2, Save, X, Check, RefreshCw,
    AlertCircle, Settings, Power, PowerOff, Link, ExternalLink,
    TestTube, CheckCircle, XCircle, Copy
  } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Input from '../components/Input.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Text from '../components/Text.svelte';
  import Card from '../components/Card.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import { t } from '../stores/i18n.svelte.js';

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

  // Form state
  let formData = $state({
    slug: '',
    name: '',
    provider_type: 'oidc',
    enabled: true,
    is_default: true,
    issuer_url: '',
    client_id: '',
    client_secret: '',
    scopes: 'openid email profile',
    auto_provision_users: false,
    allow_password_login: true,
    require_verified_email: true,
    attribute_mapping: ''
  });

  let formErrors = $state({});

  // Load providers on mount
  onMount(async () => {
    await loadProviders();
  });

  async function loadProviders() {
    try {
      loading = true;
      error = null;
      await ssoStore.loadProviders();
      // Get providers from store
      ssoStore.subscribe(state => {
        providers = state.providers || [];
      })();
    } catch (err) {
      console.error('Failed to load SSO providers:', err);
      error = t('settings.sso.failedToLoad');
    } finally {
      loading = false;
    }
  }

  function openCreateModal() {
    formData = {
      slug: '',
      name: '',
      provider_type: 'oidc',
      enabled: true,
      is_default: true,
      issuer_url: '',
      client_id: '',
      client_secret: '',
      scopes: 'openid email profile',
      auto_provision_users: false,
      allow_password_login: true,
      require_verified_email: true,
      attribute_mapping: ''
    };
    formErrors = {};
    testResult = null;
    showCreateModal = true;
  }

  function openEditModal(provider) {
    editingProvider = provider;
    formData = {
      slug: provider.slug,
      name: provider.name,
      provider_type: provider.provider_type,
      enabled: provider.enabled,
      is_default: provider.is_default,
      issuer_url: provider.issuer_url || '',
      client_id: provider.client_id || '',
      client_secret: '', // Never pre-fill secret
      scopes: provider.scopes || 'openid email profile',
      auto_provision_users: provider.auto_provision_users,
      allow_password_login: provider.allow_password_login,
      require_verified_email: provider.require_verified_email !== false, // Default to true
      attribute_mapping: provider.attribute_mapping || ''
    };
    formErrors = {};
    testResult = null;
    showEditModal = true;
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

  function copyCallbackUrl() {
    const url = `${window.location.origin}/api/sso/callback/${formData.slug || 'slug'}`;
    navigator.clipboard.writeText(url);
  }

  function validateForm() {
    formErrors = {};

    if (!formData.slug.trim()) {
      formErrors.slug = t('settings.sso.slugRequired');
    } else if (!/^[a-z0-9-]+$/.test(formData.slug)) {
      formErrors.slug = t('settings.sso.slugInvalid');
    }

    if (!formData.name.trim()) {
      formErrors.name = t('settings.sso.nameRequired');
    }

    if (!formData.issuer_url.trim()) {
      formErrors.issuer_url = t('settings.sso.issuerUrlRequired');
    } else if (!formData.issuer_url.startsWith('https://') && !formData.issuer_url.startsWith('http://')) {
      formErrors.issuer_url = t('settings.sso.issuerUrlInvalid');
    }

    if (!formData.client_id.trim()) {
      formErrors.client_id = t('settings.sso.clientIdRequired');
    }

    // Client secret required for new providers
    if (showCreateModal && !formData.client_secret.trim()) {
      formErrors.client_secret = t('settings.sso.clientSecretRequired');
    }

    return Object.keys(formErrors).length === 0;
  }

  async function handleCreate() {
    if (!validateForm()) return;

    try {
      saving = true;
      error = null;
      await ssoStore.createProvider(formData);
      closeModals();
      await loadProviders();
    } catch (err) {
      console.error('Failed to create SSO provider:', err);
      error = err.message || t('settings.sso.failedToCreate');
    } finally {
      saving = false;
    }
  }

  async function handleUpdate() {
    if (!validateForm()) return;

    try {
      saving = true;
      error = null;
      await ssoStore.updateProvider(editingProvider.id, formData);
      closeModals();
      await loadProviders();
    } catch (err) {
      console.error('Failed to update SSO provider:', err);
      error = err.message || t('settings.sso.failedToUpdate');
    } finally {
      saving = false;
    }
  }

  async function handleDelete() {
    try {
      saving = true;
      error = null;
      await ssoStore.deleteProvider(deletingProvider.id);
      closeModals();
      await loadProviders();
    } catch (err) {
      console.error('Failed to delete SSO provider:', err);
      error = err.message || t('settings.sso.failedToDelete');
    } finally {
      saving = false;
    }
  }

  async function handleTest() {
    try {
      testLoading = true;
      testResult = null;
      error = null;

      const result = await ssoStore.testProvider(editingProvider.id);
      testResult = result;
    } catch (err) {
      console.error('Failed to test SSO provider:', err);
      testResult = { success: false, error: err.message || t('settings.sso.connectionTestFailed') };
    } finally {
      testLoading = false;
    }
  }

  function generateSlug(name) {
    return name.toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '');
  }

  function handleNameChange() {
    // Auto-generate slug from name if slug is empty or was auto-generated
    if (!formData.slug || formData.slug === generateSlug(formData.name.slice(0, -1))) {
      formData.slug = generateSlug(formData.name);
    }
  }
</script>

<div class="space-y-6">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <Text as="h2" size="lg" weight="semibold" class="flex items-center gap-2">
        <KeyRound class="w-5 h-5" style="color: var(--ds-icon-subtle);" />
        {t('settings.sso.title')}
      </Text>
      <Text as="p" size="sm" variant="subtle" class="mt-1">
        {t('settings.sso.subtitle')}
      </Text>
    </div>
    {#if providers.length === 0}
      <Button variant="primary" onclick={openCreateModal} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('sso', 'addProvider'), guard: () => !showCreateModal && !showEditModal && !showDeleteModal }}>
        <Plus class="w-4 h-4 mr-2" />
        {t('settings.sso.addProvider')}
      </Button>
    {/if}
  </div>

  <!-- Error Message -->
  {#if error}
    <AlertBox type="error">{error}</AlertBox>
  {/if}

  <!-- Loading State -->
  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Spinner />
    </div>
  {:else if providers.length === 0}
    <!-- Empty State -->
    <Card variant="dashed" padding="spacious" class="text-center">
      <KeyRound class="w-12 h-12 mx-auto mb-4" style="color: var(--ds-icon-subtle);" />
      <Text as="h3" size="lg" weight="medium" class="mb-2">{t('settings.sso.noProviderConfigured')}</Text>
      <Text as="p" size="sm" variant="subtle" class="mb-4 max-w-md mx-auto">
        {t('settings.sso.noProviderDescription')}
      </Text>
      <Button variant="primary" onclick={openCreateModal} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('sso', 'addProvider'), guard: () => !showCreateModal && !showEditModal && !showDeleteModal }}>
        <Plus class="w-4 h-4 mr-2" />
        {t('settings.sso.addProvider')}
      </Button>
    </Card>
  {:else}
    <!-- Provider Card -->
    {#each providers as provider}
      <Card shadow padding="spacious">
        <div class="flex items-start justify-between">
          <div class="flex items-center gap-3">
            <div class="p-2 rounded-lg" style="background-color: var(--ds-accent-blue-subtler);">
              <KeyRound class="w-6 h-6" style="color: var(--ds-accent-blue);" />
            </div>
            <div>
              <Text as="h3" size="lg" weight="semibold">{provider.name}</Text>
              <Text as="p" size="sm" variant="subtle">/{provider.slug}</Text>
            </div>
              {#if provider.enabled}
                <Lozenge color="green" size="md">
                  <Power class="w-3 h-3" />
                  {t('settings.sso.enabled')}
                </Lozenge>
              {:else}
                <Lozenge color="gray" size="md">
                  <PowerOff class="w-3 h-3" />
                  {t('settings.sso.disabled')}
                </Lozenge>
              {/if}
            </div>
            <div class="flex items-center gap-2">
              <Button variant="default" size="sm" onclick={() => openEditModal(provider)}>
                <Edit class="w-4 h-4 mr-1" />
                {t('common.edit')}
              </Button>
              <Button variant="danger" size="sm" onclick={() => openDeleteModal(provider)}>
                <Trash2 class="w-4 h-4 mr-1" />
                {t('common.delete')}
              </Button>
            </div>
          </div>

          <div class="mt-4 grid grid-cols-2 gap-4 text-sm">
            <div>
              <span style="color: var(--ds-text-subtle);">{t('settings.sso.providerType')}:</span>
              <span class="ml-2 font-medium uppercase" style="color: var(--ds-text);">{provider.provider_type}</span>
            </div>
            <div>
              <span style="color: var(--ds-text-subtle);">{t('settings.sso.issuerUrl')}:</span>
              <a href={provider.issuer_url} target="_blank" rel="noopener noreferrer" class="ml-2 hover:underline flex items-center gap-1 inline-flex" style="color: var(--ds-link);">
                {provider.issuer_url}
                <ExternalLink class="w-3 h-3" />
              </a>
            </div>
            <div>
              <span style="color: var(--ds-text-subtle);">{t('settings.sso.clientId')}:</span>
              <span class="ml-2 font-mono text-xs" style="color: var(--ds-text);">{provider.client_id}</span>
            </div>
            <div>
              <span style="color: var(--ds-text-subtle);">{t('settings.sso.clientSecret')}:</span>
              <span class="ml-2" style="color: var(--ds-text);">
                {provider.has_client_secret ? '••••••••' : t('settings.sso.notConfigured')}
              </span>
            </div>
          </div>

          <div class="mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
            <div class="flex flex-wrap items-center gap-6 text-sm">
              <div class="flex items-center gap-2">
                {#if provider.auto_provision_users}
                  <CheckCircle class="w-4 h-4" style="color: var(--ds-text-success);" />
                  <span style="color: var(--ds-text);">{t('settings.sso.autoProvisionUsers')}</span>
                {:else}
                  <XCircle class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                  <span style="color: var(--ds-text-subtle);">{t('settings.sso.manualUserCreationOnly')}</span>
                {/if}
              </div>
              <div class="flex items-center gap-2">
                {#if provider.allow_password_login}
                  <CheckCircle class="w-4 h-4" style="color: var(--ds-text-success);" />
                  <span style="color: var(--ds-text);">{t('settings.sso.passwordLoginAllowed')}</span>
                {:else}
                  <XCircle class="w-4 h-4" style="color: var(--ds-text-warning);" />
                  <span style="color: var(--ds-text-warning);">{t('settings.sso.ssoOnlyMode')}</span>
                {/if}
              </div>
              <div class="flex items-center gap-2">
                {#if provider.require_verified_email !== false}
                  <CheckCircle class="w-4 h-4" style="color: var(--ds-text-success);" />
                  <span style="color: var(--ds-text);">{t('settings.sso.trustIdpEmailVerification')}</span>
                {:else}
                  <XCircle class="w-4 h-4" style="color: var(--ds-text-warning);" />
                  <span style="color: var(--ds-text-warning);">{t('settings.sso.idpVerificationNotEnforced')}</span>
                {/if}
              </div>
            </div>
          </div>
      </Card>
    {/each}
  {/if}
</div>

<!-- Create/Edit Modal -->
{#if showCreateModal || showEditModal}
  <Modal
    isOpen={true}
    onclose={closeModals}
    maxWidth="max-w-2xl"
  >
    <ModalHeader
      title={showCreateModal ? t('settings.sso.addSsoProvider') : t('settings.sso.editSsoProvider')}
      icon={KeyRound}
      onClose={closeModals}
    />
    <form onsubmit={(e) => { e.preventDefault(); showCreateModal ? handleCreate() : handleUpdate(); }} class="p-6 space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <!-- Name -->
        <div>
          <Label for="name" color="default" required class="mb-1">{t('settings.sso.displayName')}</Label>
          <input
            type="text"
            id="name"
            bind:value={formData.name}
            oninput={handleNameChange}
            class="w-full px-3 py-2 border rounded-md focus:ring-2"
            class:border-red-500={formErrors.name}
            style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder={t('settings.sso.displayNamePlaceholder')}
          />
          {#if formErrors.name}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.name}</p>
          {/if}
        </div>

        <!-- Slug -->
        <div>
          <Label for="slug" color="default" required class="mb-1">{t('settings.sso.slug')}</Label>
          <input
            type="text"
            id="slug"
            bind:value={formData.slug}
            class="w-full px-3 py-2 border rounded-md focus:ring-2 font-mono"
            class:border-red-500={formErrors.slug}
            style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder={t('settings.sso.slugPlaceholder')}
          />
          {#if formErrors.slug}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.slug}</p>
          {/if}
          <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">{t('settings.sso.slugHelp')}: /api/sso/login/{formData.slug || 'slug'}</p>
        </div>
      </div>

      <!-- Callback URL (read-only with copy button) -->
      <div>
        <Label color="default" class="mb-1">{t('settings.sso.callbackUrl')}</Label>
        <div class="flex items-center gap-2">
          <div class="flex-1 px-3 py-2 border rounded-md font-mono text-sm overflow-x-auto"
               style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);">
            {window.location.origin}/api/sso/callback/{formData.slug || 'slug'}
          </div>
          <button
            type="button"
            class="p-2 rounded-md hover-bg"
            style="color: var(--ds-text-subtle);"
            onclick={copyCallbackUrl}
            title={t('toast.copied')}
          >
            <Copy class="w-4 h-4" />
          </button>
        </div>
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
          {t('settings.sso.callbackUrlHelp')}
        </p>
      </div>

      <!-- Issuer URL -->
      <div>
        <Label for="issuer_url" color="default" required class="mb-1">{t('settings.sso.issuerUrl')}</Label>
        <input
          type="url"
          id="issuer_url"
          bind:value={formData.issuer_url}
          class="w-full px-3 py-2 border rounded-md focus:ring-2 font-mono text-sm"
          class:border-red-500={formErrors.issuer_url}
          style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
          placeholder={t('settings.sso.issuerUrlPlaceholder')}
        />
        {#if formErrors.issuer_url}
          <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.issuer_url}</p>
        {/if}
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">{t('settings.sso.issuerUrlHelp')}</p>
      </div>

      <div class="grid grid-cols-2 gap-4">
        <!-- Client ID -->
        <div>
          <Label for="client_id" color="default" required class="mb-1">{t('settings.sso.clientId')}</Label>
          <input
            type="text"
            id="client_id"
            bind:value={formData.client_id}
            class="w-full px-3 py-2 border rounded-md focus:ring-2 font-mono"
            class:border-red-500={formErrors.client_id}
            style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder={t('settings.sso.clientIdPlaceholder')}
          />
          {#if formErrors.client_id}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.client_id}</p>
          {/if}
        </div>

        <!-- Client Secret -->
        <div>
          <Label for="client_secret" color="default" required={showCreateModal} class="mb-1">{t('settings.sso.clientSecret')}</Label>
          <input
            type="password"
            id="client_secret"
            bind:value={formData.client_secret}
            class="w-full px-3 py-2 border rounded-md focus:ring-2"
            class:border-red-500={formErrors.client_secret}
            style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder={showEditModal ? t('settings.sso.leaveEmptyToKeepCurrent') : t('settings.sso.enterClientSecret')}
          />
          {#if formErrors.client_secret}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.client_secret}</p>
          {/if}
        </div>
      </div>

      <!-- Scopes -->
      <div>
        <Label for="scopes" color="default" class="mb-1">{t('settings.sso.scopes')}</Label>
        <input
          type="text"
          id="scopes"
          bind:value={formData.scopes}
          class="w-full px-3 py-2 border rounded-md focus:ring-2"
          style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
          placeholder="openid email profile"
        />
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">{t('settings.sso.scopesHelp')}</p>
      </div>

      <!-- Checkboxes -->
      <div class="space-y-3 pt-2">
        <Checkbox
          bind:checked={formData.enabled}
          label={t('settings.sso.enableThisProvider')}
          hint={t('settings.sso.enableThisProviderDesc')}
          size="small"
        />

        <Checkbox
          bind:checked={formData.auto_provision_users}
          label={t('settings.sso.autoProvisionUsers')}
          hint={t('settings.sso.autoProvisionUsersDesc')}
          size="small"
        />

        <Checkbox
          bind:checked={formData.allow_password_login}
          label={t('settings.sso.allowPasswordLogin')}
          hint={t('settings.sso.allowPasswordLoginDesc')}
          size="small"
        />

        <Checkbox
          bind:checked={formData.require_verified_email}
          label={t('settings.sso.trustIdpEmailVerification')}
          hint={t('settings.sso.trustIdpEmailVerificationDesc')}
          size="small"
        />
      </div>

      <!-- Test Connection (only for edit) -->
      {#if showEditModal}
        <div class="pt-4 border-t" style="border-color: var(--ds-border);">
          <div class="flex items-center gap-3">
            <Button
              variant="default"
              onclick={handleTest}
              disabled={testLoading}
            >
              {#if testLoading}
                <RefreshCw class="w-4 h-4 mr-2 animate-spin" />
                {t('settings.sso.testing')}
              {:else}
                <TestTube class="w-4 h-4 mr-2" />
                {t('settings.sso.testConnection')}
              {/if}
            </Button>
            {#if testResult}
              {#if testResult.success}
                <span class="flex items-center text-sm" style="color: var(--ds-text-success);">
                  <CheckCircle class="w-4 h-4 mr-1" />
                  {t('settings.sso.connectionSuccessful')}
                </span>
              {:else}
                <span class="flex items-center text-sm" style="color: var(--ds-text-danger);">
                  <XCircle class="w-4 h-4 mr-1" />
                  {testResult.error || t('settings.sso.connectionFailed')}
                </span>
              {/if}
            {/if}
          </div>
        </div>
      {/if}

      <!-- Modal Actions -->
      <div class="flex justify-end gap-3 pt-4 border-t" style="border-color: var(--ds-border);">
        <Button variant="default" onclick={closeModals} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" type="submit" disabled={saving}>
          {#if saving}
            <Spinner class="w-4 h-4 mr-2" />
          {/if}
          {showCreateModal ? t('settings.sso.createProvider') : t('common.saveChanges')}
        </Button>
      </div>
    </form>
  </Modal>
{/if}

<!-- Delete Confirmation Modal -->
{#if showDeleteModal && deletingProvider}
  <Modal
    isOpen={true}
    onclose={closeModals}
    maxWidth="max-w-md"
  >
    <ModalHeader
      title={t('settings.sso.deleteSsoProvider')}
      icon={Trash2}
      onClose={closeModals}
    />
    <div class="p-6 space-y-4">
      <p style="color: var(--ds-text-subtle);">
        {t('settings.sso.confirmDeleteProvider')} <strong style="color: var(--ds-text);">{deletingProvider.name}</strong>?
      </p>
      <AlertBox type="warning">
        {t('settings.sso.deleteWarning')}
      </AlertBox>
      <div class="flex justify-end gap-3 pt-4">
        <Button variant="default" onclick={closeModals} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button variant="danger" onclick={handleDelete} disabled={saving}>
          {#if saving}
            <Spinner class="w-4 h-4 mr-2" />
          {/if}
          {t('settings.sso.deleteProvider')}
        </Button>
      </div>
    </div>
  </Modal>
{/if}
