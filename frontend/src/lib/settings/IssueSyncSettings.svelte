<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import Card from '../components/Card.svelte';
  import Toggle from '../components/Toggle.svelte';
  import Label from '../components/Label.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Chip from '../components/Chip.svelte';
  import { RefreshCw, Trash2, ExternalLink, Loader2, Plus, X, Layers } from '@lucide/svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import ChipPicker from '../pickers/ChipPicker.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import { itemTypeIconMap } from '../utils/icons.js';
  import { safeHref } from '../utils/sanitize';

  let { workspaceId } = $props();

  let loading = $state(true);
  let saving = $state(false);
  let syncing = $state(false);
  let config = $state(null);
  let linkedRepos = $state([]);
  let statuses = $state([]);
  let itemTypes = $state([]);
  let priorities = $state([]);
  let users = $state([]);
  let milestones = $state([]);
  let syncedItems = $state([]);
  let syncStatus = $state(null);

  // Form state
  let formData = $state({
    workspace_repository_id: 0,
    sync_enabled: false,
    status_mapping: { open: null, closed: null },
    reverse_status_mapping: {},
    label_sync_mode: 'none',
    label_mappings: [],
    filter_labels: [],
    assignee_mappings: {},
    milestone_mappings: {},
    default_item_type_id: null,
    default_priority_id: null,
    sync_comments: false,
  });

  let filterLabelInput = $state('');
  let newAssigneeGH = $state('');
  let newAssigneeWS = $state(null);

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    try {
      // Load config separately so a 404 (no config yet) doesn't break other loads
      let configRes = null;
      try {
        configRes = await api.issueSync.getConfig(workspaceId);
      } catch (e) {
        if (e.status !== 404) console.error('Failed to load issue sync config:', e);
      }

      await loadLinkedRepos();
      config = configRes;

      // Load additional data
      const [typesRes, prioritiesRes, usersRes, milestonesRes] = await Promise.all([
        api.itemTypes.getAll({ workspace_id: workspaceId }),
        api.priorities.getAll({ workspace_id: workspaceId }),
        api.getAssignableUsers(workspaceId),
        api.milestones.getAll({ workspace_id: workspaceId }),
      ]);
      itemTypes = typesRes || [];
      priorities = prioritiesRes || [];
      users = usersRes || [];
      milestones = milestonesRes || [];

      if (config) {
        populateForm(config);
        await loadSyncDetails();
      }
    } catch (error) {
      console.error('Failed to load issue sync data:', error);
    } finally {
      loading = false;
    }
  }

  async function loadLinkedRepos() {
    try {
      const connections = await api.workspaceSCM.getConnections(workspaceId);
      const allRepos = [];
      for (const conn of (connections || [])) {
        try {
          const repos = await api.workspaceSCM.getLinkedRepos(workspaceId, conn.id);
          if (repos) allRepos.push(...repos);
        } catch { /* skip */ }
      }
      linkedRepos = allRepos;
    } catch {
      linkedRepos = [];
    }
  }

  async function loadSyncDetails() {
    if (!config) return;
    try {
      const [statusRes, itemsRes] = await Promise.all([
        api.issueSync.getStatus(workspaceId),
        api.issueSync.getItems(workspaceId),
      ]);
      syncStatus = statusRes;
      syncedItems = itemsRes || [];
    } catch (error) {
      console.error('Failed to load sync details:', error);
    }
  }

  function populateForm(cfg) {
    formData = {
      workspace_repository_id: cfg.workspace_repository_id,
      sync_enabled: cfg.sync_enabled,
      status_mapping: safeJSON(cfg.status_mapping, { open: null, closed: null }),
      reverse_status_mapping: safeJSON(cfg.reverse_status_mapping, {}),
      label_sync_mode: cfg.label_sync_mode || 'none',
      label_mappings: safeJSON(cfg.label_mappings, []),
      filter_labels: safeJSON(cfg.filter_labels, []),
      assignee_mappings: safeJSON(cfg.assignee_mappings, {}),
      milestone_mappings: safeJSON(cfg.milestone_mappings, {}),
      default_item_type_id: cfg.default_item_type_id,
      default_priority_id: cfg.default_priority_id,
      sync_comments: cfg.sync_comments,
    };
  }

  function safeJSON(val, fallback) {
    if (!val || val === '{}' || val === '[]') return fallback;
    try {
      return typeof val === 'string' ? JSON.parse(val) : val;
    } catch {
      return fallback;
    }
  }

  async function saveConfig() {
    saving = true;
    try {
      const payload = {
        workspace_repository_id: formData.workspace_repository_id,
        sync_enabled: formData.sync_enabled,
        status_mapping: JSON.stringify(formData.status_mapping),
        reverse_status_mapping: JSON.stringify(formData.reverse_status_mapping),
        label_sync_mode: formData.label_sync_mode,
        label_mappings: JSON.stringify(formData.label_mappings),
        filter_labels: JSON.stringify(formData.filter_labels),
        assignee_mappings: JSON.stringify(formData.assignee_mappings),
        milestone_mappings: JSON.stringify(formData.milestone_mappings),
        default_item_type_id: formData.default_item_type_id || null,
        default_priority_id: formData.default_priority_id || null,
        sync_comments: formData.sync_comments,
      };

      if (config) {
        config = await api.issueSync.updateConfig(workspaceId, payload);
      } else {
        config = await api.issueSync.createConfig(workspaceId, payload);
      }
      if (config) populateForm(config);
      successToast(t('issueSync.saved'));
    } catch (error) {
      errorToast('Failed to save: ' + (error.message || error));
    } finally {
      saving = false;
    }
  }

  async function deleteConfig() {
    const confirmed = await confirm(t('issueSync.confirmDelete'));
    if (!confirmed) return;
    try {
      await api.issueSync.deleteConfig(workspaceId);
      config = null;
      formData = {
        workspace_repository_id: 0,
        sync_enabled: false,
        status_mapping: { open: null, closed: null },
        reverse_status_mapping: {},
        label_sync_mode: 'none',
        label_mappings: [],
        filter_labels: [],
        assignee_mappings: {},
        milestone_mappings: {},
        default_item_type_id: null,
        default_priority_id: null,
        sync_comments: false,
      };
      syncedItems = [];
      syncStatus = null;
      successToast(t('issueSync.deleted'));
    } catch (error) {
      errorToast('Failed to delete: ' + (error.message || error));
    }
  }

  async function triggerSync() {
    syncing = true;
    try {
      await api.issueSync.triggerSync(workspaceId);
      successToast(t('issueSync.syncTriggered'));
      // Reload status after a brief delay
      setTimeout(async () => {
        await loadSyncDetails();
        syncing = false;
      }, 2000);
    } catch (error) {
      errorToast('Sync failed: ' + (error.message || error));
      syncing = false;
    }
  }

  function addFilterLabel() {
    const label = filterLabelInput.trim();
    if (label && !formData.filter_labels.includes(label)) {
      formData.filter_labels = [...formData.filter_labels, label];
    }
    filterLabelInput = '';
  }

  function removeFilterLabel(label) {
    formData.filter_labels = formData.filter_labels.filter(l => l !== label);
  }

  function addAssigneeMapping() {
    if (newAssigneeGH && newAssigneeWS) {
      formData.assignee_mappings = { ...formData.assignee_mappings, [newAssigneeGH]: newAssigneeWS };
      newAssigneeGH = '';
      newAssigneeWS = null;
    }
  }

  function removeAssigneeMapping(ghUser) {
    const { [ghUser]: _, ...rest } = formData.assignee_mappings;
    formData.assignee_mappings = rest;
  }

  function formatDate(dateStr) {
    if (!dateStr) return t('issueSync.never');
    return new Date(dateStr).toLocaleString();
  }

  let loadingStatuses = $state(false);

  // Reactively load statuses when item type changes
  $effect(() => {
    const itemTypeId = formData.default_item_type_id;
    loadingStatuses = true;
    const url = itemTypeId
      ? `/workspaces/${workspaceId}/statuses?item_type_id=${itemTypeId}`
      : `/workspaces/${workspaceId}/statuses`;
    api.get(url).then(res => {
      statuses = res || [];
    }).catch(() => {
      statuses = [];
    }).finally(() => {
      loadingStatuses = false;
    });
  });

  // Clear stale status mappings when statuses change
  $effect(() => {
    const validIds = new Set(statuses.map(s => s.id));
    // Only reset if we have statuses loaded and current mappings reference invalid IDs
    if (statuses.length > 0) {
      const openValid = formData.status_mapping.open == null || validIds.has(formData.status_mapping.open);
      const closedValid = formData.status_mapping.closed == null || validIds.has(formData.status_mapping.closed);
      if (!openValid || !closedValid) {
        formData.status_mapping = { open: null, closed: null };
        formData.reverse_status_mapping = {};
      }
    }
  });

  const statusOptions = $derived(
    statuses.map(s => ({ value: String(s.id), label: s.name }))
  );

  const repoOptions = $derived(
    linkedRepos.map(r => ({ value: String(r.id), label: r.repository_name }))
  );

  const selectedItemType = $derived(itemTypes.find(it => it.id === formData.default_item_type_id));
  const selectedItemTypeIcon = $derived(
    selectedItemType?.icon ? itemTypeIconMap[selectedItemType.icon] : Layers
  );

  const priorityOptions = $derived(
    priorities.map(p => ({ value: String(p.id), label: p.name }))
  );

