import { api } from '../api.js';
import { childItemTypesForParent } from '../utils/hierarchy.js';
import { buildDetailScreenFieldConfig, resolveEffectiveScreenIds } from '../utils/screenFields.js';
import { workspaceDataStore } from './workspaceDataStore.svelte.js';

const FIELD_MAP = {
  title: 'title',
  description: 'description',
  status: 'status_id',
  priority: 'priority_id',
  dueDate: 'due_date',
  startDate: 'start_date',
  endDate: 'end_date',
  milestone: 'milestones',
  iteration: 'iteration_id',
  assignee: 'assignee_id',
  project: 'project_id',
};

const STRING_FIELDS = new Set(['title', 'description']);

function isNumericID(value) {
  return /^\d+$/.test(String(value ?? ''));
}

function isAbortError(error) {
  return error?.name === 'AbortError';
}

function hasSharedWorkspaceReferences(workspaceId) {
  return (
    workspaceDataStore.initialized && Number(workspaceDataStore.workspaceId) === Number(workspaceId)
  );
}

function childItemListsMatch(current = [], next = []) {
  if (current === next) return true;
  if (!Array.isArray(current) || !Array.isArray(next) || current.length !== next.length) {
    return false;
  }

  return current.every((item, index) => childItemSummariesMatch(item, next[index]));
}

function childItemSummariesMatch(current = {}, next = {}) {
  const fields = [
    'id',
    'workspace_id',
    'workspace_key',
    'workspace_item_number',
    'item_type_id',
    'title',
    'status_id',
    'status_name',
    'status_color',
    'frac_index',
  ];

  return fields.every((field) => (current?.[field] ?? null) === (next?.[field] ?? null));
}

const RELATED_ITEM_FIELDS = {
  status: ['status_id', 'status_name', 'status_color', 'status_category_id'],
  priority: ['priority_id', 'priority_name', 'priority_color'],
  iteration: ['iteration_id', 'iteration_name', 'iteration_end_date'],
  assignee: ['assignee_id', 'assignee_name', 'assignee_email'],
  project: ['project_id', 'project_name', 'inherit_project'],
};

const DEFAULT_EDITING_STATE = {
  title: { active: false, value: '' },
  description: { active: false, value: '' },
  status: { active: false, value: null },
  priority: { active: false, value: null },
  dueDate: { active: false, value: null },
  startDate: { active: false, value: null },
  endDate: { active: false, value: null },
  milestone: { active: false, value: [] },
  iteration: { active: false, value: null },
  project: { active: false, value: null },
  assignee: { active: false, value: null },
  customFields: { active: {}, values: {} },
};

class ItemDetailStore {
  // Monotonic counter for in-flight loadItem calls; lets us discard results
  // from superseded calls when the user clicks rapidly through items.
  #loadToken = 0;
  #loadController = null;
  #refreshToken = 0;
  #refreshController = null;
  #refreshInFlight = false;
  #refreshPending = false;
  #linksController = null;
  #worklogsController = null;
  #worklogsPromise = null;
  #worklogsPromiseItemId = null;
  #worklogsLoadedItemId = null;
  #diagramsController = null;
  #diagramsPromise = null;
  #diagramsPromiseItemId = null;
  #diagramsLoadedItemId = null;
  #timeModalDataController = null;
  #timeModalDataPromise = null;
  #timeModalDataLoaded = false;

  // === Current Item ===
  item = $state(null);
  itemId = $state(null);
  workspaceId = $state(null);
  loading = $state(true);
  error = $state(null);
  saving = $state(false);
  // The detail view closes when an SSE deletion or refresh 404 sets this.
  notFound = $state(false);

  // Workspace
  workspace = $state(null);

  // === Editing State (unified flag + value) ===
  editing = $state({ ...DEFAULT_EDITING_STATE });

  // === Related Data (cached) ===
  parentHierarchy = $state([]);
  childItems = $state([]);
  loadingChildItems = $state(false);
  milestones = $state([]);
  iterations = $state([]);
  priorities = $state([]);

  // Item types
  itemTypes = $state([]);
  currentItemType = $state(null);
  currentHierarchyLevel = $state(null);
  availableSubIssueTypes = $state([]);

  // Null editable fields preserve the legacy path: all visible fields are editable.
  customFieldDefinitions = $state([]);
  workspaceScreenFields = $state([]);
  workspaceScreenSystemFields = $state([]);
  // Virtual field metadata for the item's request type (read-only display only).
  requestTypeFields = $state([]);
  editableScreenFieldIds = $state(null);
  editableScreenSystemFields = $state(null);

  // Status
  availableStatusTransitions = $state([]);
  loadingStatusTransitions = $state(false);
  pendingApproval = $state(null);

  // Links
  itemLinks = $state([]);
  linkTypes = $state([]);
  loadingLinks = $state(false);

  // Watch
  isWatching = $state(false);
  loadingWatchStatus = $state(false);

  // Time tracking
  timeProjects = $state([]);
  timeWorklogs = $state([]);
  timeWorklogsLoading = $state(false);
  timeModalDataLoading = $state(false);
  customers = $state([]);
  workItems = $state([]);
  workspaces = $state([]);
  // Cache the optional child-item rollup for the current item.
  includeChildItems = $state(false);
  timeRollup = $state(null);
  timeRollupLoading = $state(false);

  // Diagrams & Actions
  diagrams = $state([]);
  loadingDiagrams = $state(false);
  diagramsLoaded = $state(false);
  manualActions = $state([]);

