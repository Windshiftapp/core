<script>
  import { onDestroy, onMount, untrack } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { collections } from '../../api/collections.js';
  import { items as itemsApi } from '../../api/items.js';
  import { teams as teamsApi } from '../../api/teams.js';
  import { fetchV2Data } from '../../api/core.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { authStore } from '../../stores/auth.svelte.js';
  import { collectionStore } from '../../stores/collectionContext.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import { navigate } from '../../router.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { useCollectionEventStream } from '../../composables/useCollectionEventStream.svelte.js';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import ArrowRight from '@lucide/svelte/icons/arrow-right';
  import EyeOff from '@lucide/svelte/icons/eye-off';
  import MoreHorizontal from '@lucide/svelte/icons/more-horizontal';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Badge from '../../components/Badge.svelte';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import TeamPicker from '../../pickers/TeamPicker.svelte';
  import { objectDisplayName } from '../../utils/systemLabels.js';
  import Modal from '../../dialogs/Modal.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import { createDeleteItemHandler, createItemActionsBuilder } from '../../utils/workItemTableHelpers.js';
  import {
    buildListColumnConfiguration,
    DEFAULT_LIST_COLUMNS,
    getListColumnLabel,
    listColumnsFromConfig,
    listGridMinWidth,
    listGridTemplateColumns,
  } from '../../utils/workItemListColumns.js';
  import { useGradientStyles } from '../../stores/workspaceGradient.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { collectionEditorOptions } from '../../stores/collectionEditorOptions.svelte.js';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import TableHeaderBar from '../../components/TableHeaderBar.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import LazyRender from '../../components/LazyRender.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import ListCellRenderer from '../collections/ListCellRenderer.svelte';
  import ColumnSelector from '../collections/ColumnSelector.svelte';
  import QlFilterBuilder from '../shared/QlFilterBuilder.svelte';

  let { workspaceId, collectionId = null, queue = null } = $props();

  // Resolved queue catalog for the current scope (built-ins + custom).
  let queues = $state([]);
  let activeRef = $state('');
  let loadingQueues = $state(true);

  // Ticket rows for the active queue. Paging uses the same page/page_size
  // contract the /items endpoint returns, so the list's Pagination component
  // drives the queue too.
  const QUEUE_PAGE_SIZE = 100;
  let items = $state([]);
  let loadingItems = $state(false);
  let itemsPagination = $state(null);
  let itemsLoadId = 0;
  let currentPage = $state(1);
  let itemsPerPage = $state(QUEUE_PAGE_SIZE);

  // Bulk selection state.
  let selectedIds = $state(new Set());
  let bulkRunning = $state(false);
  let bulkTeamId = $state(null);
  let bulkStatusId = $state('');
  let teams = $state([]);

  // Server-side search, matching the board and list views (GH #328).
  let searchQuery = $state('');
  let searchDebouncing = $state(false);
  let searchTimer = null;

  // Queue authoring dialog.
  let editorOpen = $state(false);
  let editorQueueId = $state(null);
  let editorName = $state('');
  let editorQl = $state('');
  let editorFilterState = $state(null);
  let editorSaving = $state(false);

  // Collection metadata gates management in a collection context; workspace
  // scope mirrors the board-configuration admin gate.
  let collection = $state(null);

  // Reference data from the shared workspace store.
  let workspace = $derived(workspaceDataStore.workspace);
  let users = $derived(workspaceDataStore.users ?? []);
  let statuses = $derived(workspaceDataStore.statuses ?? []);
  let statusCategories = $derived(workspaceDataStore.statusCategories ?? []);
  let priorities = $derived(workspaceDataStore.priorities ?? []);
  let milestones = $derived(workspaceDataStore.milestones ?? []);
  let iterations = $derived(workspaceDataStore.iterations ?? []);
  let projects = $derived(workspaceDataStore.projects ?? []);
  let itemTypes = $derived(workspaceDataStore.itemTypes ?? []);
  let customFieldDefinitions = $derived(workspaceDataStore.customFieldDefinitions ?? []);

  let activeQueue = $derived(queues.find((entry) => queueRef(entry) === activeRef) ?? null);
  let activeQueueCount = $derived(activeQueue?.count ?? 0);
  let statusOptions = $derived(
    statuses.map((status) => ({ value: String(status.id), label: objectDisplayName(status, 'status') }))
  );
  let allSelected = $derived(items.length > 0 && items.every((item) => selectedIds.has(item.id)));
  let currentUser = $derived(authStore.currentUser);
  let canEdit = $derived(workspacePermissions.canEdit(workspaceId));
  let canConfigureColumns = $derived(workspacePermissions.canAdminWorkspace(workspaceId));
  let canManage = $derived.by(() => {
    if (collectionId) {
      return Boolean(
        collection &&
          collection.created_by != null &&
          String(collection.created_by) === String(currentUser?.id)
      );
    }
    return workspacePermissions.canAdminWorkspace(workspaceId);
  });

  // The queue renders list-view rows, so it reuses the list view's configurable
  // columns (board_configurations.list_columns). Assignee and updated keep the
  // queue's own labels when those columns are present.
  let boardConfig = $state(null);
  let listColumns = $state([...DEFAULT_LIST_COLUMNS]);
  const QUEUE_COLUMN_LABEL_OVERRIDES = {
    assignee: 'supportQueue.columnOwner',
    updated_at: 'supportQueue.columnUpdated',
  };

  function columnLabel(column) {
    const override = QUEUE_COLUMN_LABEL_OVERRIDES[column.field_identifier];
    return override ? t(override) : getListColumnLabel(column, customFieldDefinitions);
  }

  // A leading selection track sits in front of the shared list grid tracks.
  const SELECT_TRACK = '2rem';
  let gridTemplateColumns = $derived(`${SELECT_TRACK} ${listGridTemplateColumns(listColumns)}`);
  let gridMinWidth = $derived(`calc(${SELECT_TRACK} + ${listGridMinWidth(listColumns)})`);

  const styles = useGradientStyles();

  // Queue tabs reuse the tree's glass button treatment so labels and counts
  // stay legible over custom gradient backgrounds. The active tab keeps the
  // design system's highlight as an overlay on top of the glass surface.
  function queueTabStyle(isActive) {
    const base = `${styles.glassStyle(12)} ${styles.glassTextStyle}`;
    return isActive
      ? `${base} font-weight: 500; background-image: linear-gradient(var(--ctx-active-bg, transparent), var(--ctx-active-bg, transparent));`
      : base;
  }

  function handleSearchInput() {
    if (searchTimer) clearTimeout(searchTimer);
    searchDebouncing = true;
    searchTimer = setTimeout(() => {
      searchTimer = null;
      searchDebouncing = false;
      void loadItems({ page: 1 });
    }, 250);
  }

  onDestroy(() => {
    if (searchTimer) clearTimeout(searchTimer);
  });

  // Built-in presets are addressed by their stable key; custom queues by id.
  function queueRef(entry) {
    return entry.builtin ? entry.key : String(entry.id);
  }

  function queuesPath() {
    if (collectionId) return `/collections/${collectionId}/queues`;
    const key = workspaceDataStore.workspace?.key;
    return key ? `/workspaces/${key}/queues` : null;
  }

  // A workspace-scoped list already has these option sets in the shared
  // workspace store. Prime the row-editor cache so opening a cell does not
  // repeat those requests, exactly like the list view does.
  $effect(() => {
    const id = Number(workspaceId);
    if (
      !Number.isInteger(id) ||
      id <= 0 ||
      !workspaceDataStore.initialized ||
      Number(workspaceDataStore.workspaceId) !== id
    ) {
      return;
    }

    untrack(() =>
      collectionEditorOptions.prime(
        id,
        {
          statuses,
          users,
          milestones,
          iterations,
          projects,
        },
        workspaceDataStore.planningCompleteness
      )
    );
  });

  async function loadCollection() {
    if (!collectionId) {
      collection = null;
      return;
    }
    try {
      collection = await collections.get(collectionId);
    } catch (error) {
      collection = null;
    }
  }

  // Reuses the list view's column configuration so both views stay in sync.
  async function loadBoardConfiguration() {
    try {
      const config = await collections.getBoardConfiguration(collectionId, workspaceId);
      boardConfig = config;
      listColumns = listColumnsFromConfig(config);
    } catch (error) {
      boardConfig = null;
      listColumns = [...DEFAULT_LIST_COLUMNS];
      if (error?.status !== 404) {
        console.error('Failed to load list column configuration:', error);
      }
    }
  }

  async function saveBoardConfiguration(newColumns) {
    try {
      const configData = buildListColumnConfiguration(boardConfig, newColumns);
      const saved = boardConfig?.id
        ? await collections.updateBoardConfiguration(
            collectionId,
            boardConfig.id,
            configData,
            workspaceId
          )
        : await collections.createBoardConfiguration(collectionId, workspaceId, configData);
      boardConfig = saved;
      listColumns = listColumnsFromConfig(saved);
      // The list view reads this scope's configuration through the shared
      // store; drop its cache so it picks up the queue's change.
      collectionStore.invalidateBoardConfiguration(workspaceId, collectionId);
    } catch (error) {
      console.error('Failed to save list column configuration:', error);
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  function handleColumnChange({ columns }) {
    void saveBoardConfiguration(columns);
  }

  async function loadQueues() {
    try {
      await workspaceDataStore.initialize(workspaceId);
      const path = queuesPath();
      if (!path) throw new Error('workspace bootstrap unavailable');
      queues = (await fetchV2Data(path)) ?? [];
      resolveActiveRef();
    } catch (error) {
      errorToast(t('supportQueue.loadFailed'));
    } finally {
      loadingQueues = false;
    }
  }

  // Keep the selected queue when it still exists; otherwise honor the deep
  // link, then fall back to the unassigned preset, then the first queue.
  function resolveActiveRef() {
    const refs = queues.map(queueRef);
    if (activeRef && refs.includes(activeRef)) return;
    if (queue && refs.includes(queue)) {
      activeRef = queue;
      return;
    }
    activeRef = refs.includes('unassigned') ? 'unassigned' : (refs[0] ?? '');
  }

  async function loadItems({ page = currentPage, limit = itemsPerPage, search = searchQuery.trim() } = {}) {
    if (!activeQueue?.ql) {
      items = [];
      itemsPagination = null;
      return;
    }
    const loadId = ++itemsLoadId;
    loadingItems = true;
    try {
      const response = await itemsApi.getAll({
        workspace_id: workspaceId,
        ql: activeQueue.ql,
        page,
        limit,
        omit_descriptions: true,
        order_by: 'updated_at',
        sort_direction: 'desc',
        ...(search ? { search } : {}),
      });
      if (loadId !== itemsLoadId) return;
      items = response?.data ?? [];
      // The v2 document names the effective size page_size; the shared
      // Pagination component reads limit.
      const pagination = response?.pagination ?? null;
      itemsPagination = pagination ? { ...pagination, limit: pagination.page_size } : null;
      currentPage = pagination?.page ?? page;
      itemsPerPage = pagination?.page_size ?? limit;
      selectedIds = new Set(
        [...selectedIds].filter((id) => items.some((item) => item.id === id))
      );
    } catch (error) {
      if (loadId !== itemsLoadId) return;
      items = [];
      itemsPagination = null;
      errorToast(t('supportQueue.loadFailed'));
    } finally {
      if (loadId === itemsLoadId) loadingItems = false;
    }
  }

  function handlePageChange(event) {
    void loadItems({ page: event.detail.page, limit: event.detail.itemsPerPage });
  }

  function handlePageSizeChange(event) {
    void loadItems({ page: event.detail.page, limit: event.detail.itemsPerPage });
  }

  async function refreshQueuesAndItems() {
    await loadQueues();
    await loadItems();
  }

  // Scoped SSE invalidations (WI-1624): a ticket deleted from the item detail
  // modal emits no local event, so the queue reconciles through the same
  // collection/workspace stream the board and list views use. Without this the
  // row and tab counts kept showing deleted tickets.
  useCollectionEventStream(
    () => (collectionId ? { kind: 'collection', id: collectionId } : { kind: 'workspace', id: workspaceId }),
    { onInvalidate: () => refreshQueuesAndItems() }
  );

  function selectQueue(ref) {
    if (ref === activeRef) return;
    activeRef = ref;
    selectedIds = new Set();
    if (searchTimer) {
      clearTimeout(searchTimer);
      searchTimer = null;
    }
    searchDebouncing = false;
    searchQuery = '';
    const url = new URL(window.location.href);
    url.searchParams.set('queue', ref);
    window.history.replaceState(null, '', `${url.pathname}${url.search}`);
    void loadItems({ page: 1 });
  }

  function toggleItem(id) {
    const next = new Set(selectedIds);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    selectedIds = next;
  }

  function toggleAll() {
    selectedIds = allSelected ? new Set() : new Set(items.map((item) => item.id));
  }

  function clearSelection() {
    selectedIds = new Set();
  }

  async function runBulkUpdate(fields) {
    const ids = [...selectedIds];
    if (ids.length === 0 || bulkRunning) return;
    bulkRunning = true;
    try {
      await itemsApi.bulkUpdate(ids, fields);
      successToast(t('supportQueue.bulkSuccess', { n: ids.length }));
      clearSelection();
      await refreshQueuesAndItems();
    } catch (error) {
      errorToast(error?.message || t('supportQueue.loadFailed'));
    } finally {
      bulkRunning = false;
    }
  }

  function bulkAssignToMe() {
    if (currentUser?.id == null) return;
    void runBulkUpdate({ assignee_id: currentUser.id });
  }

  function bulkAssignToTeam() {
    if (!bulkTeamId) return;
    void runBulkUpdate({ team_id: bulkTeamId });
  }

  // One request moves every selected item; the server enforces workflow rules
  // per item and reports which ones could not move instead of the client
  // fanning out one transition call per ticket.
  async function bulkTransition() {
    const ids = [...selectedIds];
    if (ids.length === 0 || bulkRunning || !bulkStatusId) return;
    bulkRunning = true;
    try {
      const result = await itemsApi.bulkTransition(ids, Number(bulkStatusId));
      if (result.failed_count > 0) {
        errorToast(
          t('supportQueue.bulkPartial', {
            updated: result.transitioned_count,
            requested: result.requested_count,
            failed: result.failed_count,
          })
        );
      } else {
        successToast(t('supportQueue.bulkTransitionSuccess', { n: result.transitioned_count }));
      }
      clearSelection();
      await refreshQueuesAndItems();
    } catch (error) {
      errorToast(error?.message || t('supportQueue.loadFailed'));
    } finally {
      bulkRunning = false;
    }
  }

  // One request deletes every selected item; the server keeps each item's
  // single-delete history and audit record and reports any that were refused.
  async function bulkDelete() {
    const ids = [...selectedIds];
    if (ids.length === 0 || bulkRunning) return;
    const confirmed = await confirm({
      title: t('supportQueue.bulkDelete'),
      message: t('supportQueue.bulkDeleteConfirm', { n: ids.length }),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!confirmed) return;
    bulkRunning = true;
    try {
      const result = await itemsApi.bulkDelete(ids);
      if (result.failed_count > 0) {
        errorToast(
          t('supportQueue.bulkPartial', {
            updated: result.deleted_count,
            requested: result.requested_count,
            failed: result.failed_count,
          })
        );
      } else {
        successToast(t('supportQueue.bulkDeleteSuccess', { n: result.deleted_count }));
      }
      clearSelection();
      await refreshQueuesAndItems();
    } catch (error) {
      errorToast(error?.message || t('supportQueue.loadFailed'));
    } finally {
      bulkRunning = false;
    }
  }

  function viewItem(item) {
    navigate(`/workspaces/${workspaceId}/items/${item.id}`);
  }

  // Single-cell edits replace the row in place; counts may drift until the
  // next bulk refresh, matching the list view's update behavior.
  function handleItemUpdated(data) {
    items = items.map((item) => (item.id === data.item.id ? { ...item, ...data.item } : item));
  }

  function handleUpdateError(data) {
    const { error, field, value } = data;
    console.error(`Failed to update ${field}:`, error);
    errorToast(t('dialogs.alerts.failedToUpdate', { error: `${field}: ${value ?? ''} ${error}` }));
  }

  const deleteItem = createDeleteItemHandler({
    confirmMessage: (item) => t('collections.confirmDeleteItem', { title: item.title }),
    onDeleted: () => refreshQueuesAndItems(),
  });

  const buildItemActions = createItemActionsBuilder({ viewItem, deleteItem });

  // ===== Queue management =====

  function openCreate() {
    editorQueueId = null;
    editorName = '';
    editorQl = '';
    editorFilterState = null;
    editorOpen = true;
  }

  function openEdit(entry) {
    editorQueueId = entry.id;
    editorName = entry.name;
    editorQl = entry.ql;
    editorFilterState = entry.filter_state ?? null;
    editorOpen = true;
  }

  function closeEditor() {
    if (editorSaving) return;
    editorOpen = false;
  }

  function handleEditorChange({ ql_query, filter_state }) {
    editorQl = ql_query;
    editorFilterState = filter_state;
  }

  async function saveEditor() {
    if (!editorName.trim() || !editorQl.trim() || editorSaving) return;
    editorSaving = true;
    const body = JSON.stringify({
      name: editorName.trim(),
      ql_query: editorQl,
      filter_state: editorFilterState,
    });
    try {
      if (editorQueueId) {
        await fetchV2Data(`/queues/${editorQueueId}`, { method: 'PATCH', body });
        successToast(t('supportQueue.updated'));
      } else {
        const path = queuesPath();
        await fetchV2Data(path, { method: 'POST', body });
        successToast(t('supportQueue.created'));
      }
      editorOpen = false;
      await refreshQueuesAndItems();
    } catch (error) {
      errorToast(error?.message || t('supportQueue.saveFailed'));
    } finally {
      editorSaving = false;
    }
  }

  async function removeQueue(entry) {
    const confirmed = await confirm({
      title: t('supportQueue.delete'),
      message: t('supportQueue.deleteConfirm', { name: entry.name }),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await fetchV2Data(`/queues/${entry.id}`, { method: 'DELETE' });
      successToast(t('supportQueue.deleted'));
      await refreshQueuesAndItems();
    } catch (error) {
      errorToast(error?.message || t('supportQueue.saveFailed'));
    }
  }

  async function dismissBuiltin(entry) {
    const path = queuesPath();
    try {
      await fetchV2Data(`${path}/builtins/${encodeURIComponent(entry.key)}`, {
        method: 'PUT',
        body: JSON.stringify({ hidden: true }),
      });
      successToast(t('supportQueue.builtinRemoved'));
      await refreshQueuesAndItems();
    } catch (error) {
      errorToast(error?.message || t('supportQueue.saveFailed'));
    }
  }

  async function moveQueue(entry, direction) {
    const custom = queues.filter((item) => !item.builtin);
    const index = custom.findIndex((item) => item.id === entry.id);
    const target = direction === 'left' ? index - 1 : index + 1;
    if (index < 0 || target < 0 || target >= custom.length) return;
    const reordered = [...custom];
    [reordered[index], reordered[target]] = [reordered[target], reordered[index]];
    try {
      await fetchV2Data(`${queuesPath()}/order`, {
        method: 'PUT',
        body: JSON.stringify({ ids: reordered.map((item) => item.id) }),
      });
      await refreshQueuesAndItems();
    } catch (error) {
      errorToast(error?.message || t('supportQueue.saveFailed'));
    }
  }

  function queueMenuItems(entry) {
    if (entry.builtin) {
      return [
        {
          id: 'remove-builtin',
          type: 'regular',
          icon: EyeOff,
          title: t('supportQueue.removeBuiltin'),
          testid: `support-queue-remove-builtin-${entry.key}`,
          onClick: () => dismissBuiltin(entry),
        },
      ];
    }
    const custom = queues.filter((item) => !item.builtin);
    const index = custom.findIndex((item) => item.id === entry.id);
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Pencil,
        title: t('supportQueue.edit'),
        testid: 'support-queue-edit',
        onClick: () => openEdit(entry),
      },
      {
        id: 'move-left',
        type: 'regular',
        icon: ArrowLeft,
        title: t('supportQueue.moveLeft'),
        disabled: index <= 0,
        testid: 'support-queue-move-left',
        onClick: () => moveQueue(entry, 'left'),
      },
      {
        id: 'move-right',
        type: 'regular',
        icon: ArrowRight,
        title: t('supportQueue.moveRight'),
        disabled: index === custom.length - 1,
        testid: 'support-queue-move-right',
        onClick: () => moveQueue(entry, 'right'),
      },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('supportQueue.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        testid: 'support-queue-delete',
        onClick: () => removeQueue(entry),
      },
    ];
  }

  onMount(() => {
    void loadCollection();
    void loadBoardConfiguration();
    void loadQueues().then(() => loadItems({ page: 1 }));
    void workspaceDataStore.initialize(workspaceId);
    teamsApi
      .getAll()
      .then((list) => {
        teams = list ?? [];
      })
      .catch(() => {
        teams = [];
      });
  });
</script>

<StaticViewBackground
  backgroundStyle={styles.backgroundStyle}
  contextVars={styles.contextVars}
  fullWidth
  rootClass="h-full"
  contentClass="flex h-full flex-col"
  testid="support-queue-view"
>
  <!-- Chrome stays at viewport width; only the table canvas scrolls, so wide
       column sets pan under a toolbar that is always reachable (like the board). -->
  <div class="flex-shrink-0 px-6 pt-3">
    <div class="mb-6">
      <ViewHeader
        viewName={t('supportQueue.title')}
        itemCount={activeQueueCount}
        shownCount={loadingItems ? null : items.length}
      />
    </div>

    {#if !loadingQueues}
      <div class="flex flex-wrap items-center gap-2 mb-3" role="tablist" data-testid="support-queue-tabs">
        {#each queues as entry (queueRef(entry))}
          <div
            class="flex items-center rounded-md border transition-colors"
            style={queueTabStyle(queueRef(entry) === activeRef)}
          >
            <button
              type="button"
              role="tab"
              aria-selected={queueRef(entry) === activeRef}
              class="px-3 py-1.5 text-sm whitespace-nowrap"
              data-testid={`support-queue-tab-${queueRef(entry)}`}
              onclick={() => selectQueue(queueRef(entry))}
            >
              {entry.name}
              <Badge
                size="xs"
                variant="neutral"
                class="ml-1.5"
                style="background-color: var(--ctx-badge-bg, var(--ds-background-neutral)); color: var(--ds-text); backdrop-filter: var(--ctx-backdrop, none);"
                data-testid={`support-queue-count-${queueRef(entry)}`}
              >
                {entry.count}
              </Badge>
            </button>
            {#if canManage && queueRef(entry) === activeRef}
              <DropdownMenu
                triggerText=""
                triggerIcon={MoreHorizontal}
                iconOnly
                triggerClass="p-1.5 rounded action-btn transition-colors"
                triggerTestid={`support-queue-menu-${queueRef(entry)}`}
                items={queueMenuItems(entry)}
              />
            {/if}
          </div>
        {/each}

        {#if canManage}
          <!-- shortcut-guard-exempt: contextual queue-tab action; no global shortcut. -->
          <Button
            variant="ghost"
            size="small"
            icon={Plus}
            dataTestid="support-queue-add"
            onclick={openCreate}
          >
            {t('supportQueue.add')}
          </Button>
        {/if}
      </div>

      <div class="flex items-center justify-between gap-4 mb-4">
        <SearchInput
          bind:value={searchQuery}
          placeholder={t('common.search')}
          dataTestid="support-queue-search-input"
          on_input={handleSearchInput}
        />
        <ColumnSelector
          columns={listColumns}
          {customFieldDefinitions}
          canConfigure={canConfigureColumns}
          testId="queue-column-selector-trigger"
          onchange={handleColumnChange}
        />
      </div>

      {#if searchDebouncing}
        <p class="-mt-2 mb-4 text-sm" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));" data-testid="support-queue-search-status">
          {t('common.searching')}
        </p>
      {/if}

    {#if selectedIds.size > 0}
      <div
        class="flex flex-wrap items-center gap-2 mb-3 px-3 py-2 rounded-md border"
        style="border-color: var(--ctx-border, var(--ds-border)); background-color: var(--ctx-surface, var(--ds-background-neutral)); backdrop-filter: var(--ctx-backdrop, none);"
        data-testid="support-queue-bulk-bar"
      >
        <span class="text-sm" style="color: var(--ds-text);">{t('supportQueue.bulkBar', { n: selectedIds.size })}</span>
        <Button
          variant="ghost"
          size="small"
          style="border-color: var(--ctx-border, var(--ds-border)); color: var(--ds-text);"
          dataTestid="support-queue-bulk-assign-me"
          disabled={bulkRunning}
          onclick={bulkAssignToMe}
        >
          {t('supportQueue.bulkAssignMe')}
        </Button>
        <div class="w-56" data-testid="support-queue-bulk-team-picker">
          <TeamPicker
            bind:value={bulkTeamId}
            teams={teams}
            placeholder={t('supportQueue.bulkAssignTeam')}
            searchTestid="support-queue-bulk-team-search"
            menuTestid="support-queue-bulk-team-menu"
            disabled={bulkRunning}
          />
        </div>
        <Button
          variant="ghost"
          size="small"
          style="border-color: var(--ctx-border, var(--ds-border)); color: var(--ds-text);"
          dataTestid="support-queue-bulk-team-apply"
          disabled={bulkRunning || !bulkTeamId}
          onclick={bulkAssignToTeam}
        >
          {t('supportQueue.bulkApplyTeam')}
        </Button>
        <div class="w-48" data-testid="support-queue-bulk-status-picker">
          <Select
            id="support-queue-bulk-status"
            bind:value={bulkStatusId}
            options={statusOptions}
            placeholder={t('supportQueue.bulkTransition')}
            ariaLabel={t('supportQueue.bulkTransition')}
          />
        </div>
        <Button
          variant="ghost"
          size="small"
          style="border-color: var(--ctx-border, var(--ds-border)); color: var(--ds-text);"
          dataTestid="support-queue-bulk-status-apply"
          disabled={bulkRunning || !bulkStatusId}
          onclick={bulkTransition}
        >
          {t('supportQueue.bulkApplyStatus')}
        </Button>
        <Button
          variant="ghost"
          size="small"
          style="color: var(--ds-text-danger);"
          dataTestid="support-queue-bulk-delete"
          disabled={bulkRunning}
          onclick={bulkDelete}
        >
          {t('supportQueue.bulkDelete')}
        </Button>
        <Button
          variant="ghost"
          size="small"
          style="color: var(--ds-text);"
          dataTestid="support-queue-bulk-clear"
          onclick={clearSelection}
        >
          {t('supportQueue.bulkClear')}
        </Button>
      </div>
      {/if}
    {/if}
  </div>

  <div
    class="min-h-0 min-w-0 max-w-full flex-1 w-full overflow-auto overscroll-contain"
    data-testid="support-queue-canvas"
  >
    <div class="px-6 pb-6 min-w-fit">
      {#if loadingQueues}
        <div class="flex items-center gap-2 py-8" data-testid="support-queue-loading">
          <Spinner class="w-4 h-4" />
          <span style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">{t('supportQueue.loading')}</span>
        </div>
      {:else if loadingItems}
        <div class="flex items-center gap-2 py-8">
          <Spinner class="w-4 h-4" />
        </div>
      {:else if items.length === 0}
        <div data-testid="support-queue-empty">
          <EmptyState title={t('supportQueue.empty')} description={t('supportQueue.emptyDescription')} />
        </div>
      {:else}
        <div class="rounded-xl border shadow-sm overflow-hidden" style="{styles.tableStyle(12)} border-color: var(--ctx-border, var(--ds-border));" data-testid="support-queue-table">
          <TableHeaderBar
            columns={gridTemplateColumns}
            minWidth={gridMinWidth}
            style={styles.tableHeaderStyle}
          >
            <div></div>
            {#each listColumns as column (column.field_identifier)}
              <div data-testid={`support-queue-header-${column.field_identifier}`}>{columnLabel(column)}</div>
            {/each}
            <div>{t('common.actions')}</div>
          </TableHeaderBar>

          <div>
            {#each items as item (item.id)}
              <div
                class="px-4 py-3 list-row transition-colors"
                style="border-top: 1px solid var(--ds-border);"
                data-item-row
                data-item-id={item.id}
                data-testid={`support-queue-row-${item.id}`}
              >
                <LazyRender>
                  {#snippet children()}
                    <div
                      class="grid gap-4 items-center"
                      style="grid-template-columns: {gridTemplateColumns}; min-width: {gridMinWidth};"
                    >
                      <input
                        type="checkbox"
                        checked={selectedIds.has(item.id)}
                        aria-label={t('supportQueue.selectAll')}
                        data-testid={`support-queue-item-checkbox-${item.id}`}
                        onchange={() => toggleItem(item.id)}
                      />
                      {#each listColumns as column (column.field_identifier)}
                        <div class="min-w-0 overflow-hidden">
                          <ListCellRenderer
                            {item}
                            {column}
                            {workspace}
                            canEdit={canEdit && item.workspace_id === Number(workspaceId)}
                            {statuses}
                            {statusCategories}
                            {priorities}
                            {milestones}
                            {iterations}
                            {users}
                            {projects}
                            {itemTypes}
                            {customFieldDefinitions}
                            onitemUpdated={handleItemUpdated}
                            onupdateError={handleUpdateError}
                          />
                        </div>
                      {/each}

                      <!-- Actions -->
                      <div>
                        <DropdownMenu
                          triggerText=""
                          triggerIcon={MoreHorizontal}
                          triggerClass="p-2 rounded action-btn transition-colors"
                          items={buildItemActions(item)}
                        />
                      </div>
                    </div>
                  {/snippet}
                </LazyRender>
              </div>
            {/each}
          </div>
        </div>

        {#if itemsPagination && itemsPagination.total_items > 0 && items.length > 0}
          <div class="mt-6">
            <Pagination
              currentPage={itemsPagination.page}
              totalItems={itemsPagination.total_items}
              itemsPerPage={itemsPagination.limit}
              maxItems={10000}
              onpageChange={handlePageChange}
              onpageSizeChange={handlePageSizeChange}
            />
          </div>
        {/if}
      {/if}
    </div>
  </div>
</StaticViewBackground>

<Modal
  bind:isOpen={editorOpen}
  onclose={closeEditor}
  onSubmit={saveEditor}
  submitDisabled={!editorName.trim() || !editorQl.trim() || editorSaving}
  maxWidth="max-w-2xl"
>
  <div class="p-6" data-testid="support-queue-editor">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      {editorQueueId ? t('supportQueue.edit') : t('supportQueue.add')}
    </h2>
    <label class="block text-sm font-medium mb-1" for="queue-editor-name" style="color: var(--ds-text-subtle);">
      {t('supportQueue.nameLabel')}
    </label>
    <Input
      id="queue-editor-name"
      dataTestid="queue-editor-name"
      bind:value={editorName}
      placeholder={t('supportQueue.namePlaceholder')}
      class="mb-4"
    />
    <span class="block text-sm font-medium mb-1" style="color: var(--ds-text-subtle);">
      {t('supportQueue.filterLabel')}
    </span>
    <QlFilterBuilder
      qlQuery={editorQl}
      filterState={editorFilterState}
      testIdPrefix="queue-editor-builder"
      onchange={handleEditorChange}
    />
    <DialogFooter
      onCancel={closeEditor}
      onConfirm={saveEditor}
      confirmLabel={t('common.save')}
      cancelTestid="queue-editor-cancel"
      confirmTestid="queue-editor-save"
      disabled={!editorName.trim() || !editorQl.trim()}
      loading={editorSaving}
      showKeyboardHint={true}
    />
  </div>
</Modal>

<style>
  .list-row:hover {
    background-color: var(--ctx-background-neutral-hovered, var(--ds-background-neutral-hovered));
  }
</style>
