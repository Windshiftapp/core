<script>
  import ItemPicker from './ItemPicker.svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { milestonesStore } from '../stores/milestones.js';
  import { permissionStore, isSystemAdmin } from '../stores/permissions.svelte.js';
  import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
  import MilestoneFormDialog from '../features/milestones/MilestoneFormDialog.svelte';
  import { Plus } from '@lucide/svelte';
  import Checkbox from '../components/Checkbox.svelte';

  // When `multiple` is false (the default), `value` is a single milestone ID
  // (or null) and onSelect emits `{ value, milestone }`.
  // When `multiple` is true, `value` is an array of milestone IDs and
  // onSelect emits `{ ids, milestones }`.
  let {
    value = $bindable(null),
    placeholder = '',
    class: className = '',
    disabled = false,
    workspaceId = null,
    milestones: providedMilestones = null,
    loading: providedLoading = false,
    showUnassigned = true,
    unassignedLabel = '',
    children = null,
    multiple = false,
    onOpen = null,
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(
    placeholder || (multiple ? t('pickers.selectMilestones') : t('pickers.selectMilestone'))
  );
  const resolvedUnassignedLabel = $derived(unassignedLabel || t('pickers.noMilestone'));

  let loadedMilestones = $state([]);
  let createdMilestones = $state([]);
  let internalLoading = $state(false);
  let loadToken = 0;
  let previousWorkspaceId = $state(null);
  const baseMilestones = $derived(providedMilestones ?? loadedMilestones);
  const milestones = $derived.by(() => {
    const seen = new Set();
    return [...baseMilestones, ...createdMilestones].filter((milestone) => {
      if (seen.has(milestone.id)) return false;
      seen.add(milestone.id);
      return true;
    });
  });
  const loading = $derived(providedMilestones === null ? internalLoading : providedLoading);

  $effect(() => {
    if (previousWorkspaceId === workspaceId) return;
    previousWorkspaceId = workspaceId;
    createdMilestones = [];
  });

  const hasWorkspaceContext = $derived(workspaceId !== null && workspaceId !== undefined && workspaceId !== '');
  const canCreateGlobal = $derived(
    $permissionStore.userPermissionKeys?.has('milestone.create') || $isSystemAdmin
  );
  const canCreateWorkspace = $derived(
    hasWorkspaceContext && (
      workspacePermissions.canAdminWorkspace(workspaceId) ||
      workspacePermissions.hasPermission(workspaceId, 'item.edit')
    )
  );
  const canCreate = $derived(
    hasWorkspaceContext ? canCreateWorkspace || canCreateGlobal : canCreateGlobal
  );

  let showCreateDialog = $state(false);
  let savingCreate = $state(false);
  let createFormData = $state({
    name: '',
    description: '',
    target_date: '',
    status: 'planning',
    category_id: null,
    is_global: true,
    workspace_id: null
  });

  // Terminal milestone lifecycle statuses are hidden from the list by default
  // so completed/cancelled milestones no longer look identical to active ones
  // (WI-448). They can be surfaced again via the "Show completed" footer toggle.
  const TERMINAL_STATUSES = new Set(['completed', 'cancelled']);

  // Whether the user has opted to surface terminal milestones.
  let showCompleted = $state(false);

  // Reload when workspaceId changes. Capture the ID for this request so an
  // earlier global load cannot overwrite a later workspace-scoped result.
  $effect(() => {
    if (providedMilestones !== null) return;

    const controller = new AbortController();
    loadMilestones(workspaceId, { signal: controller.signal });
    return () => controller.abort();
  });

  async function loadMilestones(currentWorkspaceId, requestOptions = {}) {
    const token = ++loadToken;
    internalLoading = true;

    try {
      const filters = {};
      if (currentWorkspaceId) {
        filters.workspace_id = currentWorkspaceId;
        filters.include_global = true;
      }

      const response = await api.milestones.getAll(filters, requestOptions);
      if (token !== loadToken) return;
      loadedMilestones = response || [];
      createdMilestones = [];
    } catch (err) {
      if (token !== loadToken || err?.name === 'AbortError') return;
      console.error('Failed to load milestones:', err);
      loadedMilestones = [];
    } finally {
      if (token === loadToken) {
        internalLoading = false;
      }
    }
  }

  function handleSelectSingle(milestone) {
    onSelect({
      value: milestone ? milestone.id : null,
      milestone: milestone || null
    });
  }

  function handleSelectMulti(ids) {
    const safe = Array.isArray(ids) ? ids : [];
    const selected = safe
      .map((id) => milestones.find((m) => m.id === id))
      .filter(Boolean);
    onSelect({ ids: safe, milestones: selected });
  }

  function createScope() {
    const workspace = hasWorkspaceContext ? Number.parseInt(String(workspaceId), 10) : null;
    const isGlobal = !hasWorkspaceContext || !canCreateWorkspace;
    return {
      is_global: isGlobal,
      workspace_id: isGlobal || !Number.isFinite(workspace) ? null : workspace
    };
  }

  function openCreateDialog(name) {
    if (!canCreate) return;
    const scope = createScope();
    createFormData = {
      name: name.trim(),
      description: '',
      target_date: '',
      status: 'planning',
      category_id: null,
      ...scope
    };
    showCreateDialog = true;
  }

  async function saveCreatedMilestone() {
    if (savingCreate) return;
    savingCreate = true;

    try {
      const data = {
        ...createFormData,
        target_date: createFormData.target_date || null
      };
      if (data.is_global) data.workspace_id = null;

      const created = await milestonesStore.add(data);
      createdMilestones = [...createdMilestones, created];

      if (multiple) {
        const nextIds = [
          ...(Array.isArray(value) ? value : []),
          created.id
        ];
        value = nextIds;
        onSelect({
          ids: nextIds,
          milestones: nextIds.map((id) => milestones.find((item) => item.id === id)).filter(Boolean)
        });
      } else {
        value = created.id;
        onSelect({ value: created.id, milestone: created });
      }

      showCreateDialog = false;
    } catch (error) {
      console.error('Failed to create milestone:', error);
      errorToast(error?.message || String(error), t('errors.failedToSave'));
    } finally {
      savingCreate = false;
    }
  }

  // Terminal milestones hidden by default; surfaced via the footer toggle.
  const hasCompletedMilestones = $derived(
    milestones.some((m) => TERMINAL_STATUSES.has(m.status))
  );

  // The currently-selected value(s) must always stay visible so the trigger
  // reflects the real selection accurately, even when it would otherwise be
  // filtered out as a terminal milestone.
  const selectedIds = $derived.by(() => {
    const ids = new Set();
    if (multiple) {
      const arr = Array.isArray(value) ? value : [];
      for (const id of arr) ids.add(id);
    } else if (value != null) {
      ids.add(value);
    }
    return ids;
  });

  const visibleMilestones = $derived.by(() => {
    if (showCompleted) return milestones;
    return milestones.filter(
      (m) => !TERMINAL_STATUSES.has(m.status) || selectedIds.has(m.id)
    );
  });

  // Status-specific badge so a cancelled milestone is never mislabelled as
  // "Completed". Cancelled uses the danger tone, completed the success tone,
  // matching the milestone detail page's lozenge colours. Backgrounds are a
  // subtle tint derived from the well-defined text tokens (there is no
  // `--ds-background-success`/`-danger` semantic token in the theme).
  function terminalBadge(milestone) {
    if (milestone.status === 'completed') {
      return {
        text: t('milestones.status.completed'),
        textColor: 'var(--ds-text-success)',
        bgColor: 'color-mix(in srgb, var(--ds-text-success) 15%, transparent)'
      };
    }
    if (milestone.status === 'cancelled') {
      return {
        text: t('milestones.status.cancelled'),
        textColor: 'var(--ds-text-danger)',
        bgColor: 'color-mix(in srgb, var(--ds-text-danger) 15%, transparent)'
      };
    }
    return null;
  }

  const config = {
    icon: {
      type: 'color-dot',
      source: (item) => item.category_color || '#9CA3AF',
      size: 'w-2 h-2'
    },
    primary: { text: (item) => item.name || '' },
    badges: [
      {
        text: (item) => terminalBadge(item)?.text ?? '',
        textColor: (item) => terminalBadge(item)?.textColor ?? 'var(--ds-text-subtle)',
        bgColor: (item) => terminalBadge(item)?.bgColor ?? 'var(--ds-background-neutral)'
      }
    ],
    searchFields: ['name', 'description'],
    getValue: (item) => item?.id,
    getLabel: (item) => item?.name ?? ''
  };
</script>

{#if multiple}
  <ItemPicker
    bind:values={value}
    items={visibleMilestones}
    {config}
    placeholder={resolvedPlaceholder}
    showUnassigned={false}
    {disabled}
    {loading}
    multiSelect={true}
    class={className}
    {children}
    keepOpenOnFooterTab={hasCompletedMilestones}
    allowCreate={canCreate}
    onCreate={openCreateDialog}
    onSelect={handleSelectMulti}
    onOpen={() => onOpen?.()}
    onCancel={() => onCancel()}
  >
    {#snippet footer()}
      {#if hasCompletedMilestones}
        <Checkbox
          bind:checked={showCompleted}
          label={t('pickers.showCompletedMilestones')}
          class="px-4 py-2.5"
          dataTestid="milestone-show-completed-toggle"
          size="small"
        />
      {/if}
    {/snippet}

    {#snippet noResultsSnippet({ searchQuery, onCreate })}
      <div class="p-4 text-center text-sm" style="color: var(--ds-text-subtle);">
        <div>{t('pickers.noResultsFor', { query: searchQuery })}</div>
        {#if canCreate}
          <button
            type="button"
            data-testid="milestone-create-option"
            class="inline-flex items-center gap-2 mt-3 px-3 py-1.5 rounded transition-colors"
            style="background-color: var(--ds-background-accent-blue-subtlest); color: var(--ds-interactive);"
            onclick={onCreate}
          >
            <Plus class="w-4 h-4" />
            {t('pickers.createItem', { value: searchQuery })}
          </button>
        {/if}
      </div>
    {/snippet}
  </ItemPicker>
{:else}
  <ItemPicker
    bind:value
    items={visibleMilestones}
    {config}
    placeholder={resolvedPlaceholder}
    {showUnassigned}
    unassignedLabel={resolvedUnassignedLabel}
    {disabled}
    {loading}
    allowClear={true}
    class={className}
    {children}
    keepOpenOnFooterTab={hasCompletedMilestones}
    allowCreate={canCreate}
    onCreate={openCreateDialog}
    onSelect={handleSelectSingle}
    onOpen={() => onOpen?.()}
    onCancel={() => onCancel()}
  >
    {#snippet footer()}
      {#if hasCompletedMilestones}
        <Checkbox
          bind:checked={showCompleted}
          label={t('pickers.showCompletedMilestones')}
          class="px-4 py-2.5"
          dataTestid="milestone-show-completed-toggle"
          size="small"
        />
      {/if}
    {/snippet}

    {#snippet noResultsSnippet({ searchQuery, onCreate })}
      <div class="p-4 text-center text-sm" style="color: var(--ds-text-subtle);">
        <div>{t('pickers.noResultsFor', { query: searchQuery })}</div>
        {#if canCreate}
          <button
            type="button"
            data-testid="milestone-create-option"
            class="inline-flex items-center gap-2 mt-3 px-3 py-1.5 rounded transition-colors"
            style="background-color: var(--ds-background-accent-blue-subtlest); color: var(--ds-interactive);"
            onclick={onCreate}
          >
            <Plus class="w-4 h-4" />
            {t('pickers.createItem', { value: searchQuery })}
          </button>
        {/if}
      </div>
    {/snippet}
  </ItemPicker>
{/if}

<MilestoneFormDialog
  bind:isOpen={showCreateDialog}
  bind:formData={createFormData}
  workspaceId={workspaceId}
  isGlobalView={!hasWorkspaceContext}
  canManageGlobal={canCreateGlobal}
  canManageWorkspace={canCreateWorkspace}
  saving={savingCreate}
  onSubmit={saveCreatedMilestone}
/>
