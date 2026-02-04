import { IsIdle } from 'runed';

/**
 * Thin wrapper around runed's IsIdle sensor.
 * Tracks user activity for adaptive polling intervals.
 * Initialized from MainApp.svelte.
 */
class ActivityStore {
  /** @type {IsIdle|null} */
  _idle = null;

  init() {
    if (this._idle) return; // Already initialized
    this._idle = new IsIdle({
      timeout: 120_000, // 2 min inactivity = idle
      detectVisibilityChanges: true, // hidden tab = idle
    });
  }

  get isIdle() {
    return this._idle?.current ?? false;
  }

  get lastActive() {
    return this._idle?.lastActive ?? Date.now();
  }
}

export const activityStore = new ActivityStore();
