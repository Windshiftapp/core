<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Edit, Trash2, Power, PowerOff } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Select from '../components/Select.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let capabilities = $state([]);
  let llmConnections = $state([]);
  let loading = $state(true);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let showDeleteModal = $state(false);
  let editingCapability = $state(null);
  let deletingCapability = $state(null);
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
    // LLM fields
    llm_connection_id: '',
  });

  function resetForm() {
    form = {
      name: '',
      capability_type: '',
      is_enabled: true,
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

  onMount(async () => {
    await Promise.all([loadCapabilities(), loadLLMConnections()]);
    loading = false;
  });

  function openCreate() {
    resetForm();
    showCreateModal = true;
  }

  function openEdit(cap) {
    editingCapability = cap;
    form.name = cap.name;
    form.capability_type = cap.capability_type;
    form.is_enabled = cap.is_enabled;
    parseConfigToForm(cap.capability_type, cap.config);
    showEditModal = true;
  }

  function openDelete(cap) {
    deletingCapability = cap;
    showDeleteModal = true;
  }

  async function handleCreate() {
    saving = true;
    try {
      await api.actionCapabilities.create({
        name: form.name,
        capability_type: form.capability_type,
        config: buildConfigJSON(),
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

  async function handleDelete() {
    if (!deletingCapability) return;
    try {
      await api.actionCapabilities.delete(deletingCapability.id);
      successToast(t('settings.actionCapabilities.deleteSuccess'));
      showDeleteModal = false;
      await loadCapabilities();
    } catch (err) {
      errorToast(err.message || t('settings.actionCapabilities.deleteFailed'));
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

  const canSubmit = $derived(form.name && form.capability_type);
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
    <div class="overflow-hidden rounded-lg border" style="border-color: var(--ds-border);">
      <table class="w-full text-sm">
        <thead>
          <tr style="background-color: var(--ds-surface-sunken);">
            <th class="text-left px-4 py-2 font-medium" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.name')}</th>
            <th class="text-left px-4 py-2 font-medium" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.capabilityType')}</th>
            <th class="text-left px-4 py-2 font-medium" style="color: var(--ds-text-subtle);">Status</th>
            <th class="text-right px-4 py-2 font-medium" style="color: var(--ds-text-subtle);">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each capabilities as cap}
            <tr class="border-t" style="border-color: var(--ds-border);">
              <td class="px-4 py-3">
                <span class="font-medium" style="color: var(--ds-text);">{cap.name}</span>
              </td>
              <td class="px-4 py-3">
                <Lozenge appearance={typeAppearance(cap.capability_type)} size="sm">{typeLabel(cap.capability_type)}</Lozenge>
              </td>
              <td class="px-4 py-3">
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
              </td>
              <td class="px-4 py-3">
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
                    onclick={() => openDelete(cap)}
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
    <ModalHeader title={t('settings.actionCapabilities.addCapability')} onclose={() => showCreateModal = false} />
    <div class="p-4 space-y-4">
      {@render capabilityForm(false)}
      <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
        <Button variant="secondary" onclick={() => showCreateModal = false}>Cancel</Button>
        <Button variant="primary" onclick={handleCreate} loading={saving} disabled={!canSubmit}>
          Create
        </Button>
      </div>
    </div>
  </Modal>
{/if}

<!-- Edit Modal -->
{#if showEditModal}
  <Modal isOpen={true} onclose={() => showEditModal = false}>
    <ModalHeader title={t('settings.actionCapabilities.editCapability')} onclose={() => showEditModal = false} />
    <div class="p-4 space-y-4">
      {@render capabilityForm(true)}
      <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
        <Button variant="secondary" onclick={() => showEditModal = false}>Cancel</Button>
        <Button variant="primary" onclick={handleUpdate} loading={saving} disabled={!canSubmit}>
          Save
        </Button>
      </div>
    </div>
  </Modal>
{/if}

<!-- Delete Modal -->
{#if showDeleteModal && deletingCapability}
  <Modal isOpen={true} onclose={() => showDeleteModal = false}>
    <ModalHeader title={t('settings.actionCapabilities.deleteCapability')} onclose={() => showDeleteModal = false} />
    <div class="p-4 space-y-4">
      <p class="text-sm" style="color: var(--ds-text);">
        {t('settings.actionCapabilities.confirmDelete')} <strong>{deletingCapability.name}</strong>? This action cannot be undone.
      </p>
      <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
        <Button variant="secondary" onclick={() => showDeleteModal = false}>Cancel</Button>
        <Button variant="danger" onclick={handleDelete}>Delete</Button>
      </div>
    </div>
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
        <label class="block text-xs font-medium" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.envVars')}</label>
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
      <label class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.healthCheck')}</label>
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
              <label class="block text-xs mb-0.5" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.intervalSecs')}</label>
              <input
                type="number"
                bind:value={form.docker_health_interval}
                class="w-full px-3 py-1.5 text-sm rounded-md border"
                style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
              />
            </div>
            <div>
              <label class="block text-xs mb-0.5" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.docker.timeoutSecs')}</label>
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
        <label class="block text-xs font-medium" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.http.allowedPatterns')}</label>
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

    <!-- Default Headers -->
    <div>
      <div class="flex items-center justify-between mb-1">
        <label class="block text-xs font-medium" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.http.defaultHeaders')}</label>
        <button class="text-xs font-medium px-2 py-0.5 rounded" style="color: var(--ds-link);" onclick={addHeader}>+ {t('settings.actionCapabilities.http.addHeader')}</button>
      </div>
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
    {#if llmConnections.length === 0}
      <p class="text-sm" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.llm.noConnections')}</p>
    {:else}
      <div>
        <label for="llm-conn" class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.actionCapabilities.llm.connection')}</label>
        <Select
          id="llm-conn"
          bind:value={form.llm_connection_id}
          placeholder={t('settings.actionCapabilities.llm.selectConnection')}
          options={llmConnections.map(c => ({ value: String(c.id), label: c.name }))}
        />
      </div>
    {/if}
  </div>
{/snippet}
