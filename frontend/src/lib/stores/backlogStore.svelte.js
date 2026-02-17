import { api } from '../api.js';

/**
 * Store for managing backlog count across workspace views.
 * Uses Svelte 5 class-based reactive state.
 */
class BacklogStore {
  workspaceId = $state(null);
  count = $state(0);
  loading = $state(false);

  /**
   * Load backlog count for a workspace.
   * Skips fetch if already loaded for this workspace.
   */
  async load(workspaceId) {
    if (this.workspaceId === workspaceId && this.count > 0) return;
    this.workspaceId = workspaceId;
    this.loading = true;
    try {
      const response = await api.items.getBacklog(workspaceId);
      this.count = response?.pagination?.total ?? response?.items?.length ?? (Array.isArray(response) ? response.length : 0);
    } catch (error) {
      console.error('Failed to load backlog count:', error);
      this.count = 0;
    } finally {
      this.loading = false;
    }
  }

  /**
   * Set the backlog count directly.
   * Called when components load their own backlog data.
   */
  setCount(workspaceId, count) {
    this.workspaceId = workspaceId;
    this.count = count;
  }

  /**
   * Increment count when item added to backlog.
   */
  increment() {
    this.count++;
  }

  /**
   * Decrement count when item removed from backlog.
   */
  decrement() {
    this.count = Math.max(0, this.count - 1);
  }

  /**
   * Reset store state.
   */
  reset() {
    this.workspaceId = null;
    this.count = 0;
    this.loading = false;
  }
}

export const backlogStore = new BacklogStore();
