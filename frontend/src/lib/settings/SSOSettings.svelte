<script>
  import { onMount } from 'svelte';
  import { ssoStore } from '../stores';
  import { api } from '../api.js';
  import { createShortcutHandler } from '../utils/keyboardShortcuts.js';
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

  let providers = [];
  let loading = true;
  let error = null;
  let showCreateModal = false;
  let showEditModal = false;
  let showDeleteModal = false;
  let editingProvider = null;
  let deletingProvider = null;
  let testResult = null;
  let testLoading = false;
  let saving = false;

  // Form state
  let formData = {
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

  let formErrors = {};

  // Keyboard shortcut handler
  const handleGlobalKeydown = createShortcutHandler({
    addProvider: () => {
      if (!showCreateModal && !showEditModal && !showDeleteModal) {
        openCreateModal();
      }
    }
  }, 'sso');

  // Load providers on mount
  onMount(async () => {
    await loadProviders();

    // Add global keyboard shortcut
    window.addEventListener('keydown', handleGlobalKeydown);

    return () => {
      window.removeEventListener('keydown', handleGlobalKeydown);
    };
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
      error = 'Failed to load SSO providers';
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
      formErrors.slug = 'Slug is required';
    } else if (!/^[a-z0-9-]+$/.test(formData.slug)) {
      formErrors.slug = 'Slug must contain only lowercase letters, numbers, and hyphens';
    }

    if (!formData.name.trim()) {
      formErrors.name = 'Name is required';
    }

    if (!formData.issuer_url.trim()) {
      formErrors.issuer_url = 'Issuer URL is required';
    } else if (!formData.issuer_url.startsWith('https://') && !formData.issuer_url.startsWith('http://')) {
      formErrors.issuer_url = 'Issuer URL must start with https:// or http://';
    }

    if (!formData.client_id.trim()) {
      formErrors.client_id = 'Client ID is required';
    }

    // Client secret required for new providers
    if (showCreateModal && !formData.client_secret.trim()) {
      formErrors.client_secret = 'Client Secret is required';
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
      error = err.message || 'Failed to create SSO provider';
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
      error = err.message || 'Failed to update SSO provider';
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
      error = err.message || 'Failed to delete SSO provider';
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
      testResult = { success: false, error: err.message || 'Connection test failed' };
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
        Single Sign-On (SSO)
      </Text>
      <Text as="p" size="sm" variant="subtle" class="mt-1">
        Configure OIDC identity providers for Single Sign-On authentication
      </Text>
    </div>
    {#if providers.length === 0}
      <Button variant="primary" on:click={openCreateModal} keyboardHint="A">
        <Plus class="w-4 h-4 mr-2" />
        Add Provider
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
      <Text as="h3" size="lg" weight="medium" class="mb-2">No SSO Provider Configured</Text>
      <Text as="p" size="sm" variant="subtle" class="mb-4 max-w-md mx-auto">
        Add an OIDC identity provider to enable Single Sign-On for your users.
        Supports Keycloak, Authentik, Pocket ID, and other OIDC-compliant providers.
      </Text>
      <Button variant="primary" on:click={openCreateModal} keyboardHint="A">
        <Plus class="w-4 h-4 mr-2" />
        Add Provider
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
                  Enabled
                </Lozenge>
              {:else}
                <Lozenge color="gray" size="md">
                  <PowerOff class="w-3 h-3" />
                  Disabled
                </Lozenge>
              {/if}
            </div>
            <div class="flex items-center gap-2">
              <Button variant="default" size="sm" on:click={() => openEditModal(provider)}>
                <Edit class="w-4 h-4 mr-1" />
                Edit
              </Button>
              <Button variant="danger" size="sm" on:click={() => openDeleteModal(provider)}>
                <Trash2 class="w-4 h-4 mr-1" />
                Delete
              </Button>
            </div>
          </div>

          <div class="mt-4 grid grid-cols-2 gap-4 text-sm">
            <div>
              <span style="color: var(--ds-text-subtle);">Provider Type:</span>
              <span class="ml-2 font-medium uppercase" style="color: var(--ds-text);">{provider.provider_type}</span>
            </div>
            <div>
              <span style="color: var(--ds-text-subtle);">Issuer URL:</span>
              <a href={provider.issuer_url} target="_blank" rel="noopener noreferrer" class="ml-2 hover:underline flex items-center gap-1 inline-flex" style="color: var(--ds-link);">
                {provider.issuer_url}
                <ExternalLink class="w-3 h-3" />
              </a>
            </div>
            <div>
              <span style="color: var(--ds-text-subtle);">Client ID:</span>
              <span class="ml-2 font-mono text-xs" style="color: var(--ds-text);">{provider.client_id}</span>
            </div>
            <div>
              <span style="color: var(--ds-text-subtle);">Client Secret:</span>
              <span class="ml-2" style="color: var(--ds-text);">
                {provider.has_client_secret ? '••••••••' : 'Not configured'}
              </span>
            </div>
          </div>

          <div class="mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
            <div class="flex flex-wrap items-center gap-6 text-sm">
              <div class="flex items-center gap-2">
                {#if provider.auto_provision_users}
                  <CheckCircle class="w-4 h-4" style="color: var(--ds-text-success);" />
                  <span style="color: var(--ds-text);">Auto-provision users</span>
                {:else}
                  <XCircle class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                  <span style="color: var(--ds-text-subtle);">Manual user creation only</span>
                {/if}
              </div>
              <div class="flex items-center gap-2">
                {#if provider.allow_password_login}
                  <CheckCircle class="w-4 h-4" style="color: var(--ds-text-success);" />
                  <span style="color: var(--ds-text);">Password login allowed</span>
                {:else}
                  <XCircle class="w-4 h-4" style="color: var(--ds-text-warning);" />
                  <span style="color: var(--ds-text-warning);">SSO-only mode</span>
                {/if}
              </div>
              <div class="flex items-center gap-2">
                {#if provider.require_verified_email !== false}
                  <CheckCircle class="w-4 h-4" style="color: var(--ds-text-success);" />
                  <span style="color: var(--ds-text);">Trust IdP email verification</span>
                {:else}
                  <XCircle class="w-4 h-4" style="color: var(--ds-text-warning);" />
                  <span style="color: var(--ds-text-warning);">IdP verification not enforced</span>
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
    on:close={closeModals}
    maxWidth="max-w-2xl"
  >
    <ModalHeader
      title={showCreateModal ? 'Add SSO Provider' : 'Edit SSO Provider'}
      icon={KeyRound}
      onClose={closeModals}
    />
    <form on:submit|preventDefault={showCreateModal ? handleCreate : handleUpdate} class="p-6 space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <!-- Name -->
        <div>
          <Label for="name" color="default" required class="mb-1">Display Name</Label>
          <input
            type="text"
            id="name"
            bind:value={formData.name}
            on:input={handleNameChange}
            class="w-full px-3 py-2 border rounded-md focus:ring-2"
            class:border-red-500={formErrors.name}
            style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder="e.g., Authentik, Keycloak"
          />
          {#if formErrors.name}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.name}</p>
          {/if}
        </div>

        <!-- Slug -->
        <div>
          <Label for="slug" color="default" required class="mb-1">Slug</Label>
          <input
            type="text"
            id="slug"
            bind:value={formData.slug}
            class="w-full px-3 py-2 border rounded-md focus:ring-2 font-mono"
            class:border-red-500={formErrors.slug}
            style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder="e.g., authentik"
          />
          {#if formErrors.slug}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.slug}</p>
          {/if}
          <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">Used in the SSO login URL: /api/sso/login/{formData.slug || 'slug'}</p>
        </div>
      </div>

      <!-- Callback URL (read-only with copy button) -->
      <div>
        <Label color="default" class="mb-1">Callback URL</Label>
        <div class="flex items-center gap-2">
          <div class="flex-1 px-3 py-2 border rounded-md font-mono text-sm overflow-x-auto"
               style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);">
            {window.location.origin}/api/sso/callback/{formData.slug || 'slug'}
          </div>
          <button
            type="button"
            class="p-2 rounded-md hover:bg-gray-100"
            style="color: var(--ds-text-subtle);"
            on:click={copyCallbackUrl}
            title="Copy to clipboard"
          >
            <Copy class="w-4 h-4" />
          </button>
        </div>
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
          Configure this URL as the redirect/callback URL in your identity provider
        </p>
      </div>

      <!-- Issuer URL -->
      <div>
        <Label for="issuer_url" color="default" required class="mb-1">Issuer URL</Label>
        <input
          type="url"
          id="issuer_url"
          bind:value={formData.issuer_url}
          class="w-full px-3 py-2 border rounded-md focus:ring-2 font-mono text-sm"
          class:border-red-500={formErrors.issuer_url}
          style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
          placeholder="https://auth.example.com/realms/myrealm"
        />
        {#if formErrors.issuer_url}
          <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.issuer_url}</p>
        {/if}
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">The OIDC issuer URL. For Keycloak: https://host/realms/realmname</p>
      </div>

      <div class="grid grid-cols-2 gap-4">
        <!-- Client ID -->
        <div>
          <Label for="client_id" color="default" required class="mb-1">Client ID</Label>
          <input
            type="text"
            id="client_id"
            bind:value={formData.client_id}
            class="w-full px-3 py-2 border rounded-md focus:ring-2 font-mono"
            class:border-red-500={formErrors.client_id}
            style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder="windshift"
          />
          {#if formErrors.client_id}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.client_id}</p>
          {/if}
        </div>

        <!-- Client Secret -->
        <div>
          <Label for="client_secret" color="default" required={showCreateModal} class="mb-1">Client Secret</Label>
          <input
            type="password"
            id="client_secret"
            bind:value={formData.client_secret}
            class="w-full px-3 py-2 border rounded-md focus:ring-2"
            class:border-red-500={formErrors.client_secret}
            style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
            placeholder={showEditModal ? '(leave empty to keep current)' : 'Enter client secret'}
          />
          {#if formErrors.client_secret}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.client_secret}</p>
          {/if}
        </div>
      </div>

      <!-- Scopes -->
      <div>
        <Label for="scopes" color="default" class="mb-1">Scopes</Label>
        <input
          type="text"
          id="scopes"
          bind:value={formData.scopes}
          class="w-full px-3 py-2 border rounded-md focus:ring-2"
          style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
          placeholder="openid email profile"
        />
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">Space-separated list of OIDC scopes</p>
      </div>

      <!-- Checkboxes -->
      <div class="space-y-3 pt-2">
        <label class="flex items-center gap-3">
          <input
            type="checkbox"
            bind:checked={formData.enabled}
            class="w-4 h-4 rounded"
            style="border-color: var(--ds-border);"
          />
          <span class="text-sm" style="color: var(--ds-text);">
            <span class="font-medium">Enable this provider</span>
            <span class="block text-xs" style="color: var(--ds-text-subtle);">Users can sign in using this SSO provider</span>
          </span>
        </label>

        <label class="flex items-center gap-3">
          <input
            type="checkbox"
            bind:checked={formData.auto_provision_users}
            class="w-4 h-4 rounded"
            style="border-color: var(--ds-border);"
          />
          <span class="text-sm" style="color: var(--ds-text);">
            <span class="font-medium">Auto-provision users</span>
            <span class="block text-xs" style="color: var(--ds-text-subtle);">Automatically create user accounts on first SSO login</span>
          </span>
        </label>

        <label class="flex items-center gap-3">
          <input
            type="checkbox"
            bind:checked={formData.allow_password_login}
            class="w-4 h-4 rounded"
            style="border-color: var(--ds-border);"
          />
          <span class="text-sm" style="color: var(--ds-text);">
            <span class="font-medium">Allow password login</span>
            <span class="block text-xs" style="color: var(--ds-text-subtle);">Users can still sign in with username/password. Disable for SSO-only mode.</span>
          </span>
        </label>

        <label class="flex items-center gap-3">
          <input
            type="checkbox"
            bind:checked={formData.require_verified_email}
            class="w-4 h-4 rounded"
            style="border-color: var(--ds-border);"
          />
          <span class="text-sm" style="color: var(--ds-text);">
            <span class="font-medium">Trust IdP email verification</span>
            <span class="block text-xs" style="color: var(--ds-text-subtle);">When enabled, blocks login if the IdP explicitly reports the email as unverified. When the IdP doesn't report verification status, we'll send a verification email.</span>
          </span>
        </label>
      </div>

      <!-- Test Connection (only for edit) -->
      {#if showEditModal}
        <div class="pt-4 border-t" style="border-color: var(--ds-border);">
          <div class="flex items-center gap-3">
            <Button
              variant="default"
              on:click={handleTest}
              disabled={testLoading}
            >
              {#if testLoading}
                <RefreshCw class="w-4 h-4 mr-2 animate-spin" />
                Testing...
              {:else}
                <TestTube class="w-4 h-4 mr-2" />
                Test Connection
              {/if}
            </Button>
            {#if testResult}
              {#if testResult.success}
                <span class="flex items-center text-sm" style="color: var(--ds-text-success);">
                  <CheckCircle class="w-4 h-4 mr-1" />
                  Connection successful
                </span>
              {:else}
                <span class="flex items-center text-sm" style="color: var(--ds-text-danger);">
                  <XCircle class="w-4 h-4 mr-1" />
                  {testResult.error || 'Connection failed'}
                </span>
              {/if}
            {/if}
          </div>
        </div>
      {/if}

      <!-- Modal Actions -->
      <div class="flex justify-end gap-3 pt-4 border-t" style="border-color: var(--ds-border);">
        <Button variant="default" on:click={closeModals} disabled={saving}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" disabled={saving}>
          {#if saving}
            <Spinner class="w-4 h-4 mr-2" />
          {/if}
          {showCreateModal ? 'Create Provider' : 'Save Changes'}
        </Button>
      </div>
    </form>
  </Modal>
{/if}

<!-- Delete Confirmation Modal -->
{#if showDeleteModal && deletingProvider}
  <Modal
    isOpen={true}
    on:close={closeModals}
    maxWidth="max-w-md"
  >
    <ModalHeader
      title="Delete SSO Provider"
      icon={Trash2}
      onClose={closeModals}
    />
    <div class="p-6 space-y-4">
      <p style="color: var(--ds-text-subtle);">
        Are you sure you want to delete the SSO provider <strong style="color: var(--ds-text);">{deletingProvider.name}</strong>?
      </p>
      <AlertBox type="warning">
        This will unlink all external accounts and users will need to sign in with a password.
      </AlertBox>
      <div class="flex justify-end gap-3 pt-4">
        <Button variant="default" on:click={closeModals} disabled={saving}>
          Cancel
        </Button>
        <Button variant="danger" on:click={handleDelete} disabled={saving}>
          {#if saving}
            <Spinner class="w-4 h-4 mr-2" />
          {/if}
          Delete Provider
        </Button>
      </div>
    </div>
  </Modal>
{/if}
