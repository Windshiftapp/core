import { surfaceError } from '../stores/toasts.svelte.js';
import {
  canRunBackgroundSync,
  isExpectedBackgroundSyncError,
  onBackgroundSyncAvailable,
} from './backgroundSync.js';

/**
 * Coalesce view invalidations and retain one trailing refresh during slow reads.
 * Callers keep ownership of polling cadence.
 * @param {() => unknown} refresh
 * @param {{ delay?: number }} [options]
 */
export function createBackgroundRefresh(refresh, { delay = 250 } = {}) {
  let timer = null;
  let pending = false;
  let running = false;
  let disposed = false;

  const queue = (wait) => {
    if (disposed) return;
    pending = true;
    if (timer !== null || running || !canRunBackgroundSync()) return;
    timer = setTimeout(async () => {
      timer = null;
      if (disposed || !canRunBackgroundSync()) return;
      pending = false;
      running = true;
      try {
        await refresh();
      } catch (error) {
        if (!disposed && !isExpectedBackgroundSyncError(error)) {
          surfaceError(error, { silent: true, context: 'backgroundRefresh' });
        }
      } finally {
        running = false;
        if (pending) queue(delay);
      }
    }, wait);
  };
  const schedule = () => queue(delay);
  const stopRecovery = onBackgroundSyncAvailable(() => {
    if (timer !== null) clearTimeout(timer);
    timer = null;
    queue(0);
  });
  return {
    schedule,
    dispose() {
      disposed = true;
      pending = false;
      if (timer !== null) clearTimeout(timer);
      stopRecovery();
    },
  };
}