</script>

{#if loading}
  <div class="flex items-center justify-center py-12">
    <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
  </div>
{:else if linkedRepos.length === 0}
  <AlertBox type="info">
    <p>{t('issueSync.noLinkedRepos')}</p>
  </AlertBox>
{:else}
  <div class="space-y-6">
    <div>
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{t('issueSync.title')}</h3>
      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{t('issueSync.subtitle')}</p>
    </div>

    <!-- Repository Selection -->
    <Card rounded="lg" padding="spacious">
      <Label>{t('issueSync.repository')}</Label>
      <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">{t('issueSync.repositoryDescription')}</p>
      <select
        bind:value={formData.workspace_repository_id}
        class="w-full px-3 py-2 rounded-md border text-sm"
        style="background: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
      >
        <option value={0}>{t('issueSync.selectRepository')}</option>
        {#each repoOptions as opt}
          <option value={Number(opt.value)}>{opt.label}</option>
        {/each}
      </select>
    </Card>

    <!-- Enable/Disable Toggle -->
    <Card rounded="lg" padding="spacious">
      <div class="flex items-center justify-between">
        <div>
          <Label>{t('issueSync.enabled')}</Label>
          <p class="text-xs" style="color: var(--ds-text-subtle);">{t('issueSync.enabledDescription')}</p>
        </div>
        <Toggle bind:checked={formData.sync_enabled} />
      </div>
    </Card>

    <!-- Item Type Selection -->
    <Card rounded="lg" padding="spacious">
      <Label>{t('issueSync.itemType')}</Label>
      <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">{t('issueSync.itemTypeDescription')}</p>
      <ChipPicker
        value={formData.default_item_type_id}
        items={itemTypes}
        getValue={(t) => t.id}
        getLabel={(t) => t.name}
        icon={selectedItemTypeIcon}
        placeholder={t('issueSync.selectItemType')}
        onSelect={(itemType) => formData.default_item_type_id = itemType.id}
      >
        {#snippet itemSnippet({ item })}
          {@const TypeIcon = item.icon ? itemTypeIconMap[item.icon] : Layers}
          <TypeIcon size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          <span>{item.name}</span>
        {/snippet}
      </ChipPicker>
    </Card>

    {#if formData.default_item_type_id}
      <!-- Status Mapping -->
      <Card rounded="lg" padding="spacious">
        <Label>{t('issueSync.statusMapping')}</Label>
        <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">{t('issueSync.statusMappingDescription')}</p>
        {#if loadingStatuses}
          <div class="flex items-center gap-2 py-2">
            <Loader2 class="w-4 h-4 animate-spin" style="color: var(--ds-text-subtle);" />
            <span class="text-sm" style="color: var(--ds-text-subtle);">Loading statuses...</span>
          </div>
        {:else}
          <div class="space-y-3">
            <div class="flex items-center gap-3">
              <span class="text-sm font-medium w-32" style="color: var(--ds-text);">{t('issueSync.githubOpen')}</span>
              <span class="text-xs" style="color: var(--ds-text-subtle);">{t('issueSync.mapsTo')}</span>
              <select
                value={formData.status_mapping.open ? String(formData.status_mapping.open) : ''}
                onchange={(e) => formData.status_mapping = { ...formData.status_mapping, open: e.currentTarget.value ? Number(e.currentTarget.value) : null }}
                class="flex-1 px-3 py-1.5 rounded-md border text-sm"
                style="background: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
              >
                <option value="">{t('issueSync.selectStatus')}</option>
                {#each statusOptions as opt}
                  <option value={opt.value}>{opt.label}</option>
                {/each}
              </select>
            </div>
            <div class="flex items-center gap-3">
              <span class="text-sm font-medium w-32" style="color: var(--ds-text);">{t('issueSync.githubClosed')}</span>
              <span class="text-xs" style="color: var(--ds-text-subtle);">{t('issueSync.mapsTo')}</span>
              <select
                value={formData.status_mapping.closed ? String(formData.status_mapping.closed) : ''}
                onchange={(e) => formData.status_mapping = { ...formData.status_mapping, closed: e.currentTarget.value ? Number(e.currentTarget.value) : null }}
                class="flex-1 px-3 py-1.5 rounded-md border text-sm"
                style="background: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
              >
                <option value="">{t('issueSync.selectStatus')}</option>
                {#each statusOptions as opt}
                  <option value={opt.value}>{opt.label}</option>
                {/each}
              </select>
            </div>
          </div>
        {/if}
      </Card>

      <!-- Reverse Status Mapping -->
      <Card rounded="lg" padding="spacious">
        <Label>{t('issueSync.reverseStatusMapping')}</Label>
        <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">{t('issueSync.reverseStatusMappingDescription')}</p>
        {#if statuses.length === 0}
          <p class="text-sm py-2" style="color: var(--ds-text-subtle);">{t('issueSync.selectItemTypeHint')}</p>
        {:else}
          <div class="space-y-2">
            {#each statuses as status}
              <div class="flex items-center gap-3">
                <span class="text-sm w-40 truncate" style="color: var(--ds-text);">{status.name}</span>
                <span class="text-xs" style="color: var(--ds-text-subtle);">{t('issueSync.mapsTo')}</span>
                <select
                  value={formData.reverse_status_mapping[String(status.id)] || ''}
                  onchange={(e) => {
                    const m = { ...formData.reverse_status_mapping };
                    if (e.currentTarget.value) {
                      m[String(status.id)] = e.currentTarget.value;
                    } else {
                      delete m[String(status.id)];
                    }
                    formData.reverse_status_mapping = m;
                  }}
                  class="w-32 px-2 py-1 rounded-md border text-sm"
                  style="background: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
                >
                  <option value="">—</option>
                  <option value="open">open</option>
                  <option value="closed">closed</option>
                </select>
              </div>
            {/each}
          </div>
        {/if}
      </Card>
    {/if}

    <!-- Label Sync Mode -->
    <Card rounded="lg" padding="spacious">
      <Label>{t('issueSync.labelSync')}</Label>
      <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">{t('issueSync.labelSyncDescription')}</p>
      <div class="flex gap-4">
        {#each ['none', 'mirror', 'mapped'] as mode}
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              name="label_sync_mode"
              value={mode}
              checked={formData.label_sync_mode === mode}
              onchange={() => formData.label_sync_mode = mode}
              class="accent-blue-600"
            />
            <span class="text-sm" style="color: var(--ds-text);">{t(`issueSync.labelMode${mode.charAt(0).toUpperCase() + mode.slice(1)}`)}</span>
          </label>
        {/each}
      </div>
    </Card>

    <!-- Filter Labels -->
    <Card rounded="lg" padding="spacious">
      <Label>{t('issueSync.filterLabels')}</Label>
      <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">{t('issueSync.filterLabelsDescription')}</p>
      <div class="flex gap-2 mb-2">
        <input
          type="text"
          bind:value={filterLabelInput}
          placeholder={t('issueSync.filterLabelsPlaceholder')}
          onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addFilterLabel(); }}}
          class="flex-1 px-3 py-1.5 rounded-md border text-sm"
          style="background: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
        />
        <Button size="sm" onclick={addFilterLabel}>
          <Plus class="w-4 h-4" />
        </Button>
      </div>
      {#if formData.filter_labels.length > 0}
        <div class="flex flex-wrap gap-2">
          {#each formData.filter_labels as label}
            <Chip color="gray" removable onRemove={() => removeFilterLabel(label)}>
              {label}
            </Chip>
          {/each}
        </div>
      {/if}
    </Card>

    <!-- Assignee Mapping -->
    <Card rounded="lg" padding="spacious">
      <Label>{t('issueSync.assigneeMapping')}</Label>
      <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">{t('issueSync.assigneeMappingDescription')}</p>
      {#each Object.entries(formData.assignee_mappings) as [ghUser, wsUserId]}
        <div class="flex items-center gap-2 mb-2">
          <span class="text-sm px-2 py-1 rounded" style="background: var(--ds-surface-sunken); color: var(--ds-text);">{ghUser}</span>
          <span class="text-xs" style="color: var(--ds-text-subtle);">→</span>
          <span class="text-sm" style="color: var(--ds-text);">{users.find(u => u.id === wsUserId)?.display_name || users.find(u => u.id === wsUserId)?.username || wsUserId}</span>
          <button onclick={() => removeAssigneeMapping(ghUser)} class="ml-auto hover:opacity-70" style="color: var(--ds-text-subtle);">
            <X class="w-4 h-4" />
          </button>
        </div>
      {/each}
      <div class="flex items-center gap-2 mt-2">
        <input
          type="text"
          bind:value={newAssigneeGH}
          placeholder={t('issueSync.githubUsername')}
          class="w-40 px-2 py-1.5 rounded-md border text-sm"
          style="background: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
        />
        <span class="text-xs" style="color: var(--ds-text-subtle);">→</span>
        <UserPicker
          bind:value={newAssigneeWS}
          placeholder={t('issueSync.windshiftUser')}
          workspaceId={workspaceId}
        />
        <Button size="sm" onclick={addAssigneeMapping} disabled={!newAssigneeGH || !newAssigneeWS}>
          <Plus class="w-4 h-4" />
        </Button>
      </div>
    </Card>

    <!-- Default Priority -->
    <Card rounded="lg" padding="spacious">
      <Label>{t('issueSync.defaultPriority')}</Label>
      <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">{t('issueSync.defaultPriorityDescription')}</p>
      <select
        value={formData.default_priority_id ? String(formData.default_priority_id) : ''}
        onchange={(e) => formData.default_priority_id = e.currentTarget.value ? Number(e.currentTarget.value) : null}
        class="w-full px-3 py-2 rounded-md border text-sm"
        style="background: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
      >
        <option value="">—</option>
        {#each priorityOptions as opt}
          <option value={opt.value}>{opt.label}</option>
        {/each}
      </select>
    </Card>

    <!-- Comment Sync -->
    <Card rounded="lg" padding="spacious">
      <div class="flex items-center justify-between">
        <div>
          <Label>{t('issueSync.comments')}</Label>
          <p class="text-xs" style="color: var(--ds-text-subtle);">{t('issueSync.commentsDescription')}</p>
        </div>
        <Toggle bind:checked={formData.sync_comments} />
      </div>
    </Card>

    <!-- Sync Status Panel -->
    {#if config}
      <Card rounded="lg" padding="spacious">
        <div class="flex items-center justify-between mb-3">
          <Label>{t('issueSync.syncStatus')}</Label>
          <Button
            size="sm"
            variant="secondary"
            onclick={triggerSync}
            disabled={syncing}
          >
            {#if syncing}
              <Loader2 class="w-4 h-4 animate-spin mr-1" />
              {t('issueSync.syncing')}
            {:else}
              <RefreshCw class="w-4 h-4 mr-1" />
              {t('issueSync.syncNow')}
            {/if}
          </Button>
        </div>
        <div class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span style="color: var(--ds-text-subtle);">{t('issueSync.lastSync')}</span>
            <span style="color: var(--ds-text);">{formatDate(syncStatus?.last_sync_at)}</span>
          </div>
          <div class="flex justify-between">
            <span style="color: var(--ds-text-subtle);">{t('issueSync.lastSyncError')}</span>
            <span style="color: {syncStatus?.last_sync_error ? 'var(--ds-text-danger)' : 'var(--ds-text-success)'};">
              {syncStatus?.last_sync_error || t('issueSync.noErrors')}
            </span>
          </div>
          <div class="flex justify-between">
            <span style="color: var(--ds-text-subtle);">{t('issueSync.syncedItems')}</span>
            <span style="color: var(--ds-text);">{syncStatus?.synced_item_count || 0}</span>
          </div>
        </div>

        <!-- Synced Items List -->
        {#if syncedItems.length > 0}
          <div class="mt-4 border-t pt-3" style="border-color: var(--ds-border);">
            <div class="max-h-60 overflow-y-auto space-y-1">
              {#each syncedItems as item}
                <div class="flex items-center justify-between py-1 text-sm">
                  <span style="color: var(--ds-text);">
                    <span class="font-medium" style="color: var(--ds-text-subtle);">{item.workspace_key}-{item.workspace_item_number}</span>
                    {item.item_title}
                  </span>
                  <a href={safeHref(item.github_issue_url)} target="_blank" rel="noopener noreferrer" class="flex items-center gap-1 text-xs hover:underline" style="color: var(--ds-link);">
                    #{item.github_issue_number}
                    <ExternalLink class="w-3 h-3" />
                  </a>
                </div>
              {/each}
            </div>
          </div>
        {/if}
      </Card>
    {/if}

    <!-- Action Buttons -->
    <div class="flex items-center justify-between">
      <div>
        {#if config}
          <Button variant="danger" size="sm" onclick={deleteConfig}>
            <Trash2 class="w-4 h-4 mr-1" />
            {t('issueSync.deleteConfig')}
          </Button>
        {/if}
      </div>
      <Button
        variant="primary"
        onclick={saveConfig}
        disabled={saving || !formData.workspace_repository_id}
      >
        {#if saving}
          <Loader2 class="w-4 h-4 animate-spin mr-1" />
        {/if}
        {t('issueSync.save')}
      </Button>
    </div>
  </div>
{/if}
