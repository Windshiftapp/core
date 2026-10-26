import { catalogs, customFields, itemTypes, priorities } from '../api/configuration.js';
import { iterations, milestones } from '../api/milestones.js';
import { BACKGROUND_READ, FOREGROUND_READ } from '../api/readScheduler.js';
import { getAssignableUsers, getUserDisplayBatch } from '../api/users.js';
import { statusCategories } from '../api/workflows.js';
import { workspaces } from '../api/workspaces.js';
import {
  canRunBackgroundSync,
  isExpectedBackgroundSyncError,
  onBackgroundSyncAvailable,
} from '../utils/backgroundSync.js';

const AUTO_REFRESH_INTERVAL = 5 * 60 * 1000; // 5 minutes
const USER_DISPLAY_BATCH_SIZE = 200;
const USER_DISPLAY_CACHE_LIMIT = 2000;
const USER_DISPLAY_FRESH_MS = 5 * 60 * 1000;
const USER_DISPLAY_MAX_OUTSTANDING = 2;
const SCREEN_CONFIG_CACHE_LIMIT = 50;

/**
 * Shared workspace data store that caches reference data at workspace scope.
 * Initialized once on workspace entry, refreshed every 5 minutes.
 * Views read from the store instead of fetching independently.
 */
class WorkspaceDataStore {
  workspaceId = $state(null);

  /** @type {Map<string, Promise<object>>} */
  #screenConfigPromises = new Map();
  #screenConfigEpoch = 0;
  /** @type {Map<string, true>} Insertion order is least-recently-used first. */
  #screenConfigAccess = new Map();
  /** @type {Map<number, {promise: Promise<object|null>, resolve: (user: object|null) => void}>} */
  #userHydrationPromises = new Map();
  /** @type {Map<number, number>} */
  #userDisplayFetchedAt = new Map();
  /** @type {Map<number, number>} */
  #userDisplayNegativeUntil = new Map();
  /** @type {{id: number, resolve: (user: object|null) => void}[]} */
  #userDisplayQueue = [];
  #userDisplayFlushScheduled = false;
  #userDisplayOutstanding = 0;
  #userDisplayGeneration = 0;
  /** @type {Map<number, Promise<object|null>>} */
  #bootstrapPromises = new Map();
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
  // Resolved effective configuration keyed by "<workspaceId>:<itemTypeId|default>".
  // Bounded and invalidated on workspace switch and config mutation.
  /** @type {Record<string, any>} */
  screenConfigs = $state({});
  // Whether bootstrap's milestone/iteration lists are complete directories.
  // null means the server did not report completeness (unknown, not complete).
  planningCompleteness = $state(null);

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
   * Reference data for one workspace without activating it. Returns the active
   * scope's loaded fields when they match; otherwise issues one deduplicated
   * bootstrap request instead of a fan-out of domain reads. A different active
   * scope (e.g. a global board) is left intact.
   * @param {number|string} workspaceId
   * @returns {Promise<object|null>}
   */
  async getReferenceSnapshot(workspaceId) {
    const id = typeof workspaceId === 'string' ? parseInt(workspaceId, 10) : workspaceId;
    if (!Number.isInteger(id) || id <= 0) return null;

    const isActive = Number(this.workspaceId) === id;
    if (isActive && this._initPromise) {
      await this._initPromise;
    }
    if (isActive && this.workspaceId === id && this.initialized) {
      return {
        workspace: this.workspace,
        custom_field_definitions: this.customFieldDefinitions,
        milestones: this.milestones,
        iterations: this.iterations,
        projects: this.projects,
        item_types: this.itemTypes,
        statuses: this.statuses,
        users: this.users,
        priorities: this.priorities,
        planning_completeness: this.planningCompleteness,
      };
    }

    const pending = this.#bootstrapPromises.get(id);
    if (pending) return pending;
    const promise = workspaces
      .getBootstrap(id, FOREGROUND_READ)
      .catch((error) => {
        if (error?.status === 404) return null;
        throw error;
      })
      .finally(() => {
        if (this.#bootstrapPromises.get(id) === promise) {
          this.#bootstrapPromises.delete(id);
        }
      });
    this.#bootstrapPromises.set(id, promise);
    return promise;
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
        await this.#refreshScreenConfigs();
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
    this.#screenConfigEpoch += 1;
    this.screenConfigs = {};
    this.#screenConfigAccess.clear();
    this.planningCompleteness = null;
    this.#screenConfigPromises.clear();
    this.resetUserDisplayCache();
    this.#bootstrapPromises.clear();
    this.lastRefreshedAt = null;
  }

