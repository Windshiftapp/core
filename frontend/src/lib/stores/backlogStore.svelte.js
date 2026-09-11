import { api } from '../api.js';

/**
 * Store for managing backlog count across workspace views.
 * Uses Svelte 5 class-based reactive state.
 */
class BacklogStore {
  workspaceId = $state(null);
  count = $state(0);
  loading = $state(false);
  #loadGeneration = 0;

  /**
   * Load backlog count for a workspace.
   * Skips fetch if already loaded for this workspace.
   */
  async load(workspaceId) {
    if (this.workspaceId === workspaceId && this.count > 0) return;
    const generation = ++this.#loadGeneration;
    this.workspaceId = workspaceId;
    this.loading = true;
    try {
      const response = await api.items.getBacklog(workspaceId);
      if (generation !== this.#loadGeneration || this.workspaceId !== workspaceId) return;
      const count = response?.pagination?.total_items ?? response?.data?.length ?? 0;
      this.count = count;
    } catch (error) {
      if (generation !== this.#loadGeneration || this.workspaceId !== workspaceId) return;
      console.error('Failed to load backlog count:', error);
      this.count = 0;
    } finally {
      if (generation === this.#loadGeneration && this.workspaceId === workspaceId) {
        this.loading = false;
      }
    }
  }

  /**
   * Set the backlog count directly.
   * Called when components load their own backlog data.
   */
  setCount(workspaceId, count) {
    this.#loadGeneration += 1;
    this.workspaceId = workspaceId;
    this.count = count;
    this.loading = false;
  }

  /**
   * Increment count when item added to backlog.
   */
  increment() {
    this.#loadGeneration += 1;
    this.count++;
    this.loading = false;
  }

  /**
   * Decrement count when item removed from backlog.
   */
  decrement() {
    this.#loadGeneration += 1;
    this.count = Math.max(0, this.count - 1);
    this.loading = false;
  }

  /**
   * Reset store state.
   */
  reset() {
    this.#loadGeneration += 1;
    this.workspaceId = null;
    this.count = 0;
    this.loading = false;
  }
}

export const backlogStore = new BacklogStore();
