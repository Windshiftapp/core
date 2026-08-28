<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import {
    Plus, Edit, Trash2, TestTube, CheckCircle, Power, PowerOff, Star, AlertTriangle, Eye, EyeOff
  } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import DataTable from '../components/DataTable.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { llmConnectionTestErrorMessage } from './llmConnectionErrors.js';
  import { t } from '../stores/i18n.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  let connections = $state([]);
  let providers = $state([]);
  let actionCapabilities = $state([]);
  let loading = $state(true);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let editingConnection = $state(null);
  let testResult = $state(null);
  let testingConnectionId = $state(null);
  let saving = $state(false);

  // Form state
  let form = $state({
    name: '',
    provider_type: '',
    model: '',
    api_key: '',
    base_url: '',
    provider_config: '',
    vision_mode: 'auto',
    is_default: false,
    is_enabled: true,
  });

  function resetForm() {
    form = {
      name: '',
      provider_type: '',
      model: '',
      api_key: '',
      base_url: '',
      provider_config: '',
      vision_mode: 'auto',
      is_default: false,
      is_enabled: true,
    };
    testResult = null;
  }

  // vision_mode is stored inside the provider_config blob (a reserved key the
  // backend strips before forwarding to the provider). The form manages it via a
  // dedicated select, so we split it out of the raw JSON on edit and merge it
  // back on save — the textarea never shows or duplicates vision_mode.
  const VISION_MODES = ['auto', 'on', 'off'];

  function splitVisionMode(providerConfig) {
    let mode = 'auto';
    let rest = providerConfig || '';
    const raw = rest.trim();
    if (raw) {
      try {
        const obj = JSON.parse(raw);
        if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
          if (typeof obj.vision_mode === 'string' && VISION_MODES.includes(obj.vision_mode.toLowerCase())) {
            mode = obj.vision_mode.toLowerCase();
          }
          delete obj.vision_mode;
          rest = Object.keys(obj).length ? JSON.stringify(obj, null, 2) : '';
        }
      } catch {
        // Leave malformed JSON untouched for the user to fix in the textarea.
      }
    }
    return { mode, rest };
  }

  function composeProviderConfig() {
    /** @type {Record<string, any>} */
    let obj = {};
    const raw = form.provider_config?.trim();
    if (raw) {
      try { obj = JSON.parse(raw); } catch { obj = {}; }
    }
    if (form.vision_mode && form.vision_mode !== 'auto') {
      obj.vision_mode = form.vision_mode;
    } else {
      delete obj.vision_mode;
    }
    return Object.keys(obj).length ? JSON.stringify(obj) : '';
  }

  // Vision/cost helpers for the model picker + table.
  const modelById = $derived((id) => cachedModels.find((m) => m.id === id) || availableModels.find((m) => m.id === id) || null);

  function rateHint(model) {
    const p = model?.pricing;
    if (!p || (!p.prompt && !p.completion)) return '';
    const perM = (v) => `$${(Number(v) * 1_000_000).toFixed(2)}`;
    return t('settings.adminOperations.llmConnections.rateHint', { prompt: perM(p.prompt), completion: perM(p.completion) });
  }

  // connVision resolves a connection's effective vision capability client-side,
  // mirroring the backend (curated/catalog capability + the connection's
  // vision_mode override), for the per-row badge in the connections table.
  function connVision(conn) {
    const prov = providers.find((p) => p.type === conn.provider_type);
    const models = prov?.cached_models?.length ? prov.cached_models : (prov?.models || []);
    const base = !!models.find((m) => m.id === conn.model)?.supports_vision;
    const mode = splitVisionMode(conn.provider_config || '').mode;
    return mode === 'on' ? true : mode === 'off' ? false : base;
  }

  // Get models for the selected provider
  const selectedProvider = $derived(
    providers.find(p => p.type === form.provider_type)
  );
  const availableModels = $derived(
    selectedProvider?.models || []
  );
  const isLocalProvider = $derived(form.provider_type === 'local');
  const isDynamicProvider = $derived(!!selectedProvider?.models_endpoint);
  let refreshingModels = $state(false);
  const refreshModelsDisabled = $derived(refreshingModels || (isLocalProvider && !form.base_url.trim()));
  const cachedModels = $derived(selectedProvider?.cached_models || []);
  const lastRefreshedAt = $derived(selectedProvider?.last_refreshed_at || null);
  const lastRefreshError = $derived(selectedProvider?.last_error || '');

  function formatLastRefreshed(iso) {
    if (!iso) return t('settings.adminOperations.llmConnections.neverRefreshed');
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return t('settings.adminOperations.llmConnections.neverRefreshed');
    const diffMs = Date.now() - d.getTime();
    const mins = Math.round(diffMs / 60000);
    if (mins < 1) return t('settings.adminOperations.llmConnections.refreshedNow');
    if (mins < 60) return t('settings.adminOperations.llmConnections.refreshedMinutes', { count: mins });
    const hours = Math.round(mins / 60);
    if (hours < 24) return t('settings.adminOperations.llmConnections.refreshedHours', { count: hours });
    const days = Math.round(hours / 24);
    return t('settings.adminOperations.llmConnections.refreshedDays', { count: days });
  }

  async function refreshModels() {
    if (!selectedProvider || !isDynamicProvider || refreshModelsDisabled) return;
    refreshingModels = true;
    const refreshConnectionId = showEditModal && editingConnection?.provider_type === form.provider_type
      ? editingConnection.id
      : null;
    try {
      const result = await api.llmProviders.refreshModels(selectedProvider.type, {
        ...(refreshConnectionId ? { connection_id: refreshConnectionId } : {}),
        ...(form.base_url ? { base_url: form.base_url } : {}),
        ...(form.api_key ? { api_key: form.api_key } : {}),
      });
      successToast(t('settings.adminOperations.llmConnections.modelsFetched', { count: result.models?.length ?? 0, provider: selectedProvider.name }));
      await loadProviders();
    } catch (err) {
      const msg = err?.message || t('settings.adminOperations.llmConnections.refreshFailed');
      errorToast(msg);
      // Backend already recorded the failure in the cache; reload so last_error surfaces.
      await loadProviders();
    } finally {
      refreshingModels = false;
    }
  }

  async function loadConnections() {
    try {
      connections = await api.llmConnections.getAll();
    } catch (err) {
      console.error('Failed to load connections:', err);
      errorToast(t('settings.adminOperations.llmConnections.loadFailed'));
    }
  }

  async function loadProviders() {
    try {
      providers = await api.llmProviders.getProviders();
    } catch (err) {
      console.error('Failed to load providers:', err);
    }
  }

  async function loadActionCapabilities() {
    try {
      actionCapabilities = await api.actionCapabilities.getAll();
    } catch (err) {
      console.error('Failed to load action capabilities:', err);
      actionCapabilities = [];
    }
  }

  onMount(async () => {
    await Promise.all([loadConnections(), loadProviders(), loadActionCapabilities()]);
    loading = false;
  });

  function openCreate() {
    editingConnection = null;
    resetForm();
    showCreateModal = true;
  }

  function llmCapabilitiesForConnection(connectionId) {
    return actionCapabilities.filter((cap) => {
      if (cap.capability_type !== 'llm_connection') return false;
      try {
        const config = JSON.parse(cap.config || '{}');
        return Number(config.connection_id) === Number(connectionId);
      } catch {
        return false;
      }
    });
  }

  function enabledLLMCapabilitiesForConnection(connectionId) {
    return llmCapabilitiesForConnection(connectionId).filter((cap) => cap.is_enabled !== false);
  }

  async function reloadAfterConnectionChange() {
    await Promise.all([loadConnections(), loadActionCapabilities()]);
  }

  function capabilityUsageLabel(caps) {
    if (caps.length === 1) return t('settings.adminOperations.llmConnections.oneCapability', { name: caps[0].name });
    return t('settings.adminOperations.llmConnections.manyCapabilities', { count: caps.length, names: caps.map((cap) => cap.name).join(', ') });
  }

  function openEdit(conn) {
    editingConnection = conn;
    const { mode, rest } = splitVisionMode(conn.provider_config || '');
    form = {
      name: conn.name,
      provider_type: conn.provider_type,
      model: conn.model,
      api_key: '',
      base_url: conn.base_url || '',
      provider_config: rest,
      vision_mode: mode,
      is_default: conn.is_default,
      is_enabled: conn.is_enabled,
    };
    testResult = null;
    showEditModal = true;
  }

  async function deleteConnection(conn) {
    const impacted = enabledLLMCapabilitiesForConnection(conn.id);
    const ok = await confirm({
      title: t('settings.adminOperations.llmConnections.deleteTitle'),
      message: t('settings.adminOperations.llmConnections.deleteMessage', { name: conn.name }) + (impacted.length ? `\n\n${t('settings.adminOperations.llmConnections.deleteImpact', { capabilities: capabilityUsageLabel(impacted) })}` : ''),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.llmConnections.delete(conn.id);
      successToast(t('settings.adminOperations.llmConnections.deleted'));
      await reloadAfterConnectionChange();
    } catch (err) {
      errorToast(err.message || t('settings.adminOperations.llmConnections.deleteFailed'));
    }
  }

  function validateProviderConfig() {
    const raw = form.provider_config?.trim() || '';
    if (!raw) {
      form.provider_config = '';
      return true;
    }
    try {
      const parsed = JSON.parse(raw);
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
        errorToast(t('settings.adminOperations.llmConnections.configObject'));
        return false;
      }
      form.provider_config = raw;
      return true;
    } catch {
      errorToast(t('settings.adminOperations.llmConnections.configJson'));
      return false;
    }
  }

  async function handleCreate() {
    if (!validateProviderConfig()) return;
    saving = true;
    try {
      await api.llmConnections.create({ ...form, provider_config: composeProviderConfig() });
      successToast(t('settings.adminOperations.llmConnections.created'));
      showCreateModal = false;
      await reloadAfterConnectionChange();
    } catch (err) {
      errorToast(err.message || t('settings.adminOperations.llmConnections.createFailed'));
    } finally {
      saving = false;
    }
  }

  async function handleUpdate() {
    if (!editingConnection) return;
    if (!validateProviderConfig()) return;
    saving = true;
    try {
      await api.llmConnections.update(editingConnection.id, { ...form, provider_config: composeProviderConfig() });
      successToast(t('settings.adminOperations.llmConnections.updated'));
      showEditModal = false;
      await reloadAfterConnectionChange();
    } catch (err) {
      errorToast(err.message || t('settings.adminOperations.llmConnections.updateFailed'));
    } finally {
      saving = false;
    }
  }


  async function testConnection(id) {
    testingConnectionId = id;
    testResult = null;
    try {
      await api.llmConnections.test(id);
      testResult = { success: true, message: t('settings.adminOperations.llmConnections.connectionSuccessful') };
      successToast(t('settings.adminOperations.llmConnections.testPassed'));
    } catch (err) {
      errorToast(llmConnectionTestErrorMessage(err), t('settings.adminOperations.llmConnections.testFailed'));
    } finally {
      testingConnectionId = null;
    }
  }

  const columns = $derived([
    { key: 'name', label: t('common.name'), slot: 'name' },
    { key: 'provider_type', label: t('settings.adminOperations.llmConnections.provider'), textColor: 'var(--ds-text-subtle)' },
    { key: 'model', label: t('settings.adminOperations.llmConnections.model'), slot: 'model' },
    { key: 'is_enabled', label: t('common.status'), slot: 'status' },
    { key: 'actions', label: t('common.actions'), slot: 'actions', align: 'text-right', width: 'w-32' },
  ]);