  // Modals
  showDeleteDialog = $state(false);
  showLinkModal = $state(false);
  // Callers may preselect the link type before opening this modal.
  linkModalPreselectTypeId = $state(null);
  showTestCaseModal = $state(false);
  selectedTestCaseId = $state(null);
  showTimeLogModal = $state(false);
  editingWorklog = $state(null);

  // Track changes
  hasChanges = $state(false);

  // Animation state
  transitioning = $state(false);

  // Dropdown items (computed from item state)
  dropdownItems = $state([]);

  // === Derived Values (getters) ===

  get statusOptions() {
    if (this.availableStatusTransitions.length > 0) {
      return this.availableStatusTransitions.map((transition) => ({
        id: transition.id,
        value: transition.value,
        label: transition.name,
        builtin_key: transition.builtin_key,
        categoryColor: transition.category_color || null,
      }));
    }
    return this.loadingStatusTransitions ? [{ value: '', label: 'Loading...' }] : [];
  }

  get filteredLinkTypes() {
    return this.linkTypes;
  }

  // === Data Loading Methods ===

  /**
   * Load all item data and related data.
   *
   * Stale-while-revalidate: when an item is already displayed (a switch),
   * existing state stays in place and is overwritten only as new data arrives,
   * so the UI doesn't flash a skeleton between items. Rapid clicks are made
   * race-safe by a monotonic load token; superseded results are discarded.
   */
  async loadItem(workspaceId, itemId, options = {}) {
    this.#loadController?.abort();
    this.#refreshController?.abort();
    this.#linksController?.abort();
    this.#worklogsController?.abort();
    this.#diagramsController?.abort();
    const controller = new AbortController();
    this.#loadController = controller;
    this.#refreshController = null;
    this.#refreshToken += 1;
    this.#refreshInFlight = false;
    this.#refreshPending = false;
    const requestOptions = { signal: controller.signal };
    const token = ++this.#loadToken;
    const isSwitch = this.item != null;

    let effectiveWorkspaceId = workspaceId;
    let effectiveItemId = itemId;
    const lookupWorkspaceKey =
      options.workspaceKey || (workspaceId && !isNumericID(workspaceId) ? workspaceId : null);
    const lookupItemNumber = options.itemNumber || (lookupWorkspaceKey ? itemId : null);

    this.itemId = effectiveItemId;
    this.timeWorklogs = [];
    this.timeWorklogsLoading = false;
    this.#worklogsLoadedItemId = null;
    this.#worklogsPromise = null;
    this.#worklogsPromiseItemId = null;
    this.diagrams = [];
    this.loadingDiagrams = false;
    this.diagramsLoaded = false;
    this.#diagramsPromise = null;
    this.#diagramsPromiseItemId = null;
    this.#diagramsLoadedItemId = null;
    this.error = null;
    this.notFound = false;
    if (!isSwitch) {
      this.loading = true;
      this.loadingLinks = true;
    }
    this.loadingStatusTransitions = true;
    this.loadingWatchStatus = true;
    this.loadingChildItems = true;

    try {
      let workspaceInitPromise = isNumericID(effectiveWorkspaceId)
        ? workspaceDataStore.initialize(effectiveWorkspaceId)
        : null;
      const summary = lookupWorkspaceKey
        ? await api.items.getDetailSummaryByKey(
            lookupWorkspaceKey,
            lookupItemNumber,
            requestOptions
          )
        : await api.items.getDetailSummary(effectiveItemId, requestOptions);
      if (token !== this.#loadToken) return;

      const itemData = summary?.item;
      if (!itemData) throw new Error('Item detail summary did not include an item');
      effectiveItemId = itemData.id;
      effectiveWorkspaceId = itemData.workspace_id;
      this.itemId = effectiveItemId;
      this.item = itemData;
      if (this.item.assignee_id === undefined) {
        this.item.assignee_id = null;
      }

      // Use provided/resolved workspaceId or derive from item
      const wsId = effectiveWorkspaceId || itemData.workspace_id;
      this.workspaceId = wsId;

      // Share MainApp's in-flight workspace bootstrap.
      workspaceInitPromise ??= workspaceDataStore.initialize(wsId);
      await workspaceInitPromise;
      if (token !== this.#loadToken) return;
      const useSharedReferences = hasSharedWorkspaceReferences(wsId);

      this.workspace = useSharedReferences
        ? workspaceDataStore.workspace
        : await api.workspaces.get(wsId, requestOptions);
      this.customFieldDefinitions = useSharedReferences
        ? workspaceDataStore.customFieldDefinitions
        : [];

      // Filter milestones by workspace restrictions
      const allMilestones = useSharedReferences ? workspaceDataStore.milestones : [];
      if (this.workspace?.milestone_categories?.length > 0) {
        const allowedCategoryIds = this.workspace.milestone_categories;
        this.milestones = allMilestones.filter((m) => allowedCategoryIds.includes(m.category_id));
      } else {
        this.milestones = allMilestones;
      }

      this.iterations = useSharedReferences ? workspaceDataStore.iterations : [];
      this.timeProjects = useSharedReferences ? workspaceDataStore.projects : [];
      this.itemTypes = useSharedReferences ? workspaceDataStore.itemTypes : [];
      this.priorities = [
        ...(summary.priorities?.length
          ? summary.priorities
          : useSharedReferences
            ? workspaceDataStore.priorities
            : []),
      ].sort((a, b) => a.sort_order - b.sort_order);

      this.requestTypeFields = summary.request_type_fields || [];
      this.linkTypes = summary.link_types || [];
      this.#applyLinks(summary.links);
      this.availableStatusTransitions = summary.transitions?.available_transitions || [];
      this.pendingApproval = summary.transitions?.pending_approval || null;
      this.isWatching = summary.watching || false;
      this.childItems = summary.children || [];
      this.currentItemType = summary.current_item_type || null;
      this.currentHierarchyLevel = summary.current_hierarchy_level || null;
      this.availableSubIssueTypes = summary.available_sub_issue_types || [];
      this.manualActions = summary.manual_actions || [];

      const fieldConfig = buildDetailScreenFieldConfig(
        summary.screen_context?.edit,
        summary.screen_context?.view
      );
      this.workspaceScreenFields = fieldConfig.visibleCustomFields;
      this.workspaceScreenSystemFields = fieldConfig.visibleSystemFields;
      this.editableScreenFieldIds = fieldConfig.editableCustomFieldIds;
      this.editableScreenSystemFields = fieldConfig.editableSystemFields;

      this.parentHierarchy = (summary.ancestors || []).map((ancestor) => {
        const itemType = this.itemTypes.find((type) => type.id === ancestor.item_type_id);
        return itemType ? { ...ancestor, itemType } : ancestor;
      });

      // Heavy optional panels such as diagrams, worklogs, history, SCM detail,
      // and agent logs remain deferred behind their existing loaders.
      this.#syncEditingFromItem();
      return this.item;
    } catch (err) {
      if (token !== this.#loadToken || isAbortError(err)) return;
      console.error('Failed to load item or workspace:', err);
      this.error = err.message || 'Failed to load data';
      this.item = null;
    } finally {
      if (token === this.#loadToken) {
        if (this.#loadController === controller) this.#loadController = null;
        this.loading = false;
        this.loadingLinks = false;
        this.loadingStatusTransitions = false;
        this.loadingWatchStatus = false;
        this.loadingChildItems = false;
        this.transitioning = false;
        this.#runPendingRefresh();
      }
    }
  }

  // Refresh the item without overwriting fields being edited locally.
  async refreshCurrentItem() {
    if (!this.itemId) return;
    if (this.loading || this.saving || this.#refreshInFlight) {
      this.#refreshPending = true;
      return;
    }

    const itemId = this.itemId;
    const controller = new AbortController();
    const token = ++this.#refreshToken;
    this.#refreshController = controller;
    this.#refreshInFlight = true;

    try {
      // An SSE refresh must not share a pre-mutation GET that happens to still
      // be in flight. Otherwise the event can be consumed while the UI is
      // reconciled with the response captured before that mutation.
      const nextItem = await api.items.get(itemId, {
        cache: 'no-store',
        signal: controller.signal,
      });
      if (token !== this.#refreshToken || String(itemId) !== String(this.itemId)) return;
      if (!nextItem || String(nextItem.id) !== String(itemId)) return;

      const previousStatusID = this.item?.status_id;
      const previousItemTypeID = this.item?.item_type_id;
      const previousParentID = this.item?.parent_id;

      this.item = this.#mergeItemPreservingActiveEdits(this.item, nextItem);
      this.#syncInactiveEditingFromItem();

      if (previousStatusID !== this.item.status_id) {
        await this.#loadAvailableStatusTransitions();
      }
      if (previousItemTypeID !== this.item.item_type_id) {
        await this.#loadItemTypeData();
        await this.#loadWorkspaceScreenFields();
      }
      if (previousParentID !== this.item.parent_id) {
        if (this.item.parent_id) {
          await this.#loadParentHierarchy();
        } else {
          this.parentHierarchy = [];
        }
      }
    } catch (err) {
      if (token !== this.#refreshToken || isAbortError(err)) return;
      // A deleted item must close rather than remain stale.
      if (err?.status === 404) {
        this.markDeleted();
        return;
      }
      console.warn('Failed to refresh item detail:', err);
    } finally {
      if (token === this.#refreshToken) {
        if (this.#refreshController === controller) this.#refreshController = null;
        this.#refreshInFlight = false;
        this.#runPendingRefresh();
      }
    }
  }

  #runPendingRefresh() {
    if (!this.#refreshPending || this.loading || this.saving || this.#refreshInFlight) return;
    this.#refreshPending = false;
    queueMicrotask(() => this.refreshCurrentItem());
  }

  markDeleted() {
    // Tear down the rendered detail immediately so lazy panels abort their own
    // requests before the deleted resource starts returning 404s.
    this.#loadToken += 1;
    this.#refreshToken += 1;
    this.#loadController?.abort();
    this.#refreshController?.abort();
    this.#linksController?.abort();
    this.#worklogsController?.abort();
    this.#diagramsController?.abort();
    this.#timeModalDataController?.abort();
    this.item = null;
    this.#refreshController = null;
    this.#refreshInFlight = false;
    this.#refreshPending = false;
    this.notFound = true;
  }

  // Single-flight per-item worklog loading prevents duplicate tab requests.
  async loadWorklogs({ force = false } = {}) {
    if (!this.itemId) return;
    const itemId = this.itemId;
    if (!force && this.#worklogsLoadedItemId === itemId) return this.timeWorklogs;
    if (!force && this.#worklogsPromise && this.#worklogsPromiseItemId === itemId) {
      return this.#worklogsPromise;
    }

    this.#worklogsController?.abort();
    const controller = new AbortController();
    this.#worklogsController = controller;
    this.#worklogsPromiseItemId = itemId;
    this.timeWorklogsLoading = true;

    const promise = api.time.worklogs
      .getByItem(itemId, { signal: controller.signal })
      .then((worklogs) => {
        if (this.itemId === itemId) {
          this.timeWorklogs = worklogs || [];
          this.#worklogsLoadedItemId = itemId;
        }
        return this.timeWorklogs;
      })
      .catch((err) => {
        if (isAbortError(err)) return this.timeWorklogs;
        console.error('Failed to load worklogs:', err);
        return this.timeWorklogs;
      })
      .finally(() => {
        if (this.#worklogsController === controller) {
          this.#worklogsController = null;
          this.#worklogsPromise = null;
          this.#worklogsPromiseItemId = null;
          this.timeWorklogsLoading = false;
        }
      });
    this.#worklogsPromise = promise;
    return promise;
  }

  // Load TimeLogModal-only picker data lazily and once.
  async loadTimeModalData() {
    if (this.#timeModalDataLoaded) return;
    if (this.#timeModalDataPromise) return this.#timeModalDataPromise;

    const controller = new AbortController();
    this.#timeModalDataController = controller;
    this.timeModalDataLoading = true;
    const requestOptions = { signal: controller.signal };
    const fallback = (promise, label) =>
      promise.catch((err) => {
        if (isAbortError(err)) throw err;
        console.warn(`Failed to load ${label} for time logging:`, err);
        return [];
      });

    const promise = Promise.all([
      fallback(api.customerOrganisations.getAll({}, requestOptions), 'customers'),
      fallback(api.items.getAll({ limit: 100 }, requestOptions), 'work items'),
      fallback(api.workspaces.getAll({}, requestOptions), 'workspaces'),
    ])
      .then(([customers, workItems, workspaces]) => {
        this.customers = customers || [];
        this.workItems = workItems?.items || workItems || [];
        this.workspaces = workspaces || [];
        this.#timeModalDataLoaded = true;
      })
      .catch((err) => {
        if (!isAbortError(err)) console.error('Failed to load time-log modal data:', err);
      })
      .finally(() => {
        if (this.#timeModalDataController === controller) {
          this.#timeModalDataController = null;
          this.#timeModalDataPromise = null;
          this.timeModalDataLoading = false;
        }
      });
    this.#timeModalDataPromise = promise;
    return promise;
  }

  /** Reload worklogs after a timer or manual time-entry mutation. */
  async reloadWorklogs() {
    await this.loadWorklogs({ force: true });
    if (this.includeChildItems) this.loadTimeRollup({ force: true });
  }

  // Fetch and cache the current item's descendant time rollup.
  async loadTimeRollup({ force = false } = {}) {
    if (!this.itemId) return;
    if (this.timeRollup && !force) return;
    this.timeRollupLoading = true;
    try {
      this.timeRollup = await api.items.getTimeRollup(this.itemId);
    } catch (err) {
      console.error('Failed to load time rollup:', err);
      this.timeRollup = null;
    } finally {
      this.timeRollupLoading = false;
    }
  }

  async loadChildItems(requestOptions = {}) {
    if (!this.itemId) return;
    try {
      this.loadingChildItems = true;
      const response = await api.items.getChildren(this.itemId, requestOptions);
      let nextChildItems = [];
      if (Array.isArray(response)) {
        nextChildItems = response;
      } else if (response?.items) {
        nextChildItems = response.items;
      } else if (response?.data) {
        nextChildItems = response.data;
      }

      if (!childItemListsMatch(this.childItems, nextChildItems)) {
        this.childItems = nextChildItems;
      }
    } catch (err) {
      if (isAbortError(err)) return;
      console.error('Failed to load child items:', err);
      this.childItems = [];
    } finally {
      if (!requestOptions.signal?.aborted) this.loadingChildItems = false;
    }
  }

  // Link mutations and SSE link events refresh only links, not the full view.
  async loadLinks() {
    if (!this.itemId) return;
    this.#linksController?.abort();
    const controller = new AbortController();
    this.#linksController = controller;
    const itemId = this.itemId;
    try {
      this.loadingLinks = true;
      const links = await api.links.getForItem('items', itemId, {
        signal: controller.signal,
      });
      if (this.itemId !== itemId) return;
      this.#applyLinks(links);
    } catch (err) {
      if (isAbortError(err)) return;
      console.error('Failed to load item links:', err);
    } finally {
      if (this.#linksController === controller) {
        this.#linksController = null;
        this.loadingLinks = false;
      }
    }
  }

  async loadDiagrams({ force = false } = {}) {
    const itemId = this.item?.id;
    if (!itemId) return [];
    if (!force && this.#diagramsLoadedItemId === itemId) return this.diagrams;
    if (!force && this.#diagramsPromise && this.#diagramsPromiseItemId === itemId) {
      return this.#diagramsPromise;
    }

    this.#diagramsController?.abort();
    const controller = new AbortController();
    this.#diagramsController = controller;
    this.#diagramsPromiseItemId = itemId;
    this.loadingDiagrams = true;

    const promise = api
      .getDiagrams(itemId, { signal: controller.signal })
      .then((diagrams) => {
        if (this.item?.id === itemId) {
          this.diagrams = diagrams || [];
          this.diagramsLoaded = true;
          this.#diagramsLoadedItemId = itemId;
        }
        return this.diagrams;
      })
      .catch((err) => {
        if (isAbortError(err)) return this.diagrams;
        if (err?.status === 404) {
          this.markDeleted();
          return this.diagrams;
        }
        console.error('Failed to load diagrams:', err);
        if (this.item?.id === itemId) this.diagrams = [];
        return this.diagrams;
      })
      .finally(() => {
        if (this.#diagramsController === controller) {
          this.#diagramsController = null;
          this.#diagramsPromise = null;
          this.#diagramsPromiseItemId = null;
          this.loadingDiagrams = false;
        }
      });

    this.#diagramsPromise = promise;
    return promise;
  }

  // === Private Data Loading Methods ===

  #applyLinks(linksData) {
    const links = [];
    if (linksData?.outgoing) links.push(...linksData.outgoing);
    if (linksData?.incoming) links.push(...linksData.incoming);
    this.itemLinks = links;
  }

  async #loadAvailableStatusTransitions(requestOptions = {}) {
    if (!this.item?.id) return;
    const itemId = this.item.id;
    try {
      this.loadingStatusTransitions = true;
      const result = await api.items.getAvailableStatusTransitions(itemId, requestOptions);
      if (this.item?.id !== itemId) return;
      this.availableStatusTransitions = result.available_transitions || [];
      this.pendingApproval = result.pending_approval || null;
    } catch (err) {
      if (isAbortError(err)) return;
      console.error('Failed to load status transitions:', err);
      this.availableStatusTransitions = [];
      this.pendingApproval = null;
    } finally {
      if (!requestOptions.signal?.aborted && this.item?.id === itemId) {
        this.loadingStatusTransitions = false;
      }
    }
  }

  async refreshAvailableTransitions() {
    await this.#loadAvailableStatusTransitions();
  }

  async #loadParentHierarchy(requestOptions = {}) {
    try {
      const ancestors = await api.items.getAncestors(this.item.id, requestOptions);
      try {
        // Reuse the already-loaded type list (set by #loadItemTypeData, which
        // runs first); only fetch as a fallback if it isn't populated yet.
        const itemTypesData = this.itemTypes?.length
          ? this.itemTypes
          : await api.itemTypes.getAll({}, requestOptions);
        this.parentHierarchy = ancestors.map((ancestor) => {
          if (ancestor.item_type_id) {
            const itemType = itemTypesData.find((type) => type.id === ancestor.item_type_id);
            return { ...ancestor, itemType };
          }
          return ancestor;
        });
      } catch (err) {
        if (isAbortError(err)) throw err;
        console.warn('Failed to load item types for parent hierarchy:', err);
        this.parentHierarchy = ancestors;
      }
    } catch (err) {
      if (isAbortError(err)) return;
      console.error('Failed to load ancestors:', err);
      this.parentHierarchy = [];
    }
  }

  async #loadItemTypeData(requestOptions = {}) {
    try {
      const useSharedReferences = hasSharedWorkspaceReferences(this.workspaceId);
      const [itemTypesData, hierarchyLevels] = await Promise.all([
        useSharedReferences
          ? Promise.resolve(workspaceDataStore.itemTypes)
          : api.itemTypes.getAll({}, requestOptions),
        api.hierarchyLevels.getAll({}, requestOptions),
      ]);

      this.itemTypes = itemTypesData || [];

      if (this.item.item_type_id) {
        this.currentItemType = this.itemTypes.find((type) => type.id === this.item.item_type_id);
        if (this.currentItemType) {
          this.currentHierarchyLevel = hierarchyLevels.find(
            (level) => level.level === this.currentItemType.hierarchy_level
          );
        }
      }

      // Find available child types, including the level-independent generic
      // sub-task sentinel beneath every regular hierarchy item.
      if (this.currentItemType) {
        this.availableSubIssueTypes = childItemTypesForParent(this.itemTypes, this.currentItemType);
      } else {
        this.availableSubIssueTypes = [];
      }
    } catch (err) {
      if (isAbortError(err)) return;
      console.error('Failed to load item type data:', err);
      this.currentItemType = null;
      this.currentHierarchyLevel = null;
      this.availableSubIssueTypes = [];
    }
  }

  /**
   * @param {object|null} [configSet] Pre-resolved configuration set shared by
   *   loadItem. Pass `undefined` to let this method fetch it itself (the
   *   refresh path); `null` means "none configured / fetch failed".
   */
  async #loadWorkspaceScreenFields(configSet = undefined, requestOptions = {}) {
    try {
      let editScreenId = null;
      let viewScreenId = null;

      let cs = configSet;
      if (cs === undefined) {
        cs = this.workspace?.configuration_set_id
          ? await api.configurationSets.get(this.workspace.configuration_set_id, requestOptions)
          : null;
      }
      if (cs) {
        const itemTypeId = this.item?.item_type_id;
        const screenIds = resolveEffectiveScreenIds(cs, itemTypeId, 1);
        editScreenId = screenIds.edit;
        viewScreenId = screenIds.view;
      }

      // Hardcoded fallback (preserves legacy behavior when nothing is
      // configured). resolveEffectiveScreenIds already chains through create as
      // the universal fallback, so a null here means truly nothing is set.
      if (!editScreenId) editScreenId = 1;

      // If view screen is missing or matches edit, only fetch one — same
      // behavior as before (every visible field is editable).
      const sameScreen = !viewScreenId || viewScreenId === editScreenId;
      const [editScreen, viewScreen] = await Promise.all([
        api.screens.get(editScreenId, requestOptions),
        sameScreen ? Promise.resolve(null) : api.screens.get(viewScreenId, requestOptions),
      ]);

      const fieldConfig = buildDetailScreenFieldConfig(editScreen, sameScreen ? null : viewScreen);
      this.workspaceScreenFields = fieldConfig.visibleCustomFields;
      this.workspaceScreenSystemFields = fieldConfig.visibleSystemFields;
      this.editableScreenFieldIds = fieldConfig.editableCustomFieldIds;
      this.editableScreenSystemFields = fieldConfig.editableSystemFields;
    } catch (err) {
      if (isAbortError(err)) return;
      console.error('Failed to load workspace screen fields:', err);
      this.workspaceScreenFields = [];
      this.workspaceScreenSystemFields = [];
      this.editableScreenFieldIds = null;
      this.editableScreenSystemFields = null;
    }
  }

  // === Editing Methods ===

  startEditing(field) {
    if (field.startsWith('custom_field_')) {
      const fieldId = field.replace('custom_field_', '');
      this.editing.customFields.active[fieldId] = true;
      const currentValue = this.item.custom_field_values?.[fieldId];
      this.editing.customFields.values[fieldId] =
        currentValue !== null && currentValue !== undefined ? currentValue : '';
      // Trigger reactivity
      this.editing = { ...this.editing };
    } else {
      // Sync value from item before activating edit mode
      this.#syncFieldFromItem(field);
      this.editing[field].active = true;
      // Trigger reactivity
      this.editing = { ...this.editing };
    }
  }

  cancelEditing(field) {
    if (field.startsWith('custom_field_')) {
      const fieldId = field.replace('custom_field_', '');
      delete this.editing.customFields.active[fieldId];
      delete this.editing.customFields.values[fieldId];
      this.editing = { ...this.editing };
    } else if (this.editing[field]) {
      this.editing[field].active = false;
      this.#syncFieldFromItem(field);
      // Trigger reactivity
      this.editing = { ...this.editing };
    }
  }

  async saveField(field, directValue = null, assigneeName = null, iterationName = null) {
    if (this.saving) return;

    try {
      this.saving = true;
      let updateData = {};

      if (field === 'title') {
        const newTitle = directValue || this.editing.title.value.trim();
        if (newTitle === this.item.title) {
          this.cancelEditing('title');
          return;
        }
        updateData.title = newTitle;
      } else if (field === 'description') {
        const newDescription = directValue !== null ? directValue : this.editing.description.value;
        if (newDescription === (this.item.description || '')) {
          this.cancelEditing('description');
          return;
        }
        updateData.description = newDescription;
      } else if (field === 'status_id') {
        const newStatusId = directValue !== null ? directValue : null;
        if (newStatusId === this.item.status_id) {
          this.cancelEditing('status');
          return;
        }
        // Status changes must use the transition endpoint for workflow validation.
        const updatedItem = await api.items.transition(this.item.id, newStatusId);
        this.item = { ...this.item, ...updatedItem };
        this.hasChanges = true;
        this.cancelEditing('status');
        await this.refreshAvailableTransitions();
        return;
      } else if (field === 'priority_id') {
        const newPriorityId = directValue !== null ? directValue : null;
        if (newPriorityId === this.item.priority_id) {
          this.cancelEditing('priority');
          return;
        }
        updateData.priority_id = newPriorityId;
        this.item = { ...this.item, priority_id: newPriorityId };
      } else if (field === 'due_date') {
        const newDueDate = directValue !== null ? directValue : null;
        if (newDueDate === this.item.due_date) {
          this.cancelEditing('dueDate');
          return;
        }
        updateData.due_date = newDueDate;
        this.item = { ...this.item, due_date: newDueDate };
      } else if (field === 'start_date') {
        const newStartDate = directValue !== null ? directValue : null;
        if (newStartDate === this.item.start_date) {
          this.cancelEditing('startDate');
          return;
        }
        updateData.start_date = newStartDate;
        this.item = { ...this.item, start_date: newStartDate };
      } else if (field === 'end_date') {
        const newEndDate = directValue !== null ? directValue : null;
        if (newEndDate === this.item.end_date) {
          this.cancelEditing('endDate');
          return;
        }
        updateData.end_date = newEndDate;
        this.item = { ...this.item, end_date: newEndDate };
      } else if (field === 'milestone') {
        // value is now an array of milestone IDs (multi-milestone). Treat
        // missing/non-array as empty set.
        const newIds = Array.isArray(directValue) ? [...directValue].sort((a, b) => a - b) : [];
        const currentIds = (this.item.milestones || []).map((m) => m.id).sort((a, b) => a - b);
        const sameSet =
          newIds.length === currentIds.length && newIds.every((id, i) => id === currentIds[i]);
        if (sameSet) {
          this.cancelEditing('milestone');
          return;
        }
        updateData.milestone_ids = newIds;
        // Optimistic local update: rebuild milestones array from the picker's
        // current cache so the UI reflects the new selection immediately.
        const nextMilestones = newIds
          .map((id) => this.milestones.find((m) => m.id === id))
          .filter(Boolean);
        this.item = { ...this.item, milestones: nextMilestones };
      } else if (field === 'story_points') {
        const newPoints = directValue !== undefined ? directValue : null;
        if (newPoints === (this.item.story_points ?? null)) {
          return;
        }
        updateData.story_points = newPoints;
        this.item = { ...this.item, story_points: newPoints };
      } else if (field === 'estimate_minutes' || field === 'estimate') {
        const newEstimate = directValue !== undefined ? directValue : null;
        if (newEstimate === (this.item.estimate_minutes ?? null)) {
          return;
        }
        updateData.estimate_minutes = newEstimate;
        this.item = { ...this.item, estimate_minutes: newEstimate };
      } else if (field === 'iteration') {
        const newIteration = directValue !== null ? directValue : null;
        if (newIteration === this.item.iteration_id) {
          return;
        }
        updateData.iteration_id = newIteration;
        this.item = {
          ...this.item,
          iteration_id: newIteration,
          iteration_name: iterationName !== undefined ? iterationName : this.item.iteration_name,
        };
      } else if (field === 'project') {
        const newProject = directValue !== null ? directValue : this.editing.project.value;
        if (typeof newProject === 'object' && newProject !== null) {
          updateData.project_id = newProject.project_id;
          updateData.inherit_project = newProject.inherit_project;
          this.item = {
            ...this.item,
            project_id: newProject.project_id,
            inherit_project: newProject.inherit_project,
          };
        } else {
          if (newProject === this.item.project_id) {
            this.cancelEditing('project');
            return;
          }
          updateData.project_id = newProject;
          this.item = { ...this.item, project_id: newProject };
        }
      } else if (field === 'assignee') {
        const newAssignee = directValue !== undefined ? directValue : this.editing.assignee.value;
        if (newAssignee === this.item.assignee_id) {
          this.cancelEditing('assignee');
          return;
        }
        updateData.assignee_id = newAssignee;
        this.item = {
          ...this.item,
          assignee_id: newAssignee,
          assignee_name: assigneeName !== undefined ? assigneeName : this.item.assignee_name,
        };
      } else if (field.startsWith('custom_field_')) {
        const fieldId = field.replace('custom_field_', '');
        let newValue =
          directValue !== null ? directValue : this.editing.customFields.values[fieldId];
        const currentValue = this.item.custom_field_values?.[fieldId] || '';

        // Convert number fields
        const fieldDef = this.customFieldDefinitions.find((f) => f.id === parseInt(fieldId, 10));
        if (
          fieldDef?.field_type === 'number' &&
          newValue !== null &&
          newValue !== undefined &&
          newValue !== ''
        ) {
          newValue = parseFloat(newValue);
          if (Number.isNaN(newValue)) {
            newValue =
              directValue !== null ? directValue : this.editing.customFields.values[fieldId];
          }
        }

        if (newValue === currentValue) {
          this.cancelEditing(field);
          return;
        }

        updateData.custom_field_values = {
          ...(this.item.custom_field_values || {}),
          [fieldId]: newValue,
        };
      }

      // Update via API
      const updatedItem = await api.items.update(this.item.id, updateData);
      this.item = { ...this.item, ...updatedItem };

      // Update assignee/iteration names if provided
      if (field === 'assignee' && assigneeName !== null) {
        this.item = { ...this.item, assignee_name: assigneeName };
      }
      if (field === 'iteration' && iterationName !== undefined) {
        this.item = { ...this.item, iteration_name: iterationName };
      }

      this.hasChanges = true;
      this.cancelEditing(field);
    } catch (err) {
      console.error('Failed to update item:', err);
      throw err;
    } finally {
      this.saving = false;
      this.#runPendingRefresh();
    }
  }

  // === Auto-sync editing values when item loads/changes ===

  #syncEditingFromItem() {
    if (!this.item) return;
    for (const [editKey, itemKey] of Object.entries(FIELD_MAP)) {
      if (editKey === 'milestone') {
        // milestones is an array of objects on the item; the editing value
        // tracks the array of IDs the picker binds to.
        this.editing[editKey].value = (this.item.milestones || []).map((m) => m.id);
        continue;
      }
      this.editing[editKey].value = STRING_FIELDS.has(editKey)
        ? this.item[itemKey] || ''
        : this.item[itemKey];
    }
    this.editing.customFields.values = { ...(this.item.custom_field_values || {}) };
  }

  #syncFieldFromItem(field) {
    if (!this.item) return;
    if (FIELD_MAP[field] && this.editing[field]) {
      if (field === 'milestone') {
        this.editing[field].value = (this.item.milestones || []).map((m) => m.id);
        return;
      }
      this.editing[field].value = this.item[FIELD_MAP[field]];
    }
  }

  #syncInactiveEditingFromItem() {
    if (!this.item) return;
    for (const [editKey, itemKey] of Object.entries(FIELD_MAP)) {
      if (this.editing[editKey]?.active) continue;
      if (editKey === 'milestone') {
        this.editing[editKey].value = (this.item.milestones || []).map((m) => m.id);
        continue;
      }
      this.editing[editKey].value = STRING_FIELDS.has(editKey)
        ? this.item[itemKey] || ''
        : this.item[itemKey];
    }

    const nextCustomValues = { ...(this.item.custom_field_values || {}) };
    for (const fieldId of Object.keys(this.editing.customFields.active || {})) {
      if (this.editing.customFields.active[fieldId]) {
        nextCustomValues[fieldId] = this.editing.customFields.values[fieldId];
      }
    }
    this.editing.customFields.values = nextCustomValues;
    this.editing = { ...this.editing };
  }

  #mergeItemPreservingActiveEdits(current, next) {
    if (!current) return next;
    const merged = { ...current, ...next };

    for (const [editKey, itemKey] of Object.entries(FIELD_MAP)) {
      if (!this.editing[editKey]?.active) continue;

      if (editKey === 'milestone') {
        merged.milestones = current.milestones;
        continue;
      }

      merged[itemKey] = current[itemKey];
      for (const relatedKey of RELATED_ITEM_FIELDS[editKey] || []) {
        if (relatedKey in current) merged[relatedKey] = current[relatedKey];
      }
    }

    const activeCustomFields = Object.keys(this.editing.customFields.active || {}).filter(
      (fieldId) => this.editing.customFields.active[fieldId]
    );
    if (activeCustomFields.length > 0) {
      merged.custom_field_values = { ...(next.custom_field_values || {}) };
      for (const fieldId of activeCustomFields) {
        if (current.custom_field_values && fieldId in current.custom_field_values) {
          merged.custom_field_values[fieldId] = current.custom_field_values[fieldId];
        }
      }
    }

    return merged;
  }

  // === Watch Actions ===

  async toggleWatch() {
    if (!this.item?.id) return;
    try {
      if (this.isWatching) {
        await api.items.removeWatch(this.item.id);
        this.isWatching = false;
      } else {
        await api.items.addWatch(this.item.id);
        this.isWatching = true;
      }
      this.hasChanges = true;
    } catch (err) {
      console.error('Failed to toggle watch:', err);
      throw err;
    }
  }

  // === Link Actions ===

  async createLink(linkTypeId, targetId, targetType = 'item') {
    try {
      await api.links.create({
        source_type: 'item',
        source_id: parseInt(this.itemId, 10),
        target_type: targetType,
        target_id: parseInt(targetId, 10),
        link_type_id: parseInt(linkTypeId, 10),
      });
      await this.loadLinks();
    } catch (err) {
      console.error('Error creating link:', err);
      throw err;
    }
  }

  async removeLink(linkId) {
    try {
      await api.links.delete(linkId);
      await this.loadLinks();
    } catch (err) {
      console.error('Error removing link:', err);
      throw err;
    }
  }

  // === Copy Item ===

  async copyItem() {
    try {
      const copiedItem = await api.items.copy(this.item.id);
      return copiedItem;
    } catch (err) {
      console.error('Failed to copy item:', err);
      throw err;
    }
  }

  // === Execute Action ===

  async executeAction(actionId) {
    try {
      await api.actions.execute(this.workspaceId, actionId, this.item.id);
    } catch (err) {
      console.error('Failed to execute action:', err);
      throw err;
    }
  }

  // === Modal Controls ===

  openDeleteDialog() {
    this.showDeleteDialog = true;
  }

  closeDeleteDialog() {
    this.showDeleteDialog = false;
  }

  openLinkModal(preselectLinkTypeId = null) {
    this.linkModalPreselectTypeId = preselectLinkTypeId;
    this.showLinkModal = true;
  }

  closeLinkModal() {
    this.showLinkModal = false;
    this.linkModalPreselectTypeId = null;
  }

  openTestCaseModal(testCaseId) {
    this.selectedTestCaseId = testCaseId;
    this.showTestCaseModal = true;
  }

  closeTestCaseModal() {
    this.showTestCaseModal = false;
    this.selectedTestCaseId = null;
  }

  openTimeLogModal(worklog = null) {
    this.editingWorklog = worklog;
    this.showTimeLogModal = true;
  }

  closeTimeLogModal() {
    this.showTimeLogModal = false;
    this.editingWorklog = null;
  }

  // === Get Default Project for Time Logging ===

  getDefaultProjectForTimeLogging() {
    if (this.item?.time_project_id) return this.item.time_project_id;
    if (this.item?.effective_project_id) return this.item.effective_project_id;
    if (this.workspace?.time_project_id) return this.workspace.time_project_id;
    return null;
  }

  // === Reset ===

  reset() {
    this.#loadToken += 1;
    this.#refreshToken += 1;
    this.#refreshPending = false;
    this.#loadController?.abort();
    this.#refreshController?.abort();
    this.#linksController?.abort();
    this.#worklogsController?.abort();
    this.#diagramsController?.abort();
    this.#timeModalDataController?.abort();
    this.#loadController = null;
    this.#refreshController = null;
    this.#refreshInFlight = false;
    this.#linksController = null;
    this.#worklogsController = null;
    this.#worklogsPromise = null;
    this.#worklogsPromiseItemId = null;
    this.#worklogsLoadedItemId = null;
    this.#diagramsController = null;
    this.#diagramsPromise = null;
    this.#diagramsPromiseItemId = null;
    this.#diagramsLoadedItemId = null;
    this.#timeModalDataController = null;
    this.#timeModalDataPromise = null;
    this.#timeModalDataLoaded = false;
    this.item = null;
    this.itemId = null;
    this.workspaceId = null;
    this.loading = true;
    this.error = null;
    this.saving = false;
    this.notFound = false;
    this.workspace = null;

    this.editing = { ...DEFAULT_EDITING_STATE };

    this.parentHierarchy = [];
    this.childItems = [];
    this.loadingChildItems = false;
    this.milestones = [];
    this.iterations = [];
    this.priorities = [];
    this.itemTypes = [];
    this.currentItemType = null;
    this.currentHierarchyLevel = null;
    this.availableSubIssueTypes = [];
    this.customFieldDefinitions = [];
    this.workspaceScreenFields = [];
    this.workspaceScreenSystemFields = [];
    this.requestTypeFields = [];
    this.editableScreenFieldIds = null;
    this.editableScreenSystemFields = null;
    this.availableStatusTransitions = [];
    this.loadingStatusTransitions = false;
    this.pendingApproval = null;
    this.itemLinks = [];
    this.linkTypes = [];
    this.loadingLinks = false;
    this.isWatching = false;
    this.loadingWatchStatus = false;
    this.timeProjects = [];
    this.timeWorklogs = [];
    this.timeWorklogsLoading = false;
    this.timeModalDataLoading = false;
    this.includeChildItems = false;
    this.timeRollup = null;
    this.timeRollupLoading = false;
    this.customers = [];
    this.workItems = [];
    this.workspaces = [];
    this.diagrams = [];
    this.loadingDiagrams = false;
    this.diagramsLoaded = false;
    this.manualActions = [];
    this.showDeleteDialog = false;
    this.showLinkModal = false;
    this.linkModalPreselectTypeId = null;
    this.showTestCaseModal = false;
    this.selectedTestCaseId = null;
    this.showTimeLogModal = false;
    this.editingWorklog = null;
    this.hasChanges = false;
    this.transitioning = false;
    this.dropdownItems = [];
  }
}

export const itemDetailStore = new ItemDetailStore();
