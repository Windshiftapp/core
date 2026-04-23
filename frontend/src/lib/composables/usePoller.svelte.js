import { activityStore } from '../stores/activityStore.svelte.js';

const DEFAULT_ACTIVE = 30_000;
const DEFAULT_IDLE = 5 * 60_000;

/**
 * Adaptive polling composable. Calls fetchFn on an interval that shortens
 * when the user is active and stretches when idle / tab hidden.
 *
 * @param {() => Promise<void>|void} fetchFn
 * @param {{ active?: number, idle?: number }} [opts]
 */
export function usePoller(fetchFn, opts = {}) {
  const activeInterval = opts.active ?? DEFAULT_ACTIVE;
  const idleInterval = opts.idle ?? DEFAULT_IDLE;

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
      console.warn('usePoller: poll failed', err);
    } finally {
      isPolling = false;
    }
  }

  function _stopTimer() {
    if (_timer) {
      clearInterval(_timer);
      _timer = null;
    }
  }

  function _startTimer(interval) {
    _stopTimer();
    _timer = setInterval(poll, interval);
  }

  $effect(() => {
    const idle = activityStore.isIdle;
    _startTimer(idle ? idleInterval : activeInterval);
    return _stopTimer;
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
