import { usePoller } from './usePoller.svelte.js';

const ACTIVE_INTERVAL = 30_000;
const IDLE_INTERVAL = 5 * 60_000;

/**
 * Adaptive poller for work item fetching. Thin wrapper around usePoller
 * with the cadence board views have used since the pattern was introduced.
 *
 * @param {() => Promise<void>|void} fetchFn
 * @returns {{ poll: Function, isPolling: boolean, lastPollTime: number|null }}
 */
export function useWorkItemPoller(fetchFn) {
  return usePoller(fetchFn, { active: ACTIVE_INTERVAL, idle: IDLE_INTERVAL });
}
