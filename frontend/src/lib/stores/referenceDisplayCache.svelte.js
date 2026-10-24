import { assets } from '../api/assets.js';
import { getUsers } from '../api/users.js';

const ASSET_CACHE_TTL_MS = 5 * 60_000;
const MAX_ASSET_CACHE_ENTRIES = 2000;
const ASSET_BATCH_SIZE = 500;

function validAssetID(value) {
  const id = Number.parseInt(String(value), 10);
  return Number.isFinite(id) && id > 0 ? id : null;
}

class ReferenceDisplayCache {
  users = $state([]);
  usersLoading = $state(false);
  assets = $state(new Map());

  #usersPromise = null;
  #pendingAssetLoads = [];
  #flushScheduled = false;
  #generation = 0;

  async loadUsers() {
    if (this.users.length > 0) return this.users;
    if (this.#usersPromise) return this.#usersPromise;

    const generation = this.#generation;
    this.usersLoading = true;
    const request = Promise.resolve(getUsers())
      .then((users) => {
        if (generation !== this.#generation) return [];
        this.users = users || [];
        return this.users;
      })
      .catch((error) => {
        if (generation !== this.#generation) return [];
        console.error('Failed to load user display references:', error);
        this.users = [];
        return this.users;
      })
      .finally(() => {
        if (generation !== this.#generation) return;
        this.usersLoading = false;
        if (this.#usersPromise === request) this.#usersPromise = null;
      });
    this.#usersPromise = request;
    return this.#usersPromise;
  }

  getAsset(id) {
    const assetID = validAssetID(id);
    if (!assetID) return undefined;
    const entry = this.assets.get(assetID);
    if (!entry) return undefined;
    if (entry.expiresAt <= Date.now()) {
      const next = new Map(this.assets);
      next.delete(assetID);
      this.assets = next;
      return undefined;
    }
    return entry.value;
  }

  /**
   * @param {Array<number|string>} ids
   * @param {{ signal?: AbortSignal }} [options]
   */
  loadAssets(ids, options = {}) {
    const { signal } = options;
    const missing = [...new Set(ids.map(validAssetID).filter(Boolean))].filter(
      (id) => this.getAsset(id) === undefined
    );
    if (missing.length === 0 || signal?.aborted) return Promise.resolve();

    return new Promise((resolve) => {
      this.#pendingAssetLoads.push({ ids: missing, resolve, signal });
      if (!this.#flushScheduled) {
        this.#flushScheduled = true;
        queueMicrotask(() => this.#flushAssetLoads());
      }
    });
  }

  async #flushAssetLoads() {
    const generation = this.#generation;
    this.#flushScheduled = false;
    const pending = this.#pendingAssetLoads.splice(0);
    const requests = pending.filter((request) => !request.signal?.aborted);
    pending.filter((request) => request.signal?.aborted).forEach((request) => request.resolve());
    const ids = [
      ...new Set(
        requests.flatMap((request) => request.ids).filter((id) => this.getAsset(id) === undefined)
      ),
    ];
    if (requests.length === 0 || ids.length === 0) {
      requests.forEach((request) => request.resolve());
      return;
    }

    const controller = new AbortController();
    const abortIfUnused = () => {
      if (requests.every((request) => request.signal?.aborted)) controller.abort();
    };
    requests.forEach((request) =>
      request.signal?.addEventListener('abort', abortIfUnused, { once: true })
    );

    try {
      const summaries = [];
      for (let start = 0; start < ids.length; start += ASSET_BATCH_SIZE) {
        const chunk = ids.slice(start, start + ASSET_BATCH_SIZE);
        summaries.push(
          ...((await assets.getSummaries(chunk, { signal: controller.signal })) || [])
        );
      }

      const byID = new Map(summaries.map((asset) => [asset.id, asset]));
      if (generation !== this.#generation) return;
      const next = new Map(this.assets);
      const expiresAt = Date.now() + ASSET_CACHE_TTL_MS;
      ids.forEach((id) => next.set(id, { value: byID.get(id) ?? null, expiresAt }));
      while (next.size > MAX_ASSET_CACHE_ENTRIES) next.delete(next.keys().next().value);
      this.assets = next;
    } catch (error) {
      if (error?.name !== 'AbortError' && generation === this.#generation) {
        console.error('Failed to batch asset display values:', error);
        const next = new Map(this.assets);
        const expiresAt = Date.now() + ASSET_CACHE_TTL_MS;
        ids.forEach((id) => next.set(id, { value: null, expiresAt }));
        this.assets = next;
      }
    } finally {
      requests.forEach((request) => {
        request.signal?.removeEventListener('abort', abortIfUnused);
        request.resolve();
      });
    }
  }

  reset() {
    this.#generation += 1;
    this.users = [];
    this.usersLoading = false;
    this.assets = new Map();
    this.#usersPromise = null;
    this.#pendingAssetLoads.splice(0).forEach((request) => request.resolve());
    this.#flushScheduled = false;
  }
}

export const referenceDisplayCache = new ReferenceDisplayCache();
