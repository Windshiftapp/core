import { api } from '../api.js';

const EMPTY_OPTIONS = Object.freeze({
  statuses: [],
  users: [],
  milestones: [],
  iterations: [],
  priorities: [],
  projects: [],
  portalCustomers: [],
  customerOrganisations: [],
  personalLabels: [],
  loaded: {},
  loading: {},
  fetchedAt: {},
});

const OPTION_TTL_MS = 5 * 60 * 1000;
const MAX_ASSET_CACHE_ENTRIES = 200;

const OPTION_LOADERS = {
  statuses: (workspaceId) => api.workspaces.getStatuses(workspaceId),
  users: (workspaceId) => api.getAssignableUsers(workspaceId),
  milestones: (workspaceId) =>
    api.milestones.getAll({ workspace_id: workspaceId, include_global: true }),
  iterations: (workspaceId) => api.iterations.getAll({ workspace_id: workspaceId }),
  priorities: async (workspaceId) => {
    const workspace = await api.workspaces.get(workspaceId);
    const configurationSetId = workspace?.configuration_set_id;
    if (!configurationSetId) return api.priorities.getAll();

    const configuredPriorities = await api.priorities.getAll({
      configuration_set_id: configurationSetId,
    });
    return configuredPriorities.length > 0 ? configuredPriorities : api.priorities.getAll();
  },
  projects: (workspaceId) => api.workspaces.getProjects(workspaceId),
  portalCustomers: () => api.portalCustomers.getAll(),
  customerOrganisations: () => api.customerOrganisations.getAll(),
  personalLabels: () => api.personalLabels.getAll(null),
};

function normalizeWorkspaceId(workspaceId) {
  const id = Number(workspaceId);
  return Number.isInteger(id) && id > 0 ? id : null;
}

/**
 * Lazy editable-list options, partitioned by the workspace that owns each row.
 * A global collection can contain items from many workspaces; keeping each
 * option family under its workspace ID prevents configuration from one row
 * leaking into another row's picker.
 */
export class CollectionEditorOptionsStore {
  byWorkspace = $state({});

  /** @type {Map<string, Promise<unknown>>} */
  #pending = new Map();

  /** @type {Map<string, { value: any, fetchedAt: number }>} */
  #assetCache = new Map();
  #generation = 0;

  get(workspaceId) {
    const id = normalizeWorkspaceId(workspaceId);
    return id ? (this.byWorkspace[id] ?? EMPTY_OPTIONS) : EMPTY_OPTIONS;
  }

  /**
   * Seed option families already loaded for a workspace-scoped collection.
   * Global collections intentionally do not prime this cache because their
   * page-level reference data is not scoped to any one row workspace.
   */
  prime(workspaceId, values) {
    const id = normalizeWorkspaceId(workspaceId);
    if (!id) return;

    const current = this.get(id);
    const next = {
      ...current,
      loaded: { ...current.loaded },
      loading: { ...current.loading },
      fetchedAt: { ...current.fetchedAt },
    };
    let changed = false;

    for (const field of ['statuses', 'users', 'milestones', 'iterations', 'projects']) {
      if (
        !Array.isArray(values?.[field]) ||
        (current.loaded[field] && this.#isFresh(current, field))
      ) {
        continue;
      }
      next[field] = values[field];
      next.loaded[field] = true;
      next.fetchedAt[field] = Date.now();
      changed = true;
    }

    if (changed) this.byWorkspace = { ...this.byWorkspace, [id]: next };
  }

  async load(workspaceId, field) {
    const id = normalizeWorkspaceId(workspaceId);
    const loader = OPTION_LOADERS[field];
    if (!id || !loader) return [];

    const current = this.get(id);
    if (current.loaded[field] && this.#isFresh(current, field)) return current[field];

    const pendingKey = `${id}:${field}`;
    const existing = this.#pending.get(pendingKey);
    if (existing) return existing;

    this.#update(id, (entry) => ({
      ...entry,
      loading: { ...entry.loading, [field]: true },
    }));

    const generation = this.#generation;
    const promise = Promise.resolve(loader(id))
      .then((result) => {
        if (generation !== this.#generation) return [];
        const options = Array.isArray(result) ? result : [];
        this.#update(id, (entry) => ({
          ...entry,
          [field]: options,
          loaded: { ...entry.loaded, [field]: true },
          fetchedAt: { ...entry.fetchedAt, [field]: Date.now() },
        }));
        return options;
      })
      .catch((error) => {
        if (generation !== this.#generation) return [];
        console.error(
          `CollectionEditorOptionsStore: failed to load ${field} for workspace ${id}`,
          error
        );
        return [];
      })
      .finally(() => {
        if (generation !== this.#generation) return;
        this.#pending.delete(pendingKey);
        this.#update(id, (entry) => ({
          ...entry,
          loading: { ...entry.loading, [field]: false },
        }));
      });

    this.#pending.set(pendingKey, promise);
    return promise;
  }

  async loadAssets(workspaceId, assetSetId, cqlQuery = '', search = '') {
    const id = normalizeWorkspaceId(workspaceId);
    const setId = Number(assetSetId);
    if (!id || !Number.isInteger(setId) || setId <= 0) {
      return { assets: [], total: 0 };
    }

    const key = `${id}:assets:${setId}:${cqlQuery}:${search}`;
    const cached = this.#assetCache.get(key);
    if (cached && Date.now() - cached.fetchedAt <= OPTION_TTL_MS) return cached.value;
    if (cached) this.#assetCache.delete(key);

    const existing = this.#pending.get(key);
    if (existing) return existing;

    const filters = { cql: cqlQuery || undefined, search: search || undefined };
    const generation = this.#generation;
    const promise = Promise.resolve(api.assets.getAll(setId, filters))
      .then((result) => {
        if (generation !== this.#generation) return { assets: [], total: 0 };
        const normalized = {
          assets: Array.isArray(result?.data) ? result.data : [],
          total: Number(result?.pagination?.total_items) || 0,
        };
        this.#assetCache.set(key, { value: normalized, fetchedAt: Date.now() });
        while (this.#assetCache.size > MAX_ASSET_CACHE_ENTRIES) {
          this.#assetCache.delete(this.#assetCache.keys().next().value);
        }
        return normalized;
      })
      .finally(() => {
        if (generation === this.#generation) this.#pending.delete(key);
      });

    this.#pending.set(key, promise);
    return promise;
  }

  invalidate(workspaceId, field = null) {
    const id = normalizeWorkspaceId(workspaceId);
    if (!id) return;

    if (!field) {
      const next = { ...this.byWorkspace };
      delete next[id];
      this.byWorkspace = next;
      return;
    }

    this.#update(id, (entry) => ({
      ...entry,
      [field]: [],
      loaded: { ...entry.loaded, [field]: false },
      fetchedAt: { ...entry.fetchedAt, [field]: 0 },
    }));
  }

  reset() {
    this.#generation += 1;
    this.#pending.clear();
    this.#assetCache.clear();
    this.byWorkspace = {};
  }

  #update(workspaceId, update) {
    const current = this.byWorkspace[workspaceId] ?? EMPTY_OPTIONS;
    this.byWorkspace = {
      ...this.byWorkspace,
      [workspaceId]: update(current),
    };
  }

  #isFresh(entry, field) {
    return Date.now() - (entry.fetchedAt[field] ?? 0) <= OPTION_TTL_MS;
  }
}

export const collectionEditorOptions = new CollectionEditorOptionsStore();