</script>

<div class="space-y-4">
  <PageHeader title={t('settings.adminOperations.llmConnections.title')} subtitle={t('settings.adminOperations.llmConnections.subtitle')}>
    {#snippet actions()}
      <Button id="llm-connection-add" variant="primary" onclick={openCreate} icon={Plus} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('llmConnections', 'add') }}>
        {t('settings.adminOperations.llmConnections.addConnection')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Spinner />
    </div>
  {:else if connections.length === 0}
    <div class="flex flex-col items-center py-12 gap-3 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
      <p class="text-sm" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.llmConnections.empty')}</p>
      <Button variant="secondary" onclick={openCreate} icon={Plus} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('llmConnections', 'add') }}>
        {t('settings.adminOperations.llmConnections.addFirst')}
      </Button>
    </div>
  {:else}
    <DataTable {columns} data={connections} keyField="id">
      {#snippet name(conn)}
        <div class="flex items-center gap-2">
          <span class="font-medium" style="color: var(--ds-text);">{conn.name}</span>
          {#if conn.is_default}
            <Lozenge appearance="info" size="sm">{t('settings.adminOperations.llmConnections.default')}</Lozenge>
          {/if}
          {#if !conn.is_enabled && enabledLLMCapabilitiesForConnection(conn.id).length > 0}
            <Lozenge appearance="warning" size="sm">{t('settings.adminOperations.llmConnections.referenced')}</Lozenge>
          {/if}
        </div>
      {/snippet}
      {#snippet model(conn)}
        <div class="flex items-center gap-2">
          <span class="font-mono text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);">{conn.model}</span>
          {#if connVision(conn)}
            <span data-testid="connection-vision-badge" class="inline-flex items-center gap-1 text-xs" style="color: var(--ds-text-success);" title={t('settings.adminOperations.llmConnections.visionTitle')}>
              <Eye size={12} /> {t('settings.adminOperations.llmConnections.vision')}
            </span>
          {:else}
            <span data-testid="connection-vision-badge" class="inline-flex items-center gap-1 text-xs" style="color: var(--ds-text-subtle);" title={t('settings.adminOperations.llmConnections.noVisionTitle')}>
              <EyeOff size={12} /> {t('settings.adminOperations.llmConnections.noVision')}
            </span>
          {/if}
        </div>
      {/snippet}
      {#snippet status(conn)}
        {#if conn.is_enabled}
          <div class="flex items-center gap-1">
            <Power size={14} style="color: var(--ds-icon-success);" />
            <span class="text-xs" style="color: var(--ds-text-success);">{t('common.enabled')}</span>
          </div>
        {:else}
          <div class="flex items-center gap-1">
            <PowerOff size={14} style="color: var(--ds-text-subtle);" />
            <span class="text-xs" style="color: var(--ds-text-subtle);">{t('common.disabled')}</span>
          </div>
        {/if}
      {/snippet}
      {#snippet actions(conn)}
        <div class="flex items-center justify-end gap-1">
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-subtle);"
            title={t('settings.adminOperations.llmConnections.testConnection')}
            data-testid="llm-connection-test"
            disabled={testingConnectionId === conn.id}
            onclick={() => testConnection(conn.id)}
          >
            {#if testingConnectionId === conn.id}
              <Spinner size="sm" />
            {:else}
              <TestTube size={14} />
            {/if}
          </button>
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-subtle);"
            title={t('common.edit')}
            data-testid="llm-connection-edit"
            onclick={() => openEdit(conn)}
          >
            <Edit size={14} />
          </button>
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-danger);"
            title={t('common.delete')}
            data-testid="llm-connection-delete"
            onclick={() => deleteConnection(conn)}
          >
            <Trash2 size={14} />
          </button>
        </div>
      {/snippet}
    </DataTable>
  {/if}
</div>

<!-- Create Modal -->
{#if showCreateModal}
  <Modal
    isOpen={true}
    onclose={() => showCreateModal = false}
    onSubmit={handleCreate}
    submitDisabled={!form.name || !form.provider_type || !form.model || saving}
  >
    {#snippet children(submitHint)}
      <ModalHeader title={t('settings.adminOperations.llmConnections.addTitle')} onclose={() => showCreateModal = false} />
      <div class="p-4 space-y-4">
        {@render connectionForm()}
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={() => showCreateModal = false} keyboardHint="Esc">{t('common.cancel')}</Button>
          <Button id="llm-connection-create-submit" variant="primary" onclick={handleCreate} loading={saving} disabled={!form.name || !form.provider_type || !form.model} keyboardHint={submitHint}>
            {t('common.create')}
          </Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}

<!-- Edit Modal -->
{#if showEditModal}
  <Modal
    isOpen={true}
    onclose={() => showEditModal = false}
    onSubmit={handleUpdate}
    submitDisabled={!form.name || !form.provider_type || !form.model || saving}
  >
    {#snippet children(submitHint)}
      <ModalHeader title={t('settings.adminOperations.llmConnections.editTitle')} onclose={() => showEditModal = false} />
      <div class="p-4 space-y-4">
        {@render connectionForm()}

        {#if editingConnection && !form.is_enabled && enabledLLMCapabilitiesForConnection(editingConnection.id).length > 0}
          <div class="flex items-start gap-2 rounded-md border p-3 text-sm" style="border-color: var(--ds-border-warning, #f59e0b); background: var(--ds-background-warning-subtle, rgba(245, 158, 11, 0.12)); color: var(--ds-text-warning, #b45309);">
            <AlertTriangle size={16} class="mt-0.5 flex-shrink-0" />
            <div>
              <div class="font-medium">{t('settings.adminOperations.llmConnections.disableWarning')}</div>
              <div class="mt-1 text-xs">{t('settings.adminOperations.llmConnections.disableWarningHelp', { capabilities: capabilityUsageLabel(enabledLLMCapabilitiesForConnection(editingConnection.id)) })}</div>
            </div>
          </div>
        {/if}

        {#if editingConnection}
          <div class="flex items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
            <Button dataTestid="llm-connection-test-modal" variant="secondary" onclick={() => testConnection(editingConnection.id)} loading={testingConnectionId === editingConnection?.id} icon={TestTube}>
              {t('settings.adminOperations.llmConnections.testConnection')}
            </Button>
            {#if testResult}
              <div class="flex items-center gap-1 text-xs">
                <CheckCircle size={14} style="color: var(--ds-icon-success);" />
                <span style="color: var(--ds-text-success);">{testResult.message}</span>
              </div>
            {/if}
          </div>
        {/if}

        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={() => showEditModal = false} keyboardHint="Esc">{t('common.cancel')}</Button>
          <Button id="llm-connection-save-submit" variant="primary" onclick={handleUpdate} loading={saving} disabled={!form.name || !form.provider_type || !form.model} keyboardHint={submitHint}>
            {t('common.save')}
          </Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}


{#snippet connectionForm()}
  <!-- Name -->
  <div>
    <label for="llm-connection-name" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('common.name')}</label>
    <Input
      id="llm-connection-name"
      type="text"
      bind:value={form.name}
      placeholder={t('settings.adminOperations.llmConnections.namePlaceholder')}
      size="small"
    />
  </div>

  <!-- Provider Type -->
  <div>
    <label for="llm-connection-provider" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.llmConnections.provider')}</label>
    <Select
      id="llm-connection-provider"
      bind:value={form.provider_type}
      placeholder={t('settings.adminOperations.llmConnections.selectProvider')}
      options={providers.map(p => ({ value: p.type, label: p.name }))}
      onchange={() => { form.model = ''; form.base_url = ''; }}
    />
  </div>

  <!-- Base URL (only for local/custom) -->
  {#if isLocalProvider}
    <div>
      <label for="llm-connection-base-url" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.llmConnections.baseUrl')}</label>
      <Input
        id="llm-connection-base-url"
        type="text"
        bind:value={form.base_url}
        placeholder="e.g. http://localhost:11434 or http://localhost:11434/v1"
        size="small"
      />
    </div>
  {/if}

  <!-- API Key -->
  <div>
    <label for="llm-connection-api-key" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.llmConnections.apiKey')}</label>
    <Input
      id="llm-connection-api-key"
      type="password"
      bind:value={form.api_key}
      placeholder={editingConnection?.has_api_key ? t('settings.adminOperations.llmConnections.keyConfigured') : t('settings.adminOperations.llmConnections.enterApiKey')}
      size="small"
    />
  </div>

  <!-- Model -->
  <div>
    <label for="llm-connection-model" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.llmConnections.model')}</label>
    {#if isDynamicProvider}
      <div class="space-y-2">
        <BasePicker
          id="llm-connection-model"
          bind:value={form.model}
          items={cachedModels}
          loading={refreshingModels}
          placeholder={cachedModels.length ? t('settings.adminOperations.llmConnections.searchModels') : t('settings.adminOperations.llmConnections.typeModelId')}
          searchFields={['id', 'name']}
          getValue={(m) => m.id}
          getLabel={(m) => m.name || m.id}
          allowCreate={true}
          onCreate={(query) => { form.model = query.trim(); }}
        >
          {#snippet itemSnippet({ item })}
            <div class="flex items-center justify-between gap-2 w-full">
              <span class="truncate">{item.name || item.id}</span>
              <span class="flex items-center gap-2 shrink-0">
                {#if rateHint(item)}
                  <span class="text-xs" style="color: var(--ds-text-subtle);">{rateHint(item)}</span>
                {/if}
                {#if item.supports_vision}
                  <span class="inline-flex items-center gap-1 text-xs" style="color: var(--ds-text-success);" title={t('settings.adminOperations.llmConnections.supportsImages')}><Eye size={12} /></span>
                {:else}
                  <span class="inline-flex items-center gap-1 text-xs" style="color: var(--ds-text-subtle);" title={t('settings.adminOperations.llmConnections.textOnly')}><EyeOff size={12} /></span>
                {/if}
              </span>
            </div>
          {/snippet}
        </BasePicker>
        <div class="flex items-center justify-between text-xs" style="color: var(--ds-text-subtle);">
          <span>
            {#if lastRefreshError}
              <span style="color: var(--ds-text-danger);">{t('settings.adminOperations.llmConnections.lastAttemptFailed', { error: lastRefreshError })}</span>
            {:else}
              {formatLastRefreshed(lastRefreshedAt)}{cachedModels.length ? ` · ${t('settings.adminOperations.llmConnections.cachedCount', { count: cachedModels.length })}` : ''}
            {/if}
          </span>
          <Button variant="ghost" size="small" onclick={refreshModels} disabled={refreshModelsDisabled}>
            {refreshingModels ? t('settings.adminOperations.llmConnections.refreshing') : t('common.refresh')}
          </Button>
        </div>
        {#if !cachedModels.length && !lastRefreshError}
          <div class="text-xs" style="color: var(--ds-text-subtle);">
            {#if isLocalProvider && !form.base_url.trim()}
              {t('settings.adminOperations.llmConnections.enterBaseUrlHelp')}
            {:else}
              {t('settings.adminOperations.llmConnections.noCachedModels', { provider: selectedProvider?.name })}
            {/if}
          </div>
        {/if}
      </div>
    {:else}
      <Select
        id="llm-connection-model"
        bind:value={form.model}
        placeholder={t('settings.adminOperations.llmConnections.selectModel')}
        options={availableModels.map(m => ({ value: m.id, label: m.name + (m.supports_vision ? ` · ${t('settings.adminOperations.llmConnections.vision')}` : '') }))}
      />
    {/if}
  </div>

  <!-- Provider Config -->
  <div>
    <label for="llm-connection-provider-config" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.llmConnections.providerConfig')}</label>
    <Textarea
      id="llm-connection-provider-config"
      bind:value={form.provider_config}
      rows={5}
      spellcheck="false"
      placeholder={`{\n  "provider": {\n    "sort": "latency",\n    "allow_fallbacks": true\n  }\n}`}
      class="font-mono"
      size="small"
    />
    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
      {t('settings.adminOperations.llmConnections.providerConfigHelp')}
    </div>
  </div>

  <!-- Vision capability override -->
  <div>
    <label for="llm-connection-vision-mode" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.llmConnections.visionSupport')}</label>
    <Select
      id="llm-connection-vision-mode"
      bind:value={form.vision_mode}
      options={[
        { value: 'auto', label: t('settings.adminOperations.llmConnections.visionAuto') },
        { value: 'on', label: t('settings.adminOperations.llmConnections.visionOn') },
        { value: 'off', label: t('settings.adminOperations.llmConnections.visionOff') },
      ]}
    />
    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
      {t('settings.adminOperations.llmConnections.visionHelp')}
    </div>
  </div>

  <!-- Toggles -->
  <div class="flex items-center gap-6">
    <Checkbox bind:checked={form.is_default} label={t('settings.adminOperations.llmConnections.defaultConnection')} size="small" />
    <Checkbox bind:checked={form.is_enabled} label={t('common.enabled')} size="small" />
  </div>

{/snippet}
