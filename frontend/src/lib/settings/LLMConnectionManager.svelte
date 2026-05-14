<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import {
    Plus, Edit, Trash2, TestTube, CheckCircle, XCircle, Power, PowerOff, Star, AlertTriangle
  } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import DataTable from '../components/DataTable.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import Select from '../components/Select.svelte';
  import { confirm } from '../composables/useConfirm.js';

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
      is_default: false,
      is_enabled: true,
    };
    testResult = null;
  }

  // Get models for the selected provider
  const selectedProvider = $derived(
    providers.find(p => p.type === form.provider_type)
  );
  const availableModels = $derived(
    selectedProvider?.models || []
  );
  const isLocalProvider = $derived(form.provider_type === 'local');

  async function loadConnections() {
    try {
      connections = await api.llmConnections.getAll();
    } catch (err) {
      console.error('Failed to load connections:', err);
      errorToast('Failed to load AI connections');
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

  function capabilityUsageLabel(caps) {
    if (caps.length === 1) return `1 enabled action capability: ${caps[0].name}`;
    return `${caps.length} enabled action capabilities: ${caps.map((cap) => cap.name).join(', ')}`;
  }

  function openEdit(conn) {
    editingConnection = conn;
    form = {
      name: conn.name,
      provider_type: conn.provider_type,
      model: conn.model,
      api_key: '',
      base_url: conn.base_url || '',
      is_default: conn.is_default,
      is_enabled: conn.is_enabled,
    };
    testResult = null;
    showEditModal = true;
  }

  async function deleteConnection(conn) {
    const impacted = enabledLLMCapabilitiesForConnection(conn.id);
    const ok = await confirm({
      title: 'Delete AI Connection',
      message: 'Are you sure you want to delete ' + conn.name + '? This action cannot be undone.' + (impacted.length ? `\n\nThis connection is referenced by ${capabilityUsageLabel(impacted)}. Those capabilities will stop working.` : ''),
      confirmText: 'Delete',
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.llmConnections.delete(conn.id);
      successToast('AI connection deleted');
      await Promise.all([loadConnections(), loadActionCapabilities()]);
    } catch (err) {
      errorToast(err.message || 'Failed to delete connection');
    }
  }

  async function handleCreate() {
    saving = true;
    try {
      await api.llmConnections.create(form);
      successToast('AI connection created');
      showCreateModal = false;
      await Promise.all([loadConnections(), loadActionCapabilities()]);
    } catch (err) {
      errorToast(err.message || 'Failed to create connection');
    } finally {
      saving = false;
    }
  }

  async function handleUpdate() {
    if (!editingConnection) return;
    saving = true;
    try {
      await api.llmConnections.update(editingConnection.id, form);
      successToast('AI connection updated');
      showEditModal = false;
      await Promise.all([loadConnections(), loadActionCapabilities()]);
    } catch (err) {
      errorToast(err.message || 'Failed to update connection');
    } finally {
      saving = false;
    }
  }


  async function testConnection(id) {
    testingConnectionId = id;
    testResult = null;
    try {
      await api.llmConnections.test(id);
      testResult = { success: true, message: 'Connection successful' };
      successToast('Connection test passed');
    } catch (err) {
      testResult = { success: false, message: err.message || 'Connection test failed' };
      errorToast(err.message || 'Connection test failed');
    } finally {
      testingConnectionId = null;
    }
  }

  const columns = [
    { key: 'name', label: 'Name', slot: 'name' },
    { key: 'provider_type', label: 'Provider', textColor: 'var(--ds-text-subtle)' },
    { key: 'model', label: 'Model', slot: 'model' },
    { key: 'is_enabled', label: 'Status', slot: 'status' },
    { key: 'actions', label: 'Actions', slot: 'actions', align: 'text-right', width: 'w-32' },
  ];
</script>

<div class="space-y-4">
  <PageHeader title="AI Connections" subtitle="Configure AI model providers for intelligent features">
    {#snippet actions()}
      <Button variant="primary" onclick={openCreate} icon={Plus}>
        Add Connection
      </Button>
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Spinner />
    </div>
  {:else if connections.length === 0}
    <div class="flex flex-col items-center py-12 gap-3 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
      <p class="text-sm" style="color: var(--ds-text-subtle);">No AI connections configured yet.</p>
      <Button variant="secondary" onclick={openCreate} icon={Plus}>
        Add your first connection
      </Button>
    </div>
  {:else}
    <DataTable {columns} data={connections} keyField="id">
      {#snippet name(conn)}
        <div class="flex items-center gap-2">
          <span class="font-medium" style="color: var(--ds-text);">{conn.name}</span>
          {#if conn.is_default}
            <Lozenge appearance="info" size="sm">Default</Lozenge>
          {/if}
          {#if !conn.is_enabled && enabledLLMCapabilitiesForConnection(conn.id).length > 0}
            <Lozenge appearance="warning" size="sm">Referenced by enabled capabilities</Lozenge>
          {/if}
        </div>
      {/snippet}
      {#snippet model(conn)}
        <span class="font-mono text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);">{conn.model}</span>
      {/snippet}
      {#snippet status(conn)}
        {#if conn.is_enabled}
          <div class="flex items-center gap-1">
            <Power size={14} style="color: var(--ds-icon-success);" />
            <span class="text-xs" style="color: var(--ds-text-success);">Enabled</span>
          </div>
        {:else}
          <div class="flex items-center gap-1">
            <PowerOff size={14} style="color: var(--ds-text-subtle);" />
            <span class="text-xs" style="color: var(--ds-text-subtle);">Disabled</span>
          </div>
        {/if}
      {/snippet}
      {#snippet actions(conn)}
        <div class="flex items-center justify-end gap-1">
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-subtle);"
            title="Test connection"
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
            title="Edit"
            onclick={() => openEdit(conn)}
          >
            <Edit size={14} />
          </button>
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-danger);"
            title="Delete"
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
      <ModalHeader title="Add AI Connection" onclose={() => showCreateModal = false} />
      <div class="p-4 space-y-4">
        {@render connectionForm()}
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={() => showCreateModal = false} keyboardHint="Esc">Cancel</Button>
          <Button variant="primary" onclick={handleCreate} loading={saving} disabled={!form.name || !form.provider_type || !form.model} keyboardHint={submitHint}>
            Create
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
      <ModalHeader title="Edit AI Connection" onclose={() => showEditModal = false} />
      <div class="p-4 space-y-4">
        {@render connectionForm()}

        {#if editingConnection && !form.is_enabled && enabledLLMCapabilitiesForConnection(editingConnection.id).length > 0}
          <div class="flex items-start gap-2 rounded-md border p-3 text-sm" style="border-color: var(--ds-border-warning, #f59e0b); background: var(--ds-background-warning-subtle, rgba(245, 158, 11, 0.12)); color: var(--ds-text-warning, #b45309);">
            <AlertTriangle size={16} class="mt-0.5 flex-shrink-0" />
            <div>
              <div class="font-medium">Disabling this connection will disable dependent LLM action capabilities at runtime.</div>
              <div class="mt-1 text-xs">Referenced by {capabilityUsageLabel(enabledLLMCapabilitiesForConnection(editingConnection.id))}. The capabilities themselves will still appear enabled, but actions using them will fail until the connection is re-enabled or the capability is repointed.</div>
            </div>
          </div>
        {/if}

        {#if editingConnection}
          <div class="flex items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
            <Button variant="secondary" onclick={() => testConnection(editingConnection.id)} loading={testingConnectionId === editingConnection?.id} icon={TestTube}>
              Test Connection
            </Button>
            {#if testResult}
              <div class="flex items-center gap-1 text-xs">
                {#if testResult.success}
                  <CheckCircle size={14} style="color: var(--ds-icon-success);" />
                  <span style="color: var(--ds-text-success);">{testResult.message}</span>
                {:else}
                  <XCircle size={14} style="color: var(--ds-text-danger);" />
                  <span style="color: var(--ds-text-danger);">{testResult.message}</span>
                {/if}
              </div>
            {/if}
          </div>
        {/if}

        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={() => showEditModal = false} keyboardHint="Esc">Cancel</Button>
          <Button variant="primary" onclick={handleUpdate} loading={saving} disabled={!form.name || !form.provider_type || !form.model} keyboardHint={submitHint}>
            Save
          </Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}


{#snippet connectionForm()}
  <!-- Name -->
  <div>
    <label for="llm-connection-name" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Name</label>
    <input
      id="llm-connection-name"
      type="text"
      bind:value={form.name}
      placeholder="e.g. Claude Sonnet"
      class="w-full px-3 py-2 text-sm rounded-md border"
      style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
    />
  </div>

  <!-- Provider Type -->
  <div>
    <label for="llm-connection-provider" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Provider</label>
    <Select
      id="llm-connection-provider"
      bind:value={form.provider_type}
      placeholder="Select a provider..."
      options={providers.map(p => ({ value: p.type, label: p.name }))}
      onchange={() => { form.model = ''; form.base_url = ''; }}
    />
  </div>

  <!-- Model -->
  <div>
    <label for="llm-connection-model" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Model</label>
    {#if isLocalProvider}
      <input
        id="llm-connection-model"
        type="text"
        bind:value={form.model}
        placeholder="e.g. llama-3.1-8b"
        class="w-full px-3 py-2 text-sm rounded-md border"
        style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
      />
    {:else}
      <Select
        id="llm-connection-model"
        bind:value={form.model}
        placeholder="Select a model..."
        options={availableModels.map(m => ({ value: m.id, label: m.name }))}
      />
    {/if}
  </div>

  <!-- API Key -->
  <div>
    <label for="llm-connection-api-key" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">API Key</label>
    <input
      id="llm-connection-api-key"
      type="password"
      bind:value={form.api_key}
      placeholder={editingConnection?.has_api_key ? 'Key configured (leave blank to keep)' : 'Enter API key'}
      class="w-full px-3 py-2 text-sm rounded-md border"
      style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
    />
  </div>

  <!-- Base URL (only for local) -->
  {#if isLocalProvider}
    <div>
      <label for="llm-connection-base-url" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Base URL</label>
      <input
        id="llm-connection-base-url"
        type="text"
        bind:value={form.base_url}
        placeholder="e.g. https://llm.example.com"
        class="w-full px-3 py-2 text-sm rounded-md border"
        style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
      />
    </div>
  {/if}

  <!-- Toggles -->
  <div class="flex items-center gap-6">
    <label class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
      <input type="checkbox" bind:checked={form.is_default} class="rounded" />
      <Star size={14} />
      Default connection
    </label>
    <label class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
      <input type="checkbox" bind:checked={form.is_enabled} class="rounded" />
      <Power size={14} />
      Enabled
    </label>
  </div>

{/snippet}
