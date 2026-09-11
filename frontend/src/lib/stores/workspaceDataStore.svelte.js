import { api } from '../api.js';
import {
  canRunBackgroundSync,
  isExpectedBackgroundSyncError,
  onBackgroundSyncAvailable,
} from '../utils/backgroundSync.js';

const AUTO_REFRESH_INTERVAL = 5 * 60 * 1000; // 5 minutes

/**
 * Shared workspace data store that caches reference data at workspace scope.
 * Initialized once on workspace entry, refreshed every 5 minutes.
 * Views read from the store instead of fetching independently.
 */
class WorkspaceDataStore {
  workspaceId = $state(null);
  workspace = $state(null);
  homepageLayout = $state(null);
  statuses = $state([]);
  statusCategories = $state([]);
  itemTypes = $state([]);
  users = $state([]);
  milestones = $state([]);
  iterations = $state([]);
  priorities = $state([]);
  projects = $state([]);
  customFieldDefinitions = $state([]);
  labels = $state([]);

  initialLoading = $state(false);
  initialized = $state(false);
  error = $state(null);
  lastRefreshedAt = $state(null);

  /** @type {Promise|null} */
  _initPromise = null;
  /** @type {number|null} */
  _refreshTimer = null;
  /** @type {Promise<void>|null} */
  _refreshPromise = null;
  /** @type {null|(() => void)} */
  _stopReconnectListener = null;

  /**
   * Initialize store for a workspace. Idempotent — if already initialized
   * for this workspace, returns immediately. If an initialization is in flight,
   * returns that promise.
   */
  async initialize(workspaceId) {
    if (!workspaceId) return;

    const id = typeof workspaceId === 'string' ? parseInt(workspaceId, 10) : workspaceId;

    // Already initialized for this workspace
    if (this.initialized && this.workspaceId === id) {
      return;
    }

    // Initialization in flight for the same workspace
    if (this._initPromise && this.workspaceId === id) {
      return this._initPromise;
    }

    // Different workspace or first init — reset and start fresh
    this._stopAutoRefresh();
    this._refreshPromise = null;
    this.workspaceId = id;
    this._clearData();
    this.initialLoading = true;
    this.initialized = false;
    this.error = null;

    const request = this._fetchAll(id)
      .then(() => {
        // Race condition guard: make sure we're still on the same workspace
        if (this.workspaceId !== id) return;

        this.initialized = true;
        this.lastRefreshedAt = Date.now();
        this._startAutoRefresh();
      })
      .catch((err) => {
        if (this.workspaceId !== id) return;
        // A navigation/reload aborts outstanding fetches as the document is
        // torn down. That is expected control flow, not an initialization
        // failure to surface in the UI or browser console.
        if (isExpectedBackgroundSyncError(err)) return;
        this.error = err.message || 'Failed to load workspace data';
        console.error('WorkspaceDataStore: initialization failed', err);
      })
      .finally(() => {
        if (this.workspaceId === id) {
          this.initialLoading = false;
        }
        if (this._initPromise === request) this._initPromise = null;
      });

    this._initPromise = request;
    return request;
  }

  /**
   * Initialize store for global context (no workspace).
   * Loads global reference data: statuses, item types, users, priorities, status categories.
   * Workspace-scoped data (milestones, iterations, projects) is set to empty.
   */
  async initializeGlobal() {
    const GLOBAL_SENTINEL = 'global';

    // Already initialized for global context
    if (this.initialized && this.workspaceId === GLOBAL_SENTINEL) {
      return;
    }

    if (this._initPromise && this.workspaceId === GLOBAL_SENTINEL) {
      return this._initPromise;
    }

    this._stopAutoRefresh();
    this._refreshPromise = null;
    this.workspaceId = GLOBAL_SENTINEL;
    this._clearData();
    this.initialLoading = true;
    this.initialized = false;
    this.error = null;

    const request = this._fetchAllGlobal()
      .then(() => {
        if (this.workspaceId !== GLOBAL_SENTINEL) return;
        this.initialized = true;
        this.lastRefreshedAt = Date.now();
        this._startAutoRefresh();
      })
      .catch((err) => {
        if (this.workspaceId !== GLOBAL_SENTINEL) return;
        if (isExpectedBackgroundSyncError(err)) return;
        this.error = err.message || 'Failed to load global data';
        console.error('WorkspaceDataStore: global initialization failed', err);
      })
      .finally(() => {
        if (this.workspaceId === GLOBAL_SENTINEL) {
          this.initialLoading = false;
        }
        if (this._initPromise === request) this._initPromise = null;
      });

    this._initPromise = request;
    return request;
  }

  /**
   * Silent re-fetch of all reference data. On error, keeps stale data.
   */
  async refresh() {
    if (!this.workspaceId) return;
    if (!canRunBackgroundSync()) return;
    if (this._refreshPromise) return this._refreshPromise;

    const id = this.workspaceId;
    const request = (async () => {
      try {
        if (id === 'global') {
          await this._fetchAllGlobal();
        } else {
          await this._fetchAll(id);
        }
        if (this.workspaceId === id) {
          this.lastRefreshedAt = Date.now();
        }
      } catch (err) {
        if (!isExpectedBackgroundSyncError(err)) {
          console.warn('WorkspaceDataStore: background refresh failed, keeping stale data', err);
        }
      }
    })();

    this._refreshPromise = request;
    try {
      await request;
    } finally {
      if (this._refreshPromise === request) this._refreshPromise = null;
    }
  }