  /**
   * Drop every cached display user and pending batch. Used on scope or
   * authentication changes so one principal's display cache never serves
   * another.
   */
  resetUserDisplayCache() {
    for (const pending of this.#userHydrationPromises.values()) pending.resolve(null);
    this.#userHydrationPromises.clear();
    this.#userDisplayFetchedAt.clear();
    this.#userDisplayNegativeUntil.clear();
    this.#userDisplayQueue.length = 0;
    this.#userDisplayFlushScheduled = false;
    this.#userDisplayGeneration += 1;
  }

  /**
   * Fetch display records for the referenced user IDs only. Unknown IDs are
   * microtask-coalesced into bounded batch reads, and results are cached for
   * this workspace scope. A workspace switch discards the old scope's work.
   * @param {Array<number|string>} ids
   */
  async hydrateUsers(ids = []) {
    const wanted = [
      ...new Set(ids.map((id) => Number(id)).filter((id) => Number.isInteger(id) && id > 0)),
    ];
    if (wanted.length === 0) return;

    await Promise.all(
      wanted.map((id) =>
        this.#displayUserFresh(id) ? Promise.resolve(null) : this.#pendingDisplayUser(id)
      )
    );
  }

  #displayUserFresh(id) {
    const negativeUntil = this.#userDisplayNegativeUntil.get(id);
    if (negativeUntil !== undefined) {
      if (negativeUntil > Date.now()) return true;
      this.#userDisplayNegativeUntil.delete(id);
    }
    if (!this.users.some((user) => user.id === id)) return false;
    const fetchedAt = this.#userDisplayFetchedAt.get(id);
    return fetchedAt === undefined || Date.now() - fetchedAt < USER_DISPLAY_FRESH_MS;
  }

  #pendingDisplayUser(id) {
    const pending = this.#userHydrationPromises.get(id);
    if (pending) return pending.promise;

