import { useDebounce } from 'runed';
import { surfaceError } from '../stores/toasts.svelte.js';

/**
 * The bare string runed rejects a pending debounce with when it is cancelled.
 * It is control flow, not a failure. Exported so every boundary that can
 * observe a runed debounce promise recognises it: `useDebouncedAction` consumes
 * it for the call sites it wraps, and runed's own primitives can leak it past
 * the wrapper (see `installGlobalErrorHandlers`).
 */
export const RUNED_CANCEL_SENTINEL = 'Cancelled';

/**
 * Debounce an action while keeping its cancellation internal.
 *
 * runed's `useDebounce.cancel()` rejects the pending promise with
 * `RUNED_CANCEL_SENTINEL`. Callers invoke a debounced action and ignore its
 * promise, so that rejection otherwise reaches the window's
 * `unhandledrejection` handler and surfaces as a "Cancelled" toast. Wrap the
 * library so the sentinel is consumed here and never escapes; a genuine
 * rejection is surfaced once instead of disappearing.
 *
 * @template {(...args: any[]) => any} T
 * @param {T} callback
 * @param {number | (() => number)} [wait]
 */
export function useDebouncedAction(callback, wait) {
  const debounced = useDebounce(callback, wait);

  function run(...args) {
    // Return the caught promise so a caller that awaits the debounced action
    // resolves on cancellation instead of seeing the library sentinel.
    return debounced(...args).catch((error) => {
      // The library's cancel sentinel is expected control flow, not a failure.
      if (error === RUNED_CANCEL_SENTINEL) return;
      surfaceError(error, { context: 'useDebouncedAction' });
    });
  }

  run.cancel = () => debounced.cancel();
  run.runScheduledNow = () => debounced.runScheduledNow();
  return run;
}
