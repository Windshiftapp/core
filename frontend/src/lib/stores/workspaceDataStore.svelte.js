import { api } from '../api.js';

const AUTO_REFRESH_INTERVAL = 5 * 60 * 1000; // 5 minutes

/**
 * Shared workspace data store that caches reference data at workspace scope.
 * Initialized once on workspace entry, refreshed every 5 minutes.
 * Views read from the store instead of fetching independently.
 */
class WorkspaceDataStore {
  workspaceId = $state(null);
  workspace = $state(null);
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
    this.workspaceId = id;
    this.initialLoading = true;
    this.initialized = false;
    this.error = null;

    this._initPromise = this._fetchAll(id)
      .then(() => {
        // Race condition guard: make sure we're still on the same workspace
        if (this.workspaceId !== id) return;

        this.initialized = true;
        this.lastRefreshedAt = Date.now();
        this._startAutoRefresh();
      })
      .catch((err) => {
        if (this.workspaceId !== id) return;
        this.error = err.message || 'Failed to load workspace data';
        console.error('WorkspaceDataStore: initialization failed', err);
      })
      .finally(() => {
        if (this.workspaceId === id) {
          this.initialLoading = false;
        }
        this._initPromise = null;
      });

    return this._initPromise;
  }

  /**
   * Silent re-fetch of all reference data. On error, keeps stale data.
   */
  async refresh() {
    if (!this.workspaceId) return;

    const id = this.workspaceId;
    try {
      await this._fetchAll(id);
      if (this.workspaceId === id) {
        this.lastRefreshedAt = Date.now();
      }
    } catch (err) {
      console.warn('WorkspaceDataStore: background refresh failed, keeping stale data', err);
    }
  }

  /**
   * Clear all data and stop auto-refresh. Called when leaving workspace context.
   */
  reset() {
    this._stopAutoRefresh();
    this._initPromise = null;
    this.workspaceId = null;
    this.workspace = null;
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
    this.initialLoading = false;
    this.initialized = false;
    this.error = null;
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
    const [
      workspaceData,
      itemTypesData,
      statusesData,
      statusCategoriesData,
      usersData,
      milestonesData,
      iterationsData,
      prioritiesData,
      projectsData,
    ] = await Promise.all([
      api.workspaces.get(workspaceId),
      api.itemTypes.getAll(),
      api.workspaces.getStatuses(workspaceId),
      api.statusCategories.getAll(),
      api.getUsers(),
      api.milestones.getAll(),
      api.iterations.getAll(),
      api.priorities.getAll(),
      api.workspaces.getProjects ? api.workspaces.getProjects(workspaceId) : Promise.resolve([]),
    ]);

    // Race condition guard
    if (this.workspaceId !== workspaceId) return;

    this.workspace = workspaceData;
    this.itemTypes = itemTypesData || [];
    this.statuses = statusesData || [];
    this.statusCategories = statusCategoriesData || [];
    this.users = usersData || [];
    this.milestones = milestonesData || [];
    this.iterations = iterationsData || [];
    this.priorities = prioritiesData || [];
    this.projects = projectsData || [];

    // Custom fields loaded separately since it can fail independently
    try {
      const cfData = await api.customFields.getAll();
      if (this.workspaceId === workspaceId) {
        this.customFieldDefinitions = cfData || [];
      }
    } catch (e) {
      console.warn('WorkspaceDataStore: failed to load custom field definitions', e);
      if (this.workspaceId === workspaceId) {
        this.customFieldDefinitions = [];
      }
    }
  }

  /** @private */
  async _fetchField(workspaceId, field) {
    const fetchers = {
      workspace: () => api.workspaces.get(workspaceId),
      statuses: () => api.workspaces.getStatuses(workspaceId),
      statusCategories: () => api.statusCategories.getAll(),
      itemTypes: () => api.itemTypes.getAll(),
      users: () => api.getUsers(),
      milestones: () => api.milestones.getAll(),
      iterations: () => api.iterations.getAll(),
      priorities: () => api.priorities.getAll(),
      projects: () =>
        api.workspaces.getProjects ? api.workspaces.getProjects(workspaceId) : Promise.resolve([]),
      customFieldDefinitions: () => api.customFields.getAll(),
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
    this._refreshTimer = setInterval(() => {
      this.refresh();
    }, AUTO_REFRESH_INTERVAL);
  }

  /** @private */
  _stopAutoRefresh() {
    if (this._refreshTimer) {
      clearInterval(this._refreshTimer);
      this._refreshTimer = null;
    }
  }
}

export const workspaceDataStore = new WorkspaceDataStore();