  /**
   * Clear all data and stop auto-refresh. Called when leaving workspace context.
   */
  reset() {
    this._stopAutoRefresh();
    this._initPromise = null;
    this._refreshPromise = null;
    this.workspaceId = null;
    this._clearData();
    this.initialLoading = false;
    this.initialized = false;
    this.error = null;
  }

  /** @private */
  _clearData() {
    this.workspace = null;
    this.homepageLayout = null;
    this.statuses = [];
    this.statusCategories = [];
    this.itemTypes = [];
    this.users = [];
    this.milestones = [];
    this.iterations = [];
    this.priorities = [];
    this.projects = [];
    this.customFieldDefinitions = [];
    this.labels = [];
    this.lastRefreshedAt = null;
  }

  /**
   * Granular re-fetch for a specific field, or everything if no field specified.
   */
  async invalidate(field) {
    if (!this.workspaceId) return;

    const id = this.workspaceId;

    if (!field) {
      return this.refresh();
    }

    try {
      const data = await this._fetchField(id, field);
      if (this.workspaceId === id && data !== undefined) {
        this[field] = data;
      }
    } catch (err) {
      console.warn(`WorkspaceDataStore: failed to invalidate "${field}"`, err);
    }
  }

  /** @private */
  async _fetchAll(workspaceId) {
    const bootstrap = await api.workspaces.getBootstrap(workspaceId);

    // Race condition guard
    if (this.workspaceId !== workspaceId) return;

    this.workspace = bootstrap?.workspace ?? null;
    this.homepageLayout = bootstrap?.homepage_layout ?? null;
    this.itemTypes = bootstrap?.item_types ?? [];
    this.statuses = bootstrap?.statuses ?? [];
    this.statusCategories = bootstrap?.status_categories ?? [];
    this.users = bootstrap?.users ?? [];
    this.milestones = bootstrap?.milestones ?? [];
    this.iterations = bootstrap?.iterations ?? [];
    this.priorities = bootstrap?.priorities ?? [];
    this.projects = bootstrap?.projects ?? [];
    this.customFieldDefinitions = bootstrap?.custom_field_definitions ?? [];
  }

  hydrateHomepageLayout(workspaceId, layout) {
    const id = typeof workspaceId === 'string' ? parseInt(workspaceId, 10) : workspaceId;
    if (this.workspaceId === id) this.homepageLayout = layout;
  }

  /** @private */
  async _fetchAllGlobal() {
    const [itemTypesData, statusesData, statusCategoriesData, usersData, prioritiesData] =
      await Promise.all([
        api.itemTypes.getAll(),
        api.statuses.getAll(),
        api.statusCategories.getAll(),
        api.getUsers(),
        api.priorities.getAll(),
      ]);

    if (this.workspaceId !== 'global') return;

    this.workspace = null;
    this.itemTypes = itemTypesData || [];
    this.statuses = statusesData || [];
    this.statusCategories = statusCategoriesData || [];
    this.users = usersData || [];
    this.priorities = prioritiesData || [];
    this.milestones = [];
    this.iterations = [];
    this.projects = [];
    this.labels = [];

    try {
      const cfData = await api.customFields.getAll();
      if (this.workspaceId === 'global') {
        this.customFieldDefinitions = cfData || [];
      }
    } catch (e) {
      console.warn('WorkspaceDataStore: failed to load custom field definitions', e);
      if (this.workspaceId === 'global') {
        this.customFieldDefinitions = [];
      }
    }
  }

  /** @private */
  async _fetchField(workspaceId, field) {
    const fetchers = {
      workspace: () => api.workspaces.get(workspaceId),
      homepageLayout: () => api.workspaces.getHomepageLayout(workspaceId),
      statuses: () => api.workspaces.getStatuses(workspaceId),
      statusCategories: () => api.statusCategories.getAll(),
      itemTypes: () => api.itemTypes.getAll(),
      users: () =>
        workspaceId === 'global' ? api.getUsers() : api.getAssignableUsers(workspaceId),
      milestones: () =>
        workspaceId === 'global'
          ? api.milestones.getAll()
          : api.milestones.getAll({ workspace_id: workspaceId, include_global: true }),
      iterations: () => api.iterations.getAll(),
      priorities: () => api.priorities.getAll(),
      projects: () =>
        api.workspaces.getProjects ? api.workspaces.getProjects(workspaceId) : Promise.resolve([]),
      customFieldDefinitions: async () => {
        const res = await api.customFields.getAll();
        return res || [];
      },
    };

    const fetcher = fetchers[field];
    if (!fetcher) {
      console.warn(`WorkspaceDataStore: unknown field "${field}"`);
      return undefined;
    }

    const data = await fetcher();
    return data || [];
  }

  /** @private */
  _startAutoRefresh() {
    this._stopAutoRefresh();
    this._refreshTimer = window.setInterval(() => {
      if (canRunBackgroundSync()) void this.refresh();
    }, AUTO_REFRESH_INTERVAL);
    this._stopReconnectListener = onBackgroundSyncAvailable(() => {
      void this.refresh();
    });
  }

  /** @private */
  _stopAutoRefresh() {
    if (this._refreshTimer) {
      window.clearInterval(this._refreshTimer);
      this._refreshTimer = null;
    }
    this._stopReconnectListener?.();
    this._stopReconnectListener = null;
  }
}

export const workspaceDataStore = new WorkspaceDataStore();
