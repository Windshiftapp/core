<script>
  import { onMount } from 'svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { ssoStore } from '../stores/sso.svelte.js';
  import { capabilitiesStore } from '../stores/capabilities.svelte.js';

  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import Plus from '@lucide/svelte/icons/plus';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Save from '@lucide/svelte/icons/save';
  import X from '@lucide/svelte/icons/x';
  import Check from '@lucide/svelte/icons/check';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import AlertCircle from '@lucide/svelte/icons/alert-circle';
  import Settings from '@lucide/svelte/icons/settings';
  import Power from '@lucide/svelte/icons/power';
  import PowerOff from '@lucide/svelte/icons/power-off';
  import Link from '@lucide/svelte/icons/link';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import TestTube from '@lucide/svelte/icons/test-tube';
  import CheckCircle from '@lucide/svelte/icons/check-circle';
  import XCircle from '@lucide/svelte/icons/x-circle';
  import Button from '../components/Button.svelte';
  import CopyButton from '../components/CopyButton.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Text from '../components/Text.svelte';
  import Card from '../components/Card.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import PageHeader from '../layout/PageHeader.svelte';
  import { safeHref } from '../utils/sanitize';
  import DescriptionText from '../components/DescriptionText.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { publicBaseURL } from '../runtime/contextPath.js';
	import TextField from '../components/TextField.svelte';

  let providers = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  /** @type {{ id: string, name: string, slug: string, type: string, enabled: boolean, [key: string]: any } | null} */
  let editingProvider = $state(null);
  /** @type {{ success: boolean, error?: string } | null} */
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
    require_verified_email: true
  });

  let canAddProvider = $derived(providers.length === 0 || capabilitiesStore.has('sso.multi-provider'));

  /** @type {Record<string, string>} */
  let formErrors = $state({});

  // Attribute mapping is edited as individual fields and serialized to the JSON
  // blob the API stores. Empty fields fall back to the standard claim.
  const ATTRIBUTE_KEYS = ['email', 'email_verified', 'name', 'given_name', 'family_name', 'username'];

  function emptyAttributeMapping() {
    return { email: '', email_verified: '', name: '', given_name: '', family_name: '', username: '' };
  }

  /** @type {Record<string, string>} */
  let attributeMapping = $state(emptyAttributeMapping());

  function parseAttributeMapping(raw) {
    const fields = emptyAttributeMapping();
    if (!raw) return fields;
    try {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object') {
        for (const key of ATTRIBUTE_KEYS) {
          if (typeof parsed[key] === 'string') fields[key] = parsed[key];
        }
      }
    } catch {
      // A malformed blob is treated as unset; saving rewrites it.
    }
    return fields;
  }

  function serializeAttributeMapping() {
    const mapping = {};
    for (const key of ATTRIBUTE_KEYS) {
      const value = attributeMapping[key]?.trim();
      if (value) mapping[key] = value;
    }
    // Always send valid JSON: the API treats an empty string as "leave the
    // stored mapping unchanged", so "{}" is how the fields get cleared.
    return JSON.stringify(mapping);
  }

  function buildPayload() {
    return { ...formData, attribute_mapping: serializeAttributeMapping() };
  }

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
        providers = state.adminProviders || [];
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
      is_default: providers.length === 0,
      issuer_url: '',
      client_id: '',
      client_secret: '',
      scopes: 'openid email profile',
      auto_provision_users: false,
      require_verified_email: true
    };
    formErrors = {};
    attributeMapping = emptyAttributeMapping();
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
      require_verified_email: provider.require_verified_email !== false, // Default to true
    };
    formErrors = {};
    attributeMapping = parseAttributeMapping(provider.attribute_mapping);
    testResult = null;
    showEditModal = true;
  }

  function closeModals() {
    showCreateModal = false;
    showEditModal = false;
    editingProvider = null;
    testResult = null;
    formErrors = {};
  }

  const callbackUrl = () =>
    `${publicBaseURL()}/api/sso/callback/${formData.slug || 'slug'}`;

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
      await ssoStore.createProvider(buildPayload());
      closeModals();
      await loadProviders();
    } catch (err) {
      console.error('Failed to create SSO provider:', err);
      formErrors = {
        ...formErrors,
        submit: err.message || t('settings.sso.failedToCreate')
      };
    } finally {
      saving = false;
    }
  }

  async function handleUpdate() {
    if (!validateForm()) return;

    try {
      saving = true;
      error = null;
      await ssoStore.updateProvider(editingProvider.id, buildPayload());
      closeModals();
      await loadProviders();
    } catch (err) {
      console.error('Failed to update SSO provider:', err);
      formErrors = {
        ...formErrors,
        submit: err.message || t('settings.sso.failedToUpdate')
      };
    } finally {
      saving = false;
    }
  }

  async function deleteProvider(provider) {
    const ok = await confirm({
      title: t('settings.sso.deleteSsoProvider'),
      message: t('settings.sso.confirmDeleteProvider') + ' ' + provider.name + '?\n\n' + t('settings.sso.deleteWarning'),
      confirmText: t('settings.sso.deleteProvider'),
      variant: 'danger',
    });
    if (!ok) return;

    try {
      saving = true;
      error = null;
      await ssoStore.deleteProvider(provider.id);
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
  <PageHeader title={t('settings.sso.title')} subtitle={t('settings.sso.subtitle')} icon={KeyRound}>
    {#snippet actions()}
      {#if canAddProvider}
        <Button variant="primary" onclick={openCreateModal} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('sso', 'addProvider'), guard: () => !showCreateModal && !showEditModal }}>
          <Plus class="w-4 h-4 mr-2" />
          {t('settings.sso.addProvider')}
        </Button>
      {/if}
    {/snippet}
  </PageHeader>

  <!-- Error Message -->
  {#if error}
    <AlertBox type="error">{error}</AlertBox>
  {/if}

  <!-- Loading State -->
  {#if loading}
    <StateDisplay type="loading" />
  {:else if providers.length === 0}
    <Card variant="dashed" padding="spacious">
      <EmptyState
        icon={KeyRound}
        title={t('settings.sso.noProviderConfigured')}
        description={t('settings.sso.noProviderDescription')}
      >
        {#snippet action()}
          <Button variant="primary" onclick={openCreateModal}>
            <Plus class="w-4 h-4 mr-2" />
            {t('settings.sso.addProvider')}
          </Button>
        {/snippet}
      </EmptyState>
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
              <Button variant="danger" size="sm" onclick={() => deleteProvider(provider)}>
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
              <a href={safeHref(provider.issuer_url)} target="_blank" rel="noopener noreferrer" class="ml-2 hover:underline flex items-center gap-1 inline-flex" style="color: var(--ds-link);">
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
      {#if formErrors.submit}
        <AlertBox type="error">{formErrors.submit}</AlertBox>
      {/if}

      <div class="grid grid-cols-2 gap-4">
        <!-- Name -->
        <div>
          <TextField
            label={t('settings.sso.displayName')}
            id="name"
            labelColor="default"
            labelClass="mb-1"
            required
            type="text"
            size="small"
            inputClass={formErrors.name ? 'border-ds-border-danger' : ''}
            placeholder={t('settings.sso.displayNamePlaceholder')}
            bind:value={formData.name}
            oninput={handleNameChange}
          />
          {#if formErrors.name}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.name}</p>
          {/if}
        </div>

        <!-- Slug -->
        <div>
          <TextField
            label={t('settings.sso.slug')}
            id="slug"
            labelColor="default"
            labelClass="mb-1"
            required
            type="text"
            size="small"
            inputClass={formErrors.slug ? 'font-mono border-ds-border-danger' : 'font-mono'}
            placeholder={t('settings.sso.slugPlaceholder')}
            bind:value={formData.slug}
          />
          {#if formErrors.slug}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.slug}</p>
          {/if}
          <DescriptionText>{t('settings.sso.slugHelp')}: /api/sso/login/{formData.slug || 'slug'}</DescriptionText>
        </div>
      </div>

      <!-- Callback URL (read-only with copy button) -->
      <div>
        <Label color="default" class="mb-1">{t('settings.sso.callbackUrl')}</Label>
        <div class="flex items-center gap-2">
          <div class="flex-1 px-3 py-2 border rounded-md font-mono text-sm overflow-x-auto"
               style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);">
            {publicBaseURL()}/api/sso/callback/{formData.slug || 'slug'}
          </div>
          <CopyButton getText={callbackUrl} title={t('toast.copied')} />
        </div>
        <DescriptionText>
          {t('settings.sso.callbackUrlHelp')}
        </DescriptionText>
      </div>

      <!-- Issuer URL -->
      <div>
        <TextField
          label={t('settings.sso.issuerUrl')}
          id="issuer_url"
          labelColor="default"
          labelClass="mb-1"
          required
          type="url"
          size="small"
          inputClass={formErrors.issuer_url ? 'font-mono border-ds-border-danger' : 'font-mono'}
          placeholder={t('settings.sso.issuerUrlPlaceholder')}
          bind:value={formData.issuer_url}
        />
        {#if formErrors.issuer_url}
          <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.issuer_url}</p>
        {/if}
        <DescriptionText>{t('settings.sso.issuerUrlHelp')}</DescriptionText>
      </div>

      <div class="grid grid-cols-2 gap-4">
        <!-- Client ID -->
        <div>
          <TextField
            label={t('settings.sso.clientId')}
            id="client_id"
            labelColor="default"
            labelClass="mb-1"
            required
            type="text"
            size="small"
            inputClass={formErrors.client_id ? 'font-mono border-ds-border-danger' : 'font-mono'}
            placeholder={t('settings.sso.clientIdPlaceholder')}
            bind:value={formData.client_id}
          />
          {#if formErrors.client_id}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.client_id}</p>
          {/if}
        </div>

        <!-- Client Secret -->
        <div>
          <TextField
            label={t('settings.sso.clientSecret')}
            id="client_secret"
            labelColor="default"
            labelClass="mb-1"
            required={showCreateModal}
            type="password"
            size="small"
            inputClass={formErrors.client_secret ? 'border-ds-border-danger' : ''}
            placeholder={showEditModal ? t('settings.sso.leaveEmptyToKeepCurrent') : t('settings.sso.enterClientSecret')}
            bind:value={formData.client_secret}
          />
          {#if formErrors.client_secret}
            <p class="mt-1 text-sm" style="color: var(--ds-text-danger);">{formErrors.client_secret}</p>
          {/if}
        </div>
      </div>

      <!-- Scopes -->
      <div>
        <TextField
          label={t('settings.sso.scopes')}
          id="scopes"
          labelColor="default"
          type="text"
          size="small"
          placeholder="openid email profile"
          bind:value={formData.scopes}
        />
        <DescriptionText>{t('settings.sso.scopesHelp')}</DescriptionText>
      </div>

      <AlertBox type="info">
        {t('settings.sso.nameClaimsHelp')}
      </AlertBox>

      <!-- Attribute mapping (advanced) -->
      <div class="pt-4 border-t" style="border-color: var(--ds-border);">
        <h3 class="text-sm font-semibold mb-1" style="color: var(--ds-text);">
          {t('settings.sso.attributeMapping')}
        </h3>
        <DescriptionText class="mb-3">{t('settings.sso.attributeMappingHelp')}</DescriptionText>
        <div class="grid grid-cols-2 gap-4">
          <TextField
            label={t('settings.sso.attributeEmail')}
            id="attr_email"
            labelColor="default"
            labelClass="mb-1"
            type="text"
            size="small"
            inputClass="font-mono"
            placeholder="email"
            bind:value={attributeMapping.email}
          />
          <TextField
            label={t('settings.sso.attributeEmailVerified')}
            id="attr_email_verified"
            labelColor="default"
            labelClass="mb-1"
            type="text"
            size="small"
            inputClass="font-mono"
            placeholder="email_verified"
            bind:value={attributeMapping.email_verified}
          />
          <TextField
            label={t('settings.sso.attributeName')}
            id="attr_name"
            labelColor="default"
            labelClass="mb-1"
            type="text"
            size="small"
            inputClass="font-mono"
            placeholder="name"
            bind:value={attributeMapping.name}
          />
          <TextField
            label={t('settings.sso.attributeUsername')}
            id="attr_username"
            labelColor="default"
            labelClass="mb-1"
            type="text"
            size="small"
            inputClass="font-mono"
            placeholder="preferred_username"
            bind:value={attributeMapping.username}
          />
          <TextField
            label={t('settings.sso.attributeGivenName')}
            id="attr_given_name"
            labelColor="default"
            labelClass="mb-1"
            type="text"
            size="small"
            inputClass="font-mono"
            placeholder="given_name"
            bind:value={attributeMapping.given_name}
          />
          <TextField
            label={t('settings.sso.attributeFamilyName')}
            id="attr_family_name"
            labelColor="default"
            labelClass="mb-1"
            type="text"
            size="small"
            inputClass="font-mono"
            placeholder="family_name"
            bind:value={attributeMapping.family_name}
          />
        </div>
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
          bind:checked={formData.require_verified_email}
          label={t('settings.sso.trustIdpEmailVerification')}
          hint={t('settings.sso.trustIdpEmailVerificationDesc')}
          size="small"
        />

        {#if formData.require_verified_email}
          <AlertBox type="warning">
            <strong>{t('settings.sso.unsafeProvisioningWarningTitle')}</strong>
            <div class="mt-1">{t('settings.sso.unsafeProvisioningWarningBody')}</div>
          </AlertBox>
        {/if}
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
