import { writable } from 'svelte/store';

// Global confirmation dialog state
export const confirmDialog = writable({
  show: false,
  title: '',
  message: '',
  confirmText: 'Confirm',
  cancelText: 'Cancel',
  variant: 'danger',
  icon: null,
  onConfirm: null,
  onCancel: null
});

// Helper function to show confirmation dialog
export function confirm(options = {}) {
  return new Promise((resolve) => {
    confirmDialog.set({
      show: true,
      title: options.title || 'Confirm Action',
      message: options.message || 'Are you sure you want to proceed?',
      confirmText: options.confirmText || 'Confirm',
      cancelText: options.cancelText || 'Cancel',
      variant: options.variant || 'danger',
      icon: options.icon || null,
      onConfirm: () => {
        confirmDialog.update(state => ({ ...state, show: false }));
        resolve(true);
      },
      onCancel: () => {
        confirmDialog.update(state => ({ ...state, show: false }));
        resolve(false);
      }
    });
  });
}