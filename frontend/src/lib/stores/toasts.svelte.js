// Toast store for managing multiple stacked toasts
// Uses Svelte 5 runes for reactivity

import { t, translateError } from './i18n.svelte.js';

let toastId = 0;

// Reactive state for toasts array
let toastsState = $state([]);

export const toasts = {
  get value() {
    return toastsState;
  },
  subscribe(fn) {
    // Simple subscription for compatibility
    const dispose = $effect.root(() => {
      $effect(() => {
        fn(toastsState);
      });
    });
    fn(toastsState);
    return dispose;
  },
};

/**
 * Add a new toast to the stack
 * @param {Object} options - Toast options
 * @param {string} options.message - Toast message
 * @param {string} [options.title] - Optional title
 * @param {'default'|'error'|'success'|'warning'|'info'} [options.variant='default'] - Toast variant
 * @param {number} [options.duration=5000] - Auto-hide duration (0 = no auto-hide)
 * @param {boolean} [options.showCloseButton=true] - Show close button
 * @param {boolean} [options.clickable=false] - Whether the toast is clickable
 * @param {Function} [options.onClick] - Callback when toast is clicked (only if clickable)
 * @param {Function} [options.onDismiss] - Callback when toast is removed for any reason
 * @param {string} [options.actionLabel] - Optional inline action label
 * @param {string} [options.keyboardHint] - Optional keyboard shortcut hint for the action
 * @returns {number} Toast ID
 */
export function addToast(options) {
  const id = toastId++;
  const toast = {
    id,
    message: options.message || '',
    title: options.title || '',
    variant: options.variant || 'default',
    duration: options.duration ?? 5000,
    showCloseButton: options.showCloseButton ?? true,
    clickable: options.clickable ?? false,
    onClick: options.onClick || null,
    onDismiss: options.onDismiss || null,
    actionLabel: options.actionLabel || '',
    keyboardHint: options.keyboardHint || '',
    createdAt: Date.now(),
  };

  // Add to beginning (newest first)
  toastsState = [toast, ...toastsState];

  return id;
}

/**
 * Remove a toast by ID
 * @param {number} id - Toast ID to remove
 */
export function removeToast(id) {
  const toast = toastsState.find((entry) => entry.id === id);
  toastsState = toastsState.filter((entry) => entry.id !== id);
  if (toast?.onDismiss) {
    try {
      toast.onDismiss();
    } catch (err) {
      console.error('toast onDismiss callback threw:', err);
    }
  }
}

/**
 * Cancellation is control flow, not a failure the user should see. The API
 * layer reports a client-canceled request as an AbortError, and the server
 * reports it as REQUEST_CANCELED; both mean the work was intentionally
 * abandoned (navigation, closed modal, superseded request).
 *
 * @param {unknown} error
 * @returns {boolean}
 */
function isCancellation(error) {
  if (!error || typeof error !== 'object') return false;
  if (/** @type {any} */ (error).name === 'AbortError') return true;
  const code = String(
    /** @type {any} */ (error).code ?? /** @type {any} */ (error).errorCode ?? ''
  ).toLowerCase();
  return code === 'request_canceled';
}

/**
 * Surface a caught failure to the user while keeping the console diagnostic.
 * Prefer this over a bare `console.error` so a failure cannot disappear into
 * the console. Pass `silent: true` only for work the user did not initiate and
 * that has another visible outcome (background sync, optional persistence).
 *
 * @param {unknown} error - The caught error, or a message string.
 * @param {Object} [options]
 * @param {string} [options.title] - Toast title; defaults to the error title.
 * @param {string} [options.message] - Override the translated error message.
 * @param {string} [options.context] - Label for the console diagnostic.
 * @param {'error'|'warning'} [options.level] - Toast severity. Default `error`.
 * @param {boolean} [options.silent] - Log without a toast.
 */
export function surfaceError(error, options = {}) {
  const { title, message, context, level = 'error', silent = false } = options;
  if (isCancellation(error)) return;
  console.error(context ? `[${context}]` : '[windshift]', error);
  if (silent) return;
  const text = message ?? (typeof error === 'string' ? error : translateError(error));
  if (level === 'warning') warningToast(text, title);
  else errorToast(text, title);
}

/**
 * Convenience function for error toast
 */
export function errorToast(message, title) {
  return addToast({ message, title: title ?? t('toast.error'), variant: 'error' });
}

/**
 * Convenience function for success toast
 */
export function successToast(message, title) {
  return addToast({ message, title: title ?? t('toast.success'), variant: 'success' });
}

/**
 * Convenience function for warning toast
 */
export function warningToast(message, title) {
  return addToast({ message, title: title ?? t('toast.warning'), variant: 'warning' });
}

/**
 * Convenience function for info toast
 */
export function infoToast(message, title) {
  return addToast({ message, title: title ?? t('toast.info'), variant: 'info' });
}
