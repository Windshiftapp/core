/**
 * Store for tracking plugin capabilities.
 * Capabilities are loaded from /api/features and used to gate UI features.
 */

class CapabilitiesStore {
  /** @type {Set<string>} */
  capabilities = $state(new Set());
  sshAvailable = $state(false);
  loaded = $state(false);
  #loadPromise = null;
  #hydrationPromise = null;
  #resolveHydration = null;

  hydrate(data) {
    if (!data) return;
    this.capabilities = new Set(data.capabilities || []);
    this.sshAvailable = data.ssh_available === true;
    this.loaded = true;
    this.#finishHydration(true);
  }

  beginHydration() {
    if (this.loaded) return Promise.resolve(true);
    if (!this.#hydrationPromise) {
      this.#hydrationPromise = new Promise((resolve) => {
        this.#resolveHydration = resolve;
      });
    }
    return this.#hydrationPromise;
  }

  failHydration() {
    this.#finishHydration(false);
  }

  #finishHydration(hydrated) {
    this.#resolveHydration?.(hydrated);
    this.#resolveHydration = null;
    this.#hydrationPromise = null;
  }

  /**
   * Load capabilities from the features endpoint.
   */
  async load() {
    if (this.loaded) return;
    if (this.#hydrationPromise) {
      await this.#hydrationPromise;
      if (this.loaded) return;
    }
    if (this.#loadPromise) return this.#loadPromise;

    const request = this.#load().finally(() => {
      if (this.#loadPromise === request) this.#loadPromise = null;
    });
    this.#loadPromise = request;
    return request;
  }

  async #load() {
    try {
      const resp = await fetch('/api/features');
      if (resp.ok) {
        const data = await resp.json();
        this.hydrate(data);
      }
    } catch (err) {
      console.warn('Failed to load capabilities:', err);
    } finally {
      this.loaded = true;
    }
  }

  reset() {
    this.capabilities = new Set();
    this.sshAvailable = false;
    this.loaded = false;
    this.#loadPromise = null;
    this.#finishHydration(false);
  }

  /**
   * Check if a capability is available.
   * @param {string} name
   * @returns {boolean}
   */
  has(name) {
    return this.capabilities.has(name);
  }
}

export const capabilitiesStore = new CapabilitiesStore();
