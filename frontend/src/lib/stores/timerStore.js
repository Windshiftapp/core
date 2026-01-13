import { writable, derived, get } from 'svelte/store';
import { api } from '../api.js';

// Timer state - holds the current active timer (if any)
export const activeTimer = writable(null);

// Timer sync status
export const timerSyncing = writable(false);

// Timer error state
export const timerError = writable(null);

// Timer duration store that updates every second
export const timerDuration = writable(0);

// Auto-increment timer duration
let timerInterval = null;

// Derived store for formatted timer duration (HH:MM:SS)
export const timerDurationFormatted = derived(
  [timerDuration],
  ([duration]) => {
    const hours = Math.floor(duration / 3600);
    const minutes = Math.floor((duration % 3600) / 60);
    const seconds = duration % 60;
    return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
  }
);

let syncInterval = null;

/**
 * Start the timer interval to update duration every second
 */
function startTimerInterval(startTimeUTC) {
  if (timerInterval) return; // Already running
  
  const updateDuration = () => {
    const now = Math.floor(Date.now() / 1000);
    const duration = Math.max(0, now - startTimeUTC);
    timerDuration.set(duration);
  };
  
  // Update immediately
  updateDuration();
  
  // Then update every second
  timerInterval = setInterval(updateDuration, 1000);
}

/**
 * Stop the timer interval
 */
function stopTimerInterval() {
  if (timerInterval) {
    clearInterval(timerInterval);
    timerInterval = null;
  }
  timerDuration.set(0);
}

/**
 * Start a new timer
 * @param {Object} timerData - Timer data with workspace_id, project_id, description, and optional item_id
 * @returns {Promise<Object>} Started timer object
 */
export async function startTimer(timerData) {
  try {
    timerSyncing.set(true);
    timerError.set(null);
    
    const timer = await api.timer.start(timerData);
    activeTimer.set(timer);
    
    // Start timer interval for live updates
    startTimerInterval(timer.start_time_utc);
    
    // Start sync interval (every 30 seconds)
    startSyncInterval();
    
    return timer;
  } catch (error) {
    console.error('Failed to start timer:', error);
    timerError.set(error.message || 'Failed to start timer');
    throw error;
  } finally {
    timerSyncing.set(false);
  }
}

/**
 * Stop the active timer
 * @returns {Promise<Object>} Stop result with worklog data
 */
export async function stopTimer() {
  try {
    const timer = get(activeTimer);
    if (!timer) {
      throw new Error('No active timer to stop');
    }
    
    timerSyncing.set(true);
    timerError.set(null);
    
    const result = await api.timer.stop(timer.id);
    
    // Clear active timer
    activeTimer.set(null);
    
    // Stop timer interval
    stopTimerInterval();
    
    // Stop sync interval
    stopSyncInterval();
    
    return result;
  } catch (error) {
    console.error('Failed to stop timer:', error);
    timerError.set(error.message || 'Failed to stop timer');
    throw error;
  } finally {
    timerSyncing.set(false);
  }
}

/**
 * Sync timer state with server
 * This fetches the current active timer from the server
 */
export async function syncTimer() {
  try {
    timerError.set(null);
    
    const timer = await api.timer.getActive();
    
    if (timer) {
      activeTimer.set(timer);
      // Start timer interval for live updates
      startTimerInterval(timer.start_time_utc);
      // Ensure sync interval is running
      if (!syncInterval) {
        startSyncInterval();
      }
    } else {
      activeTimer.set(null);
      stopTimerInterval();
      stopSyncInterval();
    }
  } catch (error) {
    console.error('Failed to sync timer:', error);
    timerError.set(error.message || 'Failed to sync timer');
  }
}

/**
 * Initialize timer store
 * This should be called when the app starts to sync with any existing active timer
 */
export async function initializeTimer() {
  await syncTimer();
}

/**
 * Start the sync interval
 */
function startSyncInterval() {
  if (syncInterval) return; // Already running
  
  syncInterval = setInterval(() => {
    syncTimer();
  }, 30000); // 30 seconds
}

/**
 * Stop the sync interval
 */
function stopSyncInterval() {
  if (syncInterval) {
    clearInterval(syncInterval);
    syncInterval = null;
  }
}

/**
 * Check if there's an active timer running
 * @returns {boolean}
 */
export function hasActiveTimer() {
  return get(activeTimer) !== null;
}

/**
 * Get the current active timer
 * @returns {Object|null}
 */
export function getCurrentTimer() {
  return get(activeTimer);
}

// Clean up intervals when the page is unloaded
if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', () => {
    stopTimerInterval();
    stopSyncInterval();
  });
}