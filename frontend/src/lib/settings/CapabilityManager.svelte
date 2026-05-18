<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Edit, Trash2, Power, PowerOff } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Select from '../components/Select.svelte';
  import DataTable from '../components/DataTable.svelte';
  import CredentialPicker from '../components/CredentialPicker.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';

  let capabilities = $state([]);
  let llmConnections = $state([]);
  let workspaces = $state([]);
  let loading = $state(true);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let editingCapability = $state(null);
  let saving = $state(false);

  const CAPABILITY_TYPES = [
    { value: 'docker_environment', label: t('settings.actionCapabilities.typeDocker') },
    { value: 'http_client', label: t('settings.actionCapabilities.typeHTTP') },
    { value: 'llm_connection', label: t('settings.actionCapabilities.typeLLM') },
  ];

  const NETWORK_MODES = [
    { value: 'none', label: 'none' },
    { value: 'bridge', label: 'bridge' },
    { value: 'host', label: 'host' },
  ];

  // Form state
  let form = $state({
    name: '',
    capability_type: '',
    is_enabled: true,
    // Scope: when applies_to_all_workspaces=true, every workspace's actions
    // can reference this capability. When false, only workspaces in
    // workspace_ids may reference it.
    applies_to_all_workspaces: true,
    workspace_ids: [],
    // Docker fields
    docker_image: '',
    docker_memory: '512m',
    docker_cpus: '1',
    docker_network_mode: 'none',
    docker_env_vars: [],
    docker_health_endpoint: '',
    docker_health_interval: 30,
    docker_health_timeout: 5,
    // HTTP fields
    http_allowed_patterns: [],
    http_default_headers: [],
    http_timeout: 30,
    // HTTP auth — primary credential ref (Authorization-style header) and
    // a per-header map for APIs that need additional secret headers.
    http_auth_enabled: false,
    http_auth_credential_id: 0,
    http_auth_header_name: 'Authorization',
    http_auth_scheme: 'Bearer',
    http_secret_header_refs: [],
    // LLM fields
    llm_connection_id: '',
  });

  function resetForm() {
    form = {
      name: '',
      capability_type: '',
      is_enabled: true,
      applies_to_all_workspaces: true,
      workspace_ids: [],
      docker_image: '',
      docker_memory: '512m',
      docker_cpus: '1',
      docker_network_mode: 'none',
      docker_env_vars: [],
      docker_health_endpoint: '',
      docker_health_interval: 30,
      docker_health_timeout: 5,
      http_allowed_patterns: [],
      http_default_headers: [],
      http_timeout: 30,
      http_auth_enabled: false,
      http_auth_credential_id: 0,
      http_auth_header_name: 'Authorization',
      http_auth_scheme: 'Bearer',
      http_secret_header_refs: [],
      llm_connection_id: '',
    };
  }

  function buildConfigJSON() {
    if (form.capability_type === 'docker_environment') {
      const config = {
        image: form.docker_image,
        resource_limits: {
          memory: form.docker_memory,
          cpus: form.docker_cpus,
        },
        network_mode: form.docker_network_mode,
      };
      if (form.docker_env_vars.length > 0) {
        config.env_vars = {};
        for (const kv of form.docker_env_vars) {
          if (kv.key) config.env_vars[kv.key] = kv.value;
        }
      }
      if (form.docker_health_endpoint) {
        config.health_check = {
          endpoint: form.docker_health_endpoint,
          interval_secs: form.docker_health_interval,
          timeout_secs: form.docker_health_timeout,
        };
      }
      return JSON.stringify(config);
    }
    if (form.capability_type === 'http_client') {
      const config = {
        allowed_url_patterns: form.http_allowed_patterns.map(p => p.value).filter(Boolean),
        timeout_secs: form.http_timeout,
      };
      if (form.http_default_headers.length > 0) {
        config.default_headers = {};
        for (const kv of form.http_default_headers) {
          if (kv.key) config.default_headers[kv.key] = kv.value;
        }
      }
      if (form.http_auth_enabled && form.http_auth_credential_id) {
        config.auth = {
          credential_id: Number(form.http_auth_credential_id),
          placement: 'header',
          header_name: form.http_auth_header_name || 'Authorization',
          scheme: form.http_auth_scheme || '',
        };
      }
      const refs = {};
      for (const row of form.http_secret_header_refs) {
        if (row.header && row.credential_id) {
          refs[row.header] = Number(row.credential_id);
        }
      }
      if (Object.keys(refs).length > 0) {
        config.secret_header_refs = refs;
      }
      return JSON.stringify(config);
    }
    if (form.capability_type === 'llm_connection') {
      return JSON.stringify({ connection_id: Number(form.llm_connection_id) });
    }
    return '{}';
  }

  function parseConfigToForm(type, configStr) {
    try {
      const config = JSON.parse(configStr || '{}');
      if (type === 'docker_environment') {
        form.docker_image = config.image || '';
        form.docker_memory = config.resource_limits?.memory || '512m';
        form.docker_cpus = config.resource_limits?.cpus || '1';
        form.docker_network_mode = config.network_mode || 'none';
        form.docker_env_vars = config.env_vars
          ? Object.entries(config.env_vars).map(([key, value]) => ({ key, value }))
          : [];
        form.docker_health_endpoint = config.health_check?.endpoint || '';
        form.docker_health_interval = config.health_check?.interval_secs || 30;
        form.docker_health_timeout = config.health_check?.timeout_secs || 5;
      } else if (type === 'http_client') {
        form.http_allowed_patterns = (config.allowed_url_patterns || []).map(v => ({ value: v }));
        form.http_timeout = config.timeout_secs || 30;
        form.http_default_headers = config.default_headers
          ? Object.entries(config.default_headers).map(([key, value]) => ({ key, value }))
          : [];
        // Auth ref — the workspace listing returns header_name + scheme but
        // intentionally zeroes credential_id; the admin endpoint returns the
        // real credential_id. Either way the form mirrors what we got back.
        if (config.auth) {
          form.http_auth_enabled = true;
          form.http_auth_credential_id = config.auth.credential_id || 0;
          form.http_auth_header_name = config.auth.header_name || 'Authorization';
          form.http_auth_scheme = config.auth.scheme || '';
        } else {
          form.http_auth_enabled = false;
          form.http_auth_credential_id = 0;
          form.http_auth_header_name = 'Authorization';
          form.http_auth_scheme = 'Bearer';
        }
        form.http_secret_header_refs = config.secret_header_refs
          ? Object.entries(config.secret_header_refs).map(([header, credential_id]) => ({
              header,
              credential_id: credential_id || 0,
            }))
          : [];
      } else if (type === 'llm_connection') {
        form.llm_connection_id = config.connection_id ? String(config.connection_id) : '';
      }
    } catch {
      // ignore parse errors
    }
  }

  function typeLabel(type) {
    return CAPABILITY_TYPES.find(t => t.value === type)?.label || type;
  }

  function typeAppearance(type) {
    if (type === 'docker_environment') return 'info';
    if (type === 'http_client') return 'warning';
    if (type === 'llm_connection') return 'success';
    return 'default';
  }

  function enabledLLMConnections() {
    return llmConnections.filter((connection) => connection.is_enabled !== false);
  }

  async function loadCapabilities() {
    try {
      capabilities = await api.actionCapabilities.getAll();
    } catch (err) {
      console.error('Failed to load capabilities:', err);
      errorToast(t('settings.actionCapabilities.loadFailed'));
    }
  }

  async function loadLLMConnections() {
    try {
      llmConnections = await api.llmConnections.getAll();
    } catch (err) {
      console.error('Failed to load LLM connections:', err);
    }
  }

  async function loadWorkspaces() {
    try {
      workspaces = await api.workspaces.getAll();
    } catch (err) {
      console.error('Failed to load workspaces:', err);
    }
  }

  onMount(async () => {
    await Promise.all([loadCapabilities(), loadLLMConnections(), loadWorkspaces()]);
    loading = false;
  });

  function openCreate() {
    resetForm();
    showCreateModal = true;
  }

  function openEdit(cap) {
    resetForm();
    editingCapability = cap;
    form.name = cap.name;
    form.capability_type = cap.capability_type;
    form.is_enabled = cap.is_enabled;
    form.applies_to_all_workspaces = cap.applies_to_all_workspaces ?? true;
    form.workspace_ids = Array.isArray(cap.workspace_ids) ? [...cap.workspace_ids] : [];
    parseConfigToForm(cap.capability_type, cap.config);
    showEditModal = true;
  }

  async function deleteCapability(cap) {
    const ok = await confirm({
      title: t('settings.actionCapabilities.deleteCapability'),
      message: t('settings.actionCapabilities.confirmDelete') + ' ' + cap.name + '?',
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.actionCapabilities.delete(cap.id);
      successToast(t('settings.actionCapabilities.deleteSuccess'));
      await loadCapabilities();
    } catch (err) {
      errorToast(err.message || t('settings.actionCapabilities.deleteFailed'));
    }
  }

  async function handleCreate() {
    saving = true;
    try {
      await api.actionCapabilities.create({
        name: form.name,
        capability_type: form.capability_type,
        config: buildConfigJSON(),
        is_enabled: form.is_enabled,
        applies_to_all_workspaces: form.applies_to_all_workspaces,
        workspace_ids: form.applies_to_all_workspaces ? [] : form.workspace_ids,
      });
      successToast(t('settings.actionCapabilities.createSuccess'));
      showCreateModal = false;
      await loadCapabilities();
    } catch (err) {
      errorToast(err.message || t('settings.actionCapabilities.createFailed'));
    } finally {
      saving = false;
    }
  }

  async function handleUpdate() {
    if (!editingCapability) return;
    saving = true;
    try {
      await api.actionCapabilities.update(editingCapability.id, {
        name: form.name,
        config: buildConfigJSON(),
        is_enabled: form.is_enabled,
        applies_to_all_workspaces: form.applies_to_all_workspaces,
        workspace_ids: form.applies_to_all_workspaces ? [] : form.workspace_ids,
      });
      successToast(t('settings.actionCapabilities.updateSuccess'));
      showEditModal = false;
      await loadCapabilities();
    } catch (err) {
      errorToast(err.message || t('settings.actionCapabilities.updateFailed'));
    } finally {
      saving = false;
    }
  }

  function toggleWorkspaceScope(workspaceId) {
    const id = Number(workspaceId);
    if (form.workspace_ids.includes(id)) {
      form.workspace_ids = form.workspace_ids.filter((w) => w !== id);
    } else {
      form.workspace_ids = [...form.workspace_ids, id];
    }
  }


  // Dynamic list helpers
  function addEnvVar() {
    form.docker_env_vars = [...form.docker_env_vars, { key: '', value: '' }];
  }
  function removeEnvVar(index) {
    form.docker_env_vars = form.docker_env_vars.filter((_, i) => i !== index);
  }
  function addPattern() {
    form.http_allowed_patterns = [...form.http_allowed_patterns, { value: '' }];
  }
  function removePattern(index) {
    form.http_allowed_patterns = form.http_allowed_patterns.filter((_, i) => i !== index);
  }
  function addHeader() {
    form.http_default_headers = [...form.http_default_headers, { key: '', value: '' }];
  }
  function removeHeader(index) {
    form.http_default_headers = form.http_default_headers.filter((_, i) => i !== index);
  }
  function addSecretHeaderRef() {
    form.http_secret_header_refs = [
      ...form.http_secret_header_refs,
      { header: '', credential_id: 0 },
    ];
  }
  function removeSecretHeaderRef(index) {
    form.http_secret_header_refs = form.http_secret_header_refs.filter((_, i) => i !== index);
  }

  function hasValidTypeConfig() {
    if (form.capability_type === 'docker_environment') return Boolean(form.docker_image.trim());
    if (form.capability_type === 'http_client') return form.http_allowed_patterns.some((p) => p.value?.trim());
    if (form.capability_type === 'llm_connection') return Boolean(form.llm_connection_id);
    return false;
  }

  // Scope must be coherent: applies-to-all OR at least one explicit workspace.
  const canSubmit = $derived(
    form.name.trim() && form.capability_type && hasValidTypeConfig() &&
    (form.applies_to_all_workspaces || form.workspace_ids.length > 0)
  );

  const columns = [
    { key: 'name', label: t('settings.actionCapabilities.name'), slot: 'name' },
    { key: 'capability_type', label: t('settings.actionCapabilities.capabilityType'), slot: 'type' },
    { key: 'is_enabled', label: 'Status', slot: 'status' },
    { key: 'actions', label: 'Actions', slot: 'actions', align: 'text-right', width: 'w-32' },
  ];
</script>

<div class="space-y-4">
  <PageHeader title={t('settings.actionCapabilities.title')} subtitle={t('settings.actionCapabilities.subtitle')}>
    {#snippet actions()}
      <Button variant="primary" onclick={openCreate} icon={Plus}>
        {t('settings.actionCapabilities.addCapability')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Spinner />
    </div>
  {:else if capabilities.length === 0}
    <div class="flex flex-col items-center py-12 gap-3 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
      <p class="text-sm" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.noCapabilities')}</p>
      <Button variant="secondary" onclick={openCreate} icon={Plus}>
        {t('settings.actionCapabilities.addFirst')}
      </Button>
    </div>
  {:else}
    <DataTable {columns} data={capabilities} keyField="id">
      {#snippet name(cap)}
        <span class="font-medium" style="color: var(--ds-text);">{cap.name}</span>
      {/snippet}
      {#snippet type(cap)}
        <Lozenge appearance={typeAppearance(cap.capability_type)} size="sm">{typeLabel(cap.capability_type)}</Lozenge>
      {/snippet}
      {#snippet status(cap)}
        {#if cap.is_enabled}
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
      {#snippet actions(cap)}
        <div class="flex items-center justify-end gap-1">
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-subtle);"
            title="Edit"
            onclick={() => openEdit(cap)}
          >
            <Edit size={14} />
          </button>
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-danger);"
            title="Delete"
            onclick={() => deleteCapability(cap)}
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
    submitDisabled={!canSubmit || saving}
  >
    {#snippet children(submitHint)}
      <ModalHeader title={t('settings.actionCapabilities.addCapability')} onclose={() => showCreateModal = false} />
      <div class="p-4 space-y-4">
        {@render capabilityForm(false)}
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={() => showCreateModal = false} keyboardHint="Esc">Cancel</Button>
          <Button variant="primary" onclick={handleCreate} loading={saving} disabled={!canSubmit} keyboardHint={submitHint}>
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
    submitDisabled={!canSubmit || saving}
  >
    {#snippet children(submitHint)}
      <ModalHeader title={t('settings.actionCapabilities.editCapability')} onclose={() => showEditModal = false} />
      <div class="p-4 space-y-4">
        {@render capabilityForm(true)}
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={() => showEditModal = false} keyboardHint="Esc">Cancel</Button>
          <Button variant="primary" onclick={handleUpdate} loading={saving} disabled={!canSubmit} keyboardHint={submitHint}>
            Save
          </Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}


{#snippet capabilityForm(isEdit)}
  <!-- Name -->
  <div>
    <label for="cap-name" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.name')}</label>
    <input
      id="cap-name"
      type="text"
      bind:value={form.name}
      placeholder={t('settings.actionCapabilities.namePlaceholder')}
      class="w-full px-3 py-2 text-sm rounded-md border"
      style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
    />
  </div>

  <!-- Capability Type -->
  <div>
    <label for="cap-type" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.capabilityType')}</label>
    {#if isEdit}
      <div class="px-3 py-2 text-sm rounded-md border" style="border-color: var(--ds-border); background: var(--ds-surface-sunken); color: var(--ds-text-subtle);">
        {typeLabel(form.capability_type)}
      </div>
    {:else}
      <Select
        id="cap-type"
        bind:value={form.capability_type}
        placeholder={t('settings.actionCapabilities.selectType')}
        options={CAPABILITY_TYPES}
      />
    {/if}
  </div>

  <!-- Enabled toggle -->
  <div class="flex items-center gap-2">
    <label class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
      <input type="checkbox" bind:checked={form.is_enabled} class="rounded" />
      <Power size={14} />
      {t('settings.actionCapabilities.enabled')}
    </label>
  </div>

  <!-- Scope: applies to all workspaces vs. specific allowlist -->
  <div class="space-y-2 pt-2 border-t" style="border-color: var(--ds-border);">
    <div class="block text-xs font-medium" style="color: var(--ds-text-subtle);">Workspace scope</div>
    <label class="flex items-start gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
      <input
        type="radio"
        name="cap-scope"
        checked={form.applies_to_all_workspaces}
        onchange={() => { form.applies_to_all_workspaces = true; }}
        class="mt-0.5"
      />
      <div>
        <div>Available in all workspaces</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">Any workspace's actions can reference this capability.</div>
      </div>
    </label>
    <label class="flex items-start gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
      <input
        type="radio"
        name="cap-scope"
        checked={!form.applies_to_all_workspaces}
        onchange={() => { form.applies_to_all_workspaces = false; }}
        class="mt-0.5"
      />
      <div>
        <div>Restrict to specific workspaces</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">Only the workspaces selected below can reference this capability.</div>
      </div>
    </label>

    {#if !form.applies_to_all_workspaces}
      <div class="ml-6 mt-1 max-h-40 overflow-auto rounded-md border p-2" style="border-color: var(--ds-border); background: var(--ds-surface);">
        {#if workspaces.length === 0}
          <p class="text-xs" style="color: var(--ds-text-subtle);">No workspaces available.</p>
        {:else}
          {#each workspaces as ws}
            <label class="flex items-center gap-2 text-sm py-1 cursor-pointer" style="color: var(--ds-text);">
              <input
                type="checkbox"
                checked={form.workspace_ids.includes(ws.id)}
                onchange={() => toggleWorkspaceScope(ws.id)}
              />
              {ws.name}
            </label>
          {/each}
        {/if}
      </div>
    {/if}
  </div>

  <!-- Type-specific config -->
  {#if form.capability_type === 'docker_environment'}
    {@render dockerForm()}
  {/if}
  {#if form.capability_type === 'http_client'}
    {@render httpForm()}
  {/if}
  {#if form.capability_type === 'llm_connection'}
    {@render llmForm()}
  {/if}
{/snippet}

{#snippet dockerForm()}
  <div class="space-y-3 pt-2 border-t" style="border-color: var(--ds-border);">
    <!-- Image -->
    <div>
      <label for="docker-image" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.image')}</label>
      <input
        id="docker-image"
        type="text"
        bind:value={form.docker_image}
        placeholder={t('settings.actionCapabilities.docker.imagePlaceholder')}
        class="w-full px-3 py-2 text-sm rounded-md border"
        style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
      />
    </div>

    <!-- Resource Limits -->
    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="docker-memory" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.memory')}</label>
        <input
          id="docker-memory"
          type="text"
          bind:value={form.docker_memory}
          placeholder={t('settings.actionCapabilities.docker.memoryPlaceholder')}
          class="w-full px-3 py-2 text-sm rounded-md border"
          style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
        />
      </div>
      <div>
        <label for="docker-cpus" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.cpus')}</label>
        <input
          id="docker-cpus"
          type="text"
          bind:value={form.docker_cpus}
          placeholder={t('settings.actionCapabilities.docker.cpusPlaceholder')}
          class="w-full px-3 py-2 text-sm rounded-md border"
          style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
        />
      </div>
    </div>

    <!-- Network Mode -->
    <div>
      <label for="docker-network" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.networkMode')}</label>
      <Select
        id="docker-network"
        bind:value={form.docker_network_mode}
        options={NETWORK_MODES}
      />
    </div>

    <!-- Environment Variables -->
    <div>
      <div class="flex items-center justify-between mb-1">
        <div class="block text-xs font-medium" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.envVars')}</div>
        <button class="text-xs font-medium px-2 py-0.5 rounded" style="color: var(--ds-link);" onclick={addEnvVar}>+ {t('settings.actionCapabilities.docker.addEnvVar')}</button>
      </div>
      {#each form.docker_env_vars as envVar, i}
        <div class="flex gap-2 mb-1">
          <input
            type="text"
            bind:value={envVar.key}
            placeholder={t('settings.actionCapabilities.docker.key')}
            class="flex-1 px-3 py-1.5 text-sm rounded-md border"
            style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
          />
          <input
            type="text"
            bind:value={envVar.value}
            placeholder={t('settings.actionCapabilities.docker.value')}
            class="flex-1 px-3 py-1.5 text-sm rounded-md border"
            style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
          />
          <button class="p-1 rounded hover:opacity-80" style="color: var(--ds-text-danger);" onclick={() => removeEnvVar(i)}>
            <Trash2 size={14} />
          </button>
        </div>
      {/each}
    </div>

    <!-- Health Check -->
    <div>
      <div class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.healthCheck')}</div>
      <div class="space-y-2">
        <input
          type="text"
          bind:value={form.docker_health_endpoint}
          placeholder={t('settings.actionCapabilities.docker.endpointPlaceholder')}
          class="w-full px-3 py-1.5 text-sm rounded-md border"
          style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
        />
        {#if form.docker_health_endpoint}
          <div class="grid grid-cols-2 gap-2">
            <div>
              <div class="block text-xs mb-0.5" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.intervalSecs')}</div>
              <input
                type="number"
                bind:value={form.docker_health_interval}
                class="w-full px-3 py-1.5 text-sm rounded-md border"
                style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
              />
            </div>
            <div>
              <div class="block text-xs mb-0.5" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.timeoutSecs')}</div>
              <input
                type="number"
                bind:value={form.docker_health_timeout}
                class="w-full px-3 py-1.5 text-sm rounded-md border"
                style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
              />
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/snippet}

{#snippet httpForm()}
  <div class="space-y-3 pt-2 border-t" style="border-color: var(--ds-border);">
    <!-- Allowed URL Patterns -->
    <div>
      <div class="flex items-center justify-between mb-1">
        <div class="block text-xs font-medium" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.http.allowedPatterns')}</div>
        <button class="text-xs font-medium px-2 py-0.5 rounded" style="color: var(--ds-link);" onclick={addPattern}>+ {t('settings.actionCapabilities.http.addPattern')}</button>
      </div>
      {#each form.http_allowed_patterns as pattern, i}
        <div class="flex gap-2 mb-1">
          <input
            type="text"
            bind:value={pattern.value}
            placeholder={t('settings.actionCapabilities.http.patternPlaceholder')}
            class="flex-1 px-3 py-1.5 text-sm rounded-md border"
            style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
          />
          <button class="p-1 rounded hover:opacity-80" style="color: var(--ds-text-danger);" onclick={() => removePattern(i)}>
            <Trash2 size={14} />
          </button>
        </div>
      {/each}
    </div>

    <!-- Default Headers (non-sensitive literals only) -->
    <div>
      <div class="flex items-center justify-between mb-1">
        <div class="block text-xs font-medium" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.http.defaultHeaders')}</div>
        <button class="text-xs font-medium px-2 py-0.5 rounded" style="color: var(--ds-link);" onclick={addHeader}>+ {t('settings.actionCapabilities.http.addHeader')}</button>
      </div>
      <p class="text-xs mb-1" style="color: var(--ds-text-subtle);">
        Non-sensitive headers only (Accept, User-Agent, …). Auth tokens go in the Authentication section below.
      </p>
      {#each form.http_default_headers as header, i}
        <div class="flex gap-2 mb-1">
          <input
            type="text"
            bind:value={header.key}
            placeholder={t('settings.actionCapabilities.http.key')}
            class="flex-1 px-3 py-1.5 text-sm rounded-md border"
            style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
          />
          <input
            type="text"
            bind:value={header.value}
            placeholder={t('settings.actionCapabilities.http.value')}
            class="flex-1 px-3 py-1.5 text-sm rounded-md border"
            style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
          />
          <button class="p-1 rounded hover:opacity-80" style="color: var(--ds-text-danger);" onclick={() => removeHeader(i)}>
            <Trash2 size={14} />
          </button>
        </div>
      {/each}
    </div>

    <!-- Authentication (credential refs) -->
    <div class="rounded-md border p-3" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
      <label class="flex items-center gap-2 mb-2">
        <input type="checkbox" bind:checked={form.http_auth_enabled} />
        <span class="text-sm font-medium" style="color: var(--ds-text);">Use an auth credential</span>
      </label>
      {#if form.http_auth_enabled}
        <div class="space-y-2 mt-2">
          <div>
            <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Credential</div>
            <CredentialPicker
              workspaceId={form.applies_to_all_workspaces ? 0 : form.workspace_ids[0] || 0}
              bind:value={form.http_auth_credential_id}
              types={['bearer_token', 'api_key', 'basic_auth', 'custom_header']}
            />
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              {#if form.applies_to_all_workspaces}
                Global capabilities can only reference global credentials.
              {:else}
                Workspace-scoped capabilities can reference globals or credentials in their workspace allowlist.
              {/if}
            </p>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <label class="block">
              <span class="text-xs font-medium" style="color: var(--ds-text-subtle);">Header name</span>
              <input
                type="text"
                bind:value={form.http_auth_header_name}
                placeholder="Authorization"
                class="mt-1 w-full px-3 py-1.5 text-sm rounded-md border"
                style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
              />
            </label>
            <label class="block">
              <span class="text-xs font-medium" style="color: var(--ds-text-subtle);">Scheme (bearer tokens only)</span>
              <input
                type="text"
                bind:value={form.http_auth_scheme}
                placeholder="Bearer"
                class="mt-1 w-full px-3 py-1.5 text-sm rounded-md border"
                style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
              />
            </label>
          </div>
        </div>
      {/if}
    </div>

    <!-- Secret header refs (APIs that need multiple secret headers) -->
    <div>
      <div class="flex items-center justify-between mb-1">
        <div class="block text-xs font-medium" style="color: var(--ds-text-subtle);">Additional secret headers</div>
        <button class="text-xs font-medium px-2 py-0.5 rounded" style="color: var(--ds-link);" onclick={addSecretHeaderRef}>+ Add secret header</button>
      </div>
      <p class="text-xs mb-1" style="color: var(--ds-text-subtle);">
        For APIs that need more than one secret header (e.g. X-API-Key + X-Signature). Each row resolves a credential at request time.
      </p>
      {#each form.http_secret_header_refs as ref, i}
        <div class="flex gap-2 mb-1">
          <input
            type="text"
            bind:value={ref.header}
            placeholder="X-API-Key"
            class="flex-1 px-3 py-1.5 text-sm rounded-md border"
            style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
          />
          <div class="flex-1">
            <CredentialPicker
              workspaceId={form.applies_to_all_workspaces ? 0 : form.workspace_ids[0] || 0}
              bind:value={ref.credential_id}
            />
          </div>
          <button class="p-1 rounded hover:opacity-80" style="color: var(--ds-text-danger);" onclick={() => removeSecretHeaderRef(i)}>
            <Trash2 size={14} />
          </button>
        </div>
      {/each}
    </div>

    <!-- Timeout -->
    <div>
      <label for="http-timeout" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.http.timeout')}</label>
      <input
        id="http-timeout"
        type="number"
        bind:value={form.http_timeout}
        placeholder={t('settings.actionCapabilities.http.timeoutPlaceholder')}
        class="w-full px-3 py-2 text-sm rounded-md border"
        style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
      />
    </div>
  </div>
{/snippet}

{#snippet llmForm()}
  <div class="space-y-3 pt-2 border-t" style="border-color: var(--ds-border);">
    {#if enabledLLMConnections().length === 0}
      <p class="text-sm" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.llm.noConnections')}</p>
    {:else}
      <div>
        <label for="llm-conn" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.llm.connection')}</label>
        <Select
          id="llm-conn"
          bind:value={form.llm_connection_id}
          placeholder={t('settings.actionCapabilities.llm.selectConnection')}
          options={enabledLLMConnections().map(c => ({ value: String(c.id), label: c.name }))}
        />
      </div>
    {/if}
  </div>
{/snippet}