    /** @type {(user: object|null) => void} */
    let resolve = () => {};
    const promise = new Promise((res) => {
      resolve = res;
    });
    const entry = { promise, resolve };
    this.#userHydrationPromises.set(id, entry);
    this.#userDisplayQueue.push({ id, resolve });
    this.#scheduleDisplayFlush();
    return promise;
  }

  #scheduleDisplayFlush() {
    if (this.#userDisplayFlushScheduled) return;
    this.#userDisplayFlushScheduled = true;
    queueMicrotask(() => {
      this.#userDisplayFlushScheduled = false;
      this.#flushDisplayBatch();
    });
  }

  #flushDisplayBatch() {
    if (
      this.#userDisplayOutstanding >= USER_DISPLAY_MAX_OUTSTANDING ||
      this.#userDisplayQueue.length === 0
    ) {
      return;
    }

    const chunk = this.#userDisplayQueue.splice(0, USER_DISPLAY_BATCH_SIZE);
    const generation = this.#userDisplayGeneration;
    this.#userDisplayOutstanding += 1;
    getUserDisplayBatch(
      chunk.map((entry) => entry.id),
      BACKGROUND_READ
    )
      .then((users) => this.#settleDisplayBatch(chunk, generation, users, null))
      .catch((error) => this.#settleDisplayBatch(chunk, generation, null, error))
      .finally(() => {
        this.#userDisplayOutstanding = Math.max(0, this.#userDisplayOutstanding - 1);
        this.#flushDisplayBatch();
      });

    this.#flushDisplayBatch();
  }

  #settleDisplayBatch(chunk, generation, users, error) {
    const now = Date.now();
    const byId = new Map((users || []).map((user) => [user.id, user]));
    const scopeChanged = generation !== this.#userDisplayGeneration;

    for (const entry of chunk) {
      const pending = this.#userHydrationPromises.get(entry.id);
      if (pending?.resolve === entry.resolve) {
        this.#userHydrationPromises.delete(entry.id);
      }
      if (scopeChanged) {
        entry.resolve(null);
        continue;
      }
      if (error) {
        // Transient failures stay retryable; only successful omissions are
        // negatively cached.
        entry.resolve(null);
        continue;
      }
      const user = byId.get(entry.id);
      if (!user) {
        this.#userDisplayNegativeUntil.set(entry.id, now + USER_DISPLAY_FRESH_MS);
        entry.resolve(null);
        continue;
      }
      this.#mergeDisplayUser(user, now);
      entry.resolve(user);
    }
  }

  #mergeDisplayUser(user, fetchedAt) {
    const byId = new Map(this.users.map((candidate) => [candidate.id, candidate]));
    byId.set(user.id, user);
    this.users = [...byId.values()];

    this.#userDisplayFetchedAt.delete(user.id);
    this.#userDisplayFetchedAt.set(user.id, fetchedAt);
    if (this.#userDisplayFetchedAt.size <= USER_DISPLAY_CACHE_LIMIT) return;

    const evicted = new Set();
    for (const id of this.#userDisplayFetchedAt.keys()) {
      if (this.#userDisplayFetchedAt.size <= USER_DISPLAY_CACHE_LIMIT) break;
      this.#userDisplayFetchedAt.delete(id);
      evicted.add(id);
    }
    if (evicted.size > 0) {
      this.users = this.users.filter((candidate) => !evicted.has(candidate.id));
    }
  }

  /**
   * Resolved effective configuration for the current workspace and optional
   * item type. Convenience delegate for the workspace-scoped callers; see
   * screenConfigFor for the cross-workspace contract.
   */
  screenConfig(itemTypeId = null) {
    return this.screenConfigFor(this.workspaceId, itemTypeId);
  }

  /**
   * Resolved effective configuration for any workspace and optional item type:
   * assigned config set (else the global default), its effective screens with
   * fields, and config-set priorities. Cached per (workspace, item type);
   * in-flight requests are shared and the cache is bounded. A delayed response
   * is written only when the cache has not been invalidated in the meantime.
   */
  async screenConfigFor(workspaceId, itemTypeId = null) {
    if (!workspaceId || workspaceId === 'global') return null;
    const key = `${workspaceId}:${itemTypeId == null ? 'default' : itemTypeId}`;
    const cached = this.screenConfigs[key];
    if (cached) {
      this.#touchScreenConfig(key);
      return cached;
    }

    const pending = this.#screenConfigPromises.get(key);
    if (pending) return pending;

    const epoch = this.#screenConfigEpoch;
    const promise = workspaces
      .getEffectiveConfig(workspaceId, itemTypeId, FOREGROUND_READ)
      .then((config) => {
        if (this.#screenConfigEpoch === epoch) {
          this.#storeScreenConfig(key, config);
        }
        return config;
      })
      .finally(() => {
        if (this.#screenConfigPromises.get(key) === promise) {
          this.#screenConfigPromises.delete(key);
        }
      });
    this.#screenConfigPromises.set(key, promise);
    return promise;
  }

  /** Drop cached effective configurations (after admin config changes). */
  invalidateScreenConfigs() {
    this.#screenConfigEpoch += 1;
    this.screenConfigs = {};
    this.#screenConfigAccess.clear();
    this.#screenConfigPromises.clear();
  }

  #storeScreenConfig(key, config) {
    this.screenConfigs = { ...this.screenConfigs, [key]: config };
    this.#touchScreenConfig(key);
    while (this.#screenConfigAccess.size > SCREEN_CONFIG_CACHE_LIMIT) {
      const oldest = /** @type {string} */ (this.#screenConfigAccess.keys().next().value);
      if (oldest === key) break;
      this.#screenConfigAccess.delete(oldest);
      const { [oldest]: _evicted, ...rest } = this.screenConfigs;
      this.screenConfigs = rest;
    }
  }

  #touchScreenConfig(key) {
    this.#screenConfigAccess.delete(key);
    this.#screenConfigAccess.set(key, true);
  }

  // Silent re-fetch of cached screen configurations; keeps stale data on error.
  async #refreshScreenConfigs() {
    const keys = Object.keys(this.screenConfigs);
    if (keys.length === 0) return;
    await Promise.all(
      keys.map(async (key) => {
        const [workspacePart, itemTypePart] = key.split(':');
        const workspaceId = Number(workspacePart);
        const itemTypeId = itemTypePart === 'default' ? null : Number(itemTypePart);
        if (!Number.isInteger(workspaceId) || workspaceId <= 0) return;
        const epoch = this.#screenConfigEpoch;
        try {
          const config = await workspaces.getEffectiveConfig(workspaceId, itemTypeId);
          if (this.#screenConfigEpoch === epoch) {
            this.#storeScreenConfig(key, config);
          }
        } catch (err) {
          if (!isExpectedBackgroundSyncError(err)) {
            console.warn(
              'WorkspaceDataStore: screen config refresh failed, keeping stale data',
              err
            );
          }
        }
      })
    );
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
    const bootstrap = await workspaces.getBootstrap(workspaceId, FOREGROUND_READ);

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
    // Absent metadata means unknown; never treat it as complete.
    this.planningCompleteness = bootstrap?.planning_completeness ?? null;
  }

  hydrateHomepageLayout(workspaceId, layout) {
    const id = typeof workspaceId === 'string' ? parseInt(workspaceId, 10) : workspaceId;
    if (this.workspaceId === id) this.homepageLayout = layout;
  }

  /** @private */
  async _fetchAllGlobal() {
    const bootstrap = await catalogs.getGlobalBootstrap(FOREGROUND_READ);

    if (this.workspaceId !== 'global') return;

    this.workspace = null;
    this.itemTypes = bootstrap?.item_types ?? [];
    this.statuses = bootstrap?.statuses ?? [];
    this.statusCategories = bootstrap?.status_categories ?? [];
    this.priorities = bootstrap?.priorities ?? [];
    // The global roster is deliberately not enumerated here: with a large
    // directory that is many sequential pages before anything can render.
    // Views hydrate only the users their visible rows reference. Hydrated
    // user-display records survive reference refreshes; only a scope or auth
    // reset clears them.
    this.milestones = [];
    this.iterations = [];
    this.projects = [];
    this.labels = [];

    if (bootstrap?.custom_field_definitions != null) {
      this.customFieldDefinitions = bootstrap.custom_field_definitions;
    } else {
      console.warn(
        'WorkspaceDataStore: failed to load custom field definitions',
        bootstrap?.custom_fields_error
      );
      this.customFieldDefinitions = [];
    }
  }

  /** @private */
  async _fetchField(workspaceId, field) {
    const fetchers = {
      workspace: () => workspaces.get(workspaceId),
      homepageLayout: () => workspaces.getHomepageLayout(workspaceId),
      statuses: () => workspaces.getStatuses(workspaceId),
      statusCategories: () => statusCategories.getAll(),
      itemTypes: () => itemTypes.getAll(),
      // Global users are hydrated per referenced ID by hydrateUsers(); a field
      // refresh must not fall back to enumerating the whole directory.
      users: () =>
        workspaceId === 'global' ? Promise.resolve(this.users) : getAssignableUsers(workspaceId),
      milestones: () =>
        workspaceId === 'global'
          ? milestones.getAll()
          : milestones.getAll({ workspace_id: workspaceId, include_global: true }),
      iterations: () => iterations.getAll(),
      priorities: () => priorities.getAll(),
      projects: () =>
        workspaces.getProjects ? workspaces.getProjects(workspaceId) : Promise.resolve([]),
      customFieldDefinitions: async () => {
        const res = await customFields.getAll();
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
