<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import {
    Plus, Edit, Trash2, X, TestTube, CheckCircle, XCircle, Power, PowerOff, Star
  } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';

  let connections = $state([]);
  let providers = $state([]);
  let loading = $state(true);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let showDeleteModal = $state(false);
  let editingConnection = $state(null);
  let deletingConnection = $state(null);
  let testResult = $state(null);
  let testLoading = $state(false);
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

  onMount(async () => {
    await Promise.all([loadConnections(), loadProviders()]);
    loading = false;
  });

  function openCreate() {
    resetForm();
    showCreateModal = true;
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

  function openDelete(conn) {
    deletingConnection = conn;
    showDeleteModal = true;
  }

  async function handleCreate() {
    saving = true;
    try {
      await api.llmConnections.create(form);
      successToast('AI connection created');
      showCreateModal = false;
      await loadConnections();
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
      await loadConnections();
    } catch (err) {
      errorToast(err.message || 'Failed to update connection');
    } finally {
      saving = false;
    }
  }

  async function handleDelete() {
    if (!deletingConnection) return;
    try {
      await api.llmConnections.delete(deletingConnection.id);
      successToast('AI connection deleted');
      showDeleteModal = false;
      await loadConnections();
    } catch (err) {
      errorToast(err.message || 'Failed to delete connection');
    }
  }

  async function testConnection(id) {
    testLoading = true;
    testResult = null;
    try {
      await api.llmConnections.test(id);
      testResult = { success: true, message: 'Connection successful' };
      successToast('Connection test passed');
    } catch (err) {
      testResult = { success: false, message: err.message || 'Connection test failed' };
      errorToast('Connection test failed');
    } finally {
      testLoading = false;
    }
  }

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
    <div class="overflow-hidden rounded-lg border" style="border-color: var(--ds-border);">
      <table class="w-full text-sm">
        <thead>
          <tr style="background-color: var(--ds-surface-sunken);">
            <th class="text-left px-4 py-2 font-medium" style="color: var(--ds-text-subtle);">Name</th>
            <th class="text-left px-4 py-2 font-medium" style="color: var(--ds-text-subtle);">Provider</th>
            <th class="text-left px-4 py-2 font-medium" style="color: var(--ds-text-subtle);">Model</th>
            <th class="text-left px-4 py-2 font-medium" style="color: var(--ds-text-subtle);">Status</th>
            <th class="text-right px-4 py-2 font-medium" style="color: var(--ds-text-subtle);">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each connections as conn}
            <tr class="border-t" style="border-color: var(--ds-border);">
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <span class="font-medium" style="color: var(--ds-text);">{conn.name}</span>
                  {#if conn.is_default}
                    <Lozenge appearance="info" size="sm">Default</Lozenge>
                  {/if}
                </div>
              </td>
              <td class="px-4 py-3" style="color: var(--ds-text-subtle);">{conn.provider_type}</td>
              <td class="px-4 py-3">
                <span class="font-mono text-xs px-1.5 py-0.5 rounded" style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);">{conn.model}</span>
              </td>
              <td class="px-4 py-3">
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
              </td>
              <td class="px-4 py-3">
                <div class="flex items-center justify-end gap-1">
                  <button
                    class="p-1.5 rounded hover:opacity-80"
                    style="color: var(--ds-text-subtle);"
                    title="Test connection"
                    onclick={() => testConnection(conn.id)}
                  >
                    <TestTube size={14} />
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
                    onclick={() => openDelete(conn)}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Create Modal -->
{#if showCreateModal}
  <Modal isOpen={true} onclose={() => showCreateModal = false}>
    <ModalHeader title="Add AI Connection" onclose={() => showCreateModal = false} />
    <div class="p-4 space-y-4">
      {@render connectionForm()}
      <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
        <Button variant="secondary" onclick={() => showCreateModal = false}>Cancel</Button>
        <Button variant="primary" onclick={handleCreate} loading={saving} disabled={!form.name || !form.provider_type || !form.model}>
          Create
        </Button>
      </div>
    </div>
  </Modal>
{/if}

<!-- Edit Modal -->
{#if showEditModal}
  <Modal isOpen={true} onclose={() => showEditModal = false}>
    <ModalHeader title="Edit AI Connection" onclose={() => showEditModal = false} />
    <div class="p-4 space-y-4">
      {@render connectionForm()}

      {#if editingConnection}
        <div class="flex items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={() => testConnection(editingConnection.id)} loading={testLoading} icon={TestTube}>
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
        <Button variant="secondary" onclick={() => showEditModal = false}>Cancel</Button>
        <Button variant="primary" onclick={handleUpdate} loading={saving} disabled={!form.name || !form.provider_type || !form.model}>
          Save
        </Button>
      </div>
    </div>
  </Modal>
{/if}

<!-- Delete Modal -->
{#if showDeleteModal && deletingConnection}
  <Modal isOpen={true} onclose={() => showDeleteModal = false}>
    <ModalHeader title="Delete AI Connection" onclose={() => showDeleteModal = false} />
    <div class="p-4 space-y-4">
      <p class="text-sm" style="color: var(--ds-text);">
        Are you sure you want to delete <strong>{deletingConnection.name}</strong>? This action cannot be undone.
      </p>
      <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
        <Button variant="secondary" onclick={() => showDeleteModal = false}>Cancel</Button>
        <Button variant="danger" onclick={handleDelete}>Delete</Button>
      </div>
    </div>
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
    <select
      id="llm-connection-provider"
      bind:value={form.provider_type}
      class="w-full px-3 py-2 text-sm rounded-md border"
      style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
      onchange={() => { form.model = ''; form.base_url = ''; }}
    >
      <option value="">Select a provider...</option>
      {#each providers as provider}
        <option value={provider.type}>{provider.name}</option>
      {/each}
    </select>
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
      <select
        id="llm-connection-model"
        bind:value={form.model}
        class="w-full px-3 py-2 text-sm rounded-md border"
        style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
      >
        <option value="">Select a model...</option>
        {#each availableModels as model}
          <option value={model.id}>{model.name}</option>
        {/each}
      </select>
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
