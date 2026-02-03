// Toast store for managing multiple stacked toasts
// Uses Svelte 5 runes for reactivity

import { t } from './i18n.svelte.js';

let toastId = 0;

// Reactive state for toasts array
let toastsState = $state([]);

export const toasts = {
  get value() {
    return toastsState;
  },
  subscribe(fn) {
    // Simple subscription for compatibility
    $effect.root(() => {
      $effect(() => {
        fn(toastsState);
      });
    });
    fn(toastsState);
    return () => {};
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
  toastsState = toastsState.filter((toast) => toast.id !== id);
}

/**
 * Remove all toasts
 */
export function clearToasts() {
  toastsState = [];
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
