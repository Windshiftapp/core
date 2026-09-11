import { api } from '../api.js';

class TimerStore {
  // === State ===
  activeTimer = $state(null);
  syncing = $state(false);
  error = $state(null);
  duration = $state(0);

  // === Private Interval Refs ===
  #timerInterval = null;
  #timerStartTimeUTC = null;
  #syncInterval = null;
  #stateVersion = 0;
  #initializePromise = null;

  // === Derived Values ===

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

  get canStart() {
    return !this.activeTimer && !this.syncing;
  }

  get canStop() {
    return !!this.activeTimer && !this.syncing;
  }

  get hasActive() {
    return this.activeTimer !== null;
  }

  // === Timer Interval Management ===

  #startTimerInterval(startTimeUTC) {
    const startTime = Number(startTimeUTC);
    if (!Number.isFinite(startTime)) {
      this.duration = 0;
      return;
    }

    if (this.#timerInterval && this.#timerStartTimeUTC === startTime) return; // Already running for this timer

    if (this.#timerInterval) {
      clearInterval(this.#timerInterval);
      this.#timerInterval = null;
    }
    this.#timerStartTimeUTC = startTime;

    const updateDuration = () => {
      const now = Math.floor(Date.now() / 1000);
      this.duration = Math.max(0, now - startTime);
    };

    // Update immediately
    updateDuration();

    // Then update every second
    this.#timerInterval = setInterval(updateDuration, 1000);
  }

  #stopTimerInterval() {
    if (this.#timerInterval) {
      clearInterval(this.#timerInterval);
      this.#timerInterval = null;
    }
    this.#timerStartTimeUTC = null;
    this.duration = 0;
  }

  // === Sync Interval Management ===

  #startSyncInterval() {
    if (this.#syncInterval) return; // Already running

    this.#syncInterval = setInterval(() => {
      this.sync();
    }, 30000);
  }

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
      this.#stateVersion += 1;

      const timer = await api.timer.start(timerData);
      this.#stateVersion += 1;
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
      this.#stateVersion += 1;

      const result = await api.timer.stop();

      // Clear active timer
      this.#stateVersion += 1;
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

  async sync() {
    const syncVersion = this.#stateVersion;

    try {
      this.error = null;

      const timer = await api.timer.getActive();
      if (syncVersion !== this.#stateVersion || this.syncing) {
        return;
      }

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
      if (err?.name === 'AbortError') return;
      console.error('Failed to sync timer:', err);
      this.error = err.message || 'Failed to sync timer';
    }
  }

  async initialize() {
    if (this.#initializePromise) return this.#initializePromise;
    const request = this.sync().finally(() => {
      if (this.#initializePromise === request) this.#initializePromise = null;
    });
    this.#initializePromise = request;
    return request;
  }

  /**
   * Get the current active timer
   * @returns {Object|null}
   */
  getCurrent() {
    return this.activeTimer;
  }

  cleanup() {
    this.#stopTimerInterval();
    this.#stopSyncInterval();
  }

  reset() {
    this.cleanup();
    this.#initializePromise = null;
    this.#stateVersion += 1;
    this.activeTimer = null;
    this.syncing = false;
    this.error = null;
    this.duration = 0;
  }
}

// Create singleton instance
export const timerStore = new TimerStore();

// Clean up intervals when the page is unloaded
if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', () => {
    timerStore.cleanup();
  });
}
