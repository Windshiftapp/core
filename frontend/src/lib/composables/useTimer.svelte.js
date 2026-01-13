import { api } from '../api.js';
import { writable, derived, get } from 'svelte/store';

// Timer stores - shared across all components
export const activeTimer = writable(null); // Current active timer object
export const timerSyncing = writable(false); // Whether timer operations are in progress
export const timerError = writable(null); // Last error message
export const timerDuration = writable(0); // Timer duration in seconds

// Intervals for timer updates (singleton)
let durationInterval = null;
let syncInterval = null;

// Helper: Start duration interval
function startDurationInterval(startTimeUTC) {
  if (durationInterval) return; // Already running

  const updateDuration = () => {
    const now = Math.floor(Date.now() / 1000);
    const elapsed = Math.max(0, now - startTimeUTC);
    timerDuration.set(elapsed);
  };

  // Update immediately
  updateDuration();

  // Then update every second
  durationInterval = setInterval(updateDuration, 1000);
}

// Helper: Stop duration interval
function stopDurationInterval() {
  if (durationInterval) {
    clearInterval(durationInterval);
    durationInterval = null;
  }
  timerDuration.set(0);
}

// Helper: Start sync interval (sync with server every 30 seconds)
function startSyncInterval() {
  if (syncInterval) return; // Already running

  syncInterval = setInterval(async () => {
    await sync();
  }, 30000); // 30 seconds
}

// Helper: Stop sync interval
function stopSyncInterval() {
  if (syncInterval) {
    clearInterval(syncInterval);
    syncInterval = null;
  }
}

/**
 * Start a new timer
 * @param {Object} data Timer data (workspace_id, item_id, project_id, description)
 * @returns {Promise<Object>} Started timer object
 */
async function start(data) {
  // Guard: Check if we can start
  const active = get(activeTimer);
  const syncing = get(timerSyncing);
  const canStart = !active && !syncing;

  if (!canStart) {
    console.warn('Cannot start timer:', { active: !!active, syncing });
    return null;
  }

  try {
    timerSyncing.set(true);
    timerError.set(null);

    const timer = await api.timer.start(data);
    activeTimer.set(timer);

    // Start intervals for live updates
    startDurationInterval(timer.start_time_utc);
    startSyncInterval();

    return timer;
  } catch (err) {
    console.error('Failed to start timer:', err);
    timerError.set(err.message || 'Failed to start timer');
    // Rethrow to allow component-level error handling
    throw err;
  } finally {
    timerSyncing.set(false);
  }
}

/**
 * Stop the active timer
 * @returns {Promise<Object>} Stop result with worklog data
 */
async function stop() {
  // Guard: Check if we can stop
  const active = get(activeTimer);
  const syncing = get(timerSyncing);
  const canStop = active && !syncing;

  if (!canStop) {
    console.warn('Cannot stop timer:', { active: !!active, syncing });
    return null;
  }

  if (!active) {
    throw new Error('No active timer to stop');
  }

  try {
    timerSyncing.set(true);
    timerError.set(null);

    const result = await api.timer.stop(active.id);

    // Clear timer state
    activeTimer.set(null);

    // Stop intervals
    stopDurationInterval();
    stopSyncInterval();

    return result;
  } catch (err) {
    console.error('Failed to stop timer:', err);
    timerError.set(err.message || 'Failed to stop timer');
    throw err;
  } finally {
    timerSyncing.set(false);
  }
}

/**
 * Sync timer state with server
 * Fetches the current active timer from the server
 */
async function sync() {
  try {
    timerError.set(null);

    const timer = await api.timer.getActive();

    if (timer) {
      activeTimer.set(timer);
      // Restart duration interval with server time
      startDurationInterval(timer.start_time_utc);
      // Ensure sync interval is running
      if (!syncInterval) {
        startSyncInterval();
      }
    } else {
      activeTimer.set(null);
      stopDurationInterval();
      stopSyncInterval();
    }
  } catch (err) {
    console.error('Failed to sync timer:', err);
    timerError.set(err.message || 'Failed to sync timer');
  }
}

/**
 * Initialize timer on mount
 * Syncs with server to get current timer state
 */
async function initialize() {
  await sync();
}

/**
 * Cleanup function to call on unmount
 */
function cleanup() {
  // Note: We don't stop intervals here because timer is singleton
  // Intervals should continue running even if a component unmounts
}

// Derived stores for guard conditions
export const canStartTimer = derived(
  [activeTimer, timerSyncing],
  ([$active, $syncing]) => !$active && !$syncing
);

export const canStopTimer = derived(
  [activeTimer, timerSyncing],
  ([$active, $syncing]) => !!$active && !$syncing
);

// Derived formatted duration (HH:MM:SS)
export const formattedDuration = derived(
  timerDuration,
  ($duration) => {
    const hours = Math.floor($duration / 3600);
    const minutes = Math.floor(($duration % 3600) / 60);
    const seconds = $duration % 60;
    return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
  }
);

/**
 * Modern Svelte 5 timer composable using stores
 * Provides centralized timer state management with proper reactivity
 *
 * @returns {Object} Timer state and methods
 */
export function useTimer() {
  // Public API
  return {
    // Stores (reactive - components can subscribe)
    activeTimer,
    timerSyncing,
    timerError,
    timerDuration,

    // Derived stores (reactive)
    canStartTimer,
    canStopTimer,
    formattedDuration,

    // Methods
    start,
    stop,
    sync,
    initialize,
    cleanup,

    // Helper to check if timer is running (for non-reactive contexts)
    hasActiveTimer: () => get(activeTimer) !== null,

    // Helper to get current timer (for non-reactive contexts)
    getCurrentTimer: () => get(activeTimer),

    // Backwards compatibility getters that access store values
    get active() { return get(activeTimer); },
    get syncing() { return get(timerSyncing); },
    get error() { return get(timerError); },
    get duration() { return get(timerDuration); },
    get canStart() { return get(canStartTimer); },
    get canStop() { return get(canStopTimer); },
    get formatted() { return get(formattedDuration); }
  };
}

// Clean up intervals when the page is unloaded
if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', () => {
    stopDurationInterval();
    stopSyncInterval();
  });
}