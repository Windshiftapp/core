import { activityStore } from '../stores/activityStore.svelte.js';

const ACTIVE_INTERVAL = 30_000; // 30 seconds when user is active
const IDLE_INTERVAL = 5 * 60_000; // 5 minutes when user is idle

/**
 * Composable that manages a polling interval for work item fetching,
 * adapting to user activity state.
 *
 * @param {Function} fetchFn - Async callback to fetch work items
 * @returns {{ poll: Function, isPolling: boolean, lastPollTime: number|null }}
 */
export function useWorkItemPoller(fetchFn) {
  let isPolling = $state(false);
  let lastPollTime = $state(null);
  let _timer = null;

  async function poll() {
    if (isPolling) return;
    isPolling = true;
    try {
      await fetchFn();
      lastPollTime = Date.now();
    } catch (err) {
      console.warn('WorkItemPoller: poll failed', err);
    } finally {
      isPolling = false;
    }
  }

  function _startTimer(interval) {
    _stopTimer();
    _timer = setInterval(() => {
      poll();
    }, interval);
  }

  function _stopTimer() {
    if (_timer) {
      clearInterval(_timer);
      _timer = null;
    }
  }

  // Reactive effect: switch interval based on activity
  $effect(() => {
    const idle = activityStore.isIdle;
    const interval = idle ? IDLE_INTERVAL : ACTIVE_INTERVAL;
    _startTimer(interval);

    return () => {
      _stopTimer();
    };
  });

  return {
    poll,
    get isPolling() {
      return isPolling;
    },
    get lastPollTime() {
      return lastPollTime;
    },
  };
}
