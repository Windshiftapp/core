import { assets } from '../api/assets.js';
import { priorities } from '../api/configuration.js';
import { iterations, milestones } from '../api/milestones.js';
import { customerOrganisations, portalCustomers } from '../api/portal.js';
import { teams } from '../api/teams.js';
import { getAssignableUsers } from '../api/users.js';
import { workspaces } from '../api/workspaces.js';
import { realCustomers } from '../utils/customerLabel.js';

const EMPTY_OPTIONS = Object.freeze({
  statuses: [],
  users: [],
  teams: [],
  milestones: [],
  iterations: [],
  priorities: [],
  projects: [],
  portalCustomers: [],
  customerOrganisations: [],
  loaded: {},
  loading: {},
  fetchedAt: {},
  complete: {},
});

const OPTION_TTL_MS = 5 * 60 * 1000;
// Planning families carry explicit completeness. A partial page is still
// displayable, but it can never satisfy a caller that needs the full list.
const PLANNING_FIELDS = new Set(['milestones', 'iterations']);
const MAX_ASSET_CACHE_ENTRIES = 200;

const OPTION_LOADERS = {
  statuses: (workspaceId) => workspaces.getStatuses(workspaceId),
  users: (workspaceId) => getAssignableUsers(workspaceId),
  teams: () => teams.getAll(),
  milestones: (workspaceId) =>
    milestones.getAll({ workspace_id: workspaceId, include_global: true }),
  iterations: (workspaceId) => iterations.getAll({ workspace_id: workspaceId }),
  priorities: async (workspaceId) => {
    const workspace = await workspaces.get(workspaceId);
    const configurationSetId = workspace?.configuration_set_id;
    if (!configurationSetId) return priorities.getAll();

    const configuredPriorities = await priorities.getAll({
      configuration_set_id: configurationSetId,
    });
    return configuredPriorities.length > 0 ? configuredPriorities : priorities.getAll();
  },
  projects: (workspaceId) => workspaces.getProjects(workspaceId),
  portalCustomers: () => portalCustomers.getAll(),
  customerOrganisations: async () => realCustomers(await customerOrganisations.getAll()),
};

/**
 * Rows are equal when their primitive fields match. Nested objects are compared
 * in load(); callers prime with fresh slices of the same workspace-store rows,
 * so ignoring them keeps re-priming idempotent.
 */
function sameRow(left, right) {
  if (left === right) return true;
  if (!left || !right || typeof left !== 'object') return false;
  const leftKeys = Object.keys(left);
  if (leftKeys.length !== Object.keys(right).length) return false;
  for (const key of leftKeys) {
    const leftValue = left[key];
    const rightValue = right[key];
    if (leftValue === rightValue) continue;
    if (leftValue && rightValue && typeof leftValue === 'object') continue;
    return false;
  }
  return true;
}

function sameArrayContents(left, right) {
  if (left === right) return true;
  if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) return false;
  return left.every((value, index) => sameRow(value, right[index]));
}

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
  prime(workspaceId, values, completeness = null) {
    const id = normalizeWorkspaceId(workspaceId);
    if (!id) return;

    const current = this.get(id);
    const next = {
      ...current,
      loaded: { ...current.loaded },
      loading: { ...current.loading },
      fetchedAt: { ...current.fetchedAt },
      complete: { ...current.complete },
    };
    let changed = false;

    for (const field of ['statuses', 'users', 'milestones', 'iterations', 'projects']) {
      const incoming = values?.[field];
      // A fresh, complete entry is authoritative: a later partial priming pass
      // must not downgrade it back to an incomplete page.
      if (
        !Array.isArray(incoming) ||
        (current.loaded[field] && this.#isFresh(current, field) && this.#isComplete(current, field))
      ) {
        continue;
      }

      const complete = PLANNING_FIELDS.has(field) ? completeness?.[field] === true : true;
      // Priming runs inside effects. Writing an identical entry would mark the
      // effect dirty again and spin until Svelte aborts the update depth.
      const unchanged =
        current.loaded[field] === true &&
        sameArrayContents(current[field], incoming) &&
        this.#isComplete(current, field) === complete;
      if (unchanged) continue;

      next[field] = incoming;
      next.loaded[field] = true;
      next.fetchedAt[field] = Date.now();
      if (PLANNING_FIELDS.has(field)) {
        next.complete[field] = complete;
      }
      changed = true;
    }

    if (changed) this.byWorkspace = { ...this.byWorkspace, [id]: next };
  }

  async load(workspaceId, field) {
    const id = normalizeWorkspaceId(workspaceId);
    const loader = OPTION_LOADERS[field];
    if (!id || !loader) return [];

    const current = this.get(id);
    if (current.loaded[field] && this.#isFresh(current, field)) {
      // Partial planning data can be displayed, but a caller asking for the
      // family needs the full directory, so keep loading until it is complete.
      if (!PLANNING_FIELDS.has(field) || this.#isComplete(current, field)) {
        return current[field];
      }
    }

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
          // The scoped planning loaders drain every page, so a resolved load
          // is a complete list.
          complete: PLANNING_FIELDS.has(field)
            ? { ...entry.complete, [field]: true }
            : entry.complete,
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
    const promise = Promise.resolve(assets.getAll(setId, filters))
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

  /**
   * Drop one option family across every workspace. Global planning mutations
   * change a global-inclusive directory, so every scoped cache is stale.
   */
  invalidateFamily(field) {
    if (!field) return;
    const next = { ...this.byWorkspace };
    for (const [workspaceId, entry] of Object.entries(next)) {
      next[workspaceId] = {
        ...entry,
        [field]: [],
        loaded: { ...entry.loaded, [field]: false },
        fetchedAt: { ...entry.fetchedAt, [field]: 0 },
        complete: { ...entry.complete, [field]: false },
      };
    }
    this.byWorkspace = next;
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
      complete: { ...entry.complete, [field]: false },
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

  #isComplete(entry, field) {
    return !PLANNING_FIELDS.has(field) || entry.complete?.[field] === true;
  }
}

export const collectionEditorOptions = new CollectionEditorOptionsStore();
