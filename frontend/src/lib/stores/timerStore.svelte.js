/**
 * Timer Store - Svelte 5 Runes Pattern
 * Manages active timer state, duration tracking, and server sync.
 */
import { api } from '../api.js';

class TimerStore {
  // === State ===
  activeTimer = $state(null);
  syncing = $state(false);
  error = $state(null);
  duration = $state(0);

  // === Private Interval Refs ===
  #timerInterval = null;
  #syncInterval = null;

  // === Derived Values ===

  /**
   * Formatted timer duration (HH:MM:SS)
   */
  get durationFormatted() {
    const hours = Math.floor(this.duration / 3600);
    const minutes = Math.floor((this.duration % 3600) / 60);
    const seconds = this.duration % 60;
    return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
  }

  /**
   * Alias for backward compatibility
   */
  get formattedDuration() {
    return this.durationFormatted;
  }

  /**
   * Whether a new timer can be started
   */
  get canStart() {
    return !this.activeTimer && !this.syncing;
  }

  /**
   * Whether the active timer can be stopped
   */
  get canStop() {
    return !!this.activeTimer && !this.syncing;
  }

  /**
   * Check if there's an active timer running
   */
  get hasActive() {
    return this.activeTimer !== null;
  }

  // === Timer Interval Management ===

  /**
   * Start the timer interval to update duration every second
   */
  #startTimerInterval(startTimeUTC) {
    if (this.#timerInterval) return; // Already running

    const updateDuration = () => {
      const now = Math.floor(Date.now() / 1000);
      this.duration = Math.max(0, now - startTimeUTC);
    };

    // Update immediately
    updateDuration();

    // Then update every second
    this.#timerInterval = setInterval(updateDuration, 1000);
  }

  /**
   * Stop the timer interval
   */
  #stopTimerInterval() {
    if (this.#timerInterval) {
      clearInterval(this.#timerInterval);
      this.#timerInterval = null;
    }
    this.duration = 0;
  }

  // === Sync Interval Management ===

  /**
   * Start the sync interval (every 30 seconds)
   */
  #startSyncInterval() {
    if (this.#syncInterval) return; // Already running

    this.#syncInterval = setInterval(() => {
      this.sync();
    }, 30000);
  }

  /**
   * Stop the sync interval
   */
  #stopSyncInterval() {
    if (this.#syncInterval) {
      clearInterval(this.#syncInterval);
      this.#syncInterval = null;
    }
  }

  // === Timer Actions ===

  /**
   * Start a new timer
   * @param {Object} timerData - Timer data with workspace_id, project_id, description, and optional item_id
   * @returns {Promise<Object>} Started timer object
   */
  async start(timerData) {
    // Guard: Check if we can start
    if (!this.canStart) {
      console.warn('Cannot start timer:', { active: !!this.activeTimer, syncing: this.syncing });
      return null;
    }

    try {
      this.syncing = true;
      this.error = null;

      const timer = await api.timer.start(timerData);
      this.activeTimer = timer;

      // Start timer interval for live updates
      this.#startTimerInterval(timer.start_time_utc);

      // Start sync interval
      this.#startSyncInterval();

      return timer;
    } catch (err) {
      console.error('Failed to start timer:', err);
      this.error = err.message || 'Failed to start timer';
      throw err;
    } finally {
      this.syncing = false;
    }
  }

  /**
   * Stop the active timer
   * @returns {Promise<Object>} Stop result with worklog data
   */
  async stop() {
    // Guard: Check if we can stop
    if (!this.canStop) {
      console.warn('Cannot stop timer:', { active: !!this.activeTimer, syncing: this.syncing });
      return null;
    }

    try {
      this.syncing = true;
      this.error = null;

      const result = await api.timer.stop(this.activeTimer.id);

      // Clear active timer
      this.activeTimer = null;

      // Stop timer interval
      this.#stopTimerInterval();

      // Stop sync interval
      this.#stopSyncInterval();

      return result;
    } catch (err) {
      console.error('Failed to stop timer:', err);
      this.error = err.message || 'Failed to stop timer';
      throw err;
    } finally {
      this.syncing = false;
    }
  }

  /**
   * Sync timer state with server
   * This fetches the current active timer from the server
   */
  async sync() {
    try {
      this.error = null;

      const timer = await api.timer.getActive();

      if (timer) {
        this.activeTimer = timer;
        // Start timer interval for live updates
        this.#startTimerInterval(timer.start_time_utc);
        // Ensure sync interval is running
        if (!this.#syncInterval) {
          this.#startSyncInterval();
        }
      } else {
        this.activeTimer = null;
        this.#stopTimerInterval();
        this.#stopSyncInterval();
      }
    } catch (err) {
      console.error('Failed to sync timer:', err);
      this.error = err.message || 'Failed to sync timer';
    }
  }

  /**
   * Initialize timer store
   * This should be called when the app starts to sync with any existing active timer
   */
  async initialize() {
    await this.sync();
  }

  /**
   * Get the current active timer
   * @returns {Object|null}
   */
  getCurrent() {
    return this.activeTimer;
  }

  /**
   * Cleanup function - stops all intervals
   */
  cleanup() {
    this.#stopTimerInterval();
    this.#stopSyncInterval();
  }

  /**
   * Reset store to initial state
   */
  reset() {
    this.cleanup();
    this.activeTimer = null;
    this.syncing = false;
    this.error = null;
    this.duration = 0;
  }
}

// Create singleton instance
export const timerStore = new TimerStore();

// Backward compatibility exports - these access the store's state/methods
// Components should migrate to using timerStore directly
export const startTimer = (data) => timerStore.start(data);
export const stopTimer = () => timerStore.stop();
export const syncTimer = () => timerStore.sync();
export const initializeTimer = () => timerStore.initialize();
export const hasActiveTimer = () => timerStore.hasActive;
export const getCurrentTimer = () => timerStore.getCurrent();
export const cleanup = () => timerStore.cleanup();

// Aliases for backward compatibility
export const start = startTimer;
export const stop = stopTimer;
export const sync = syncTimer;
export const initialize = initializeTimer;

// Clean up intervals when the page is unloaded
if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', () => {
    timerStore.cleanup();
  });
}
