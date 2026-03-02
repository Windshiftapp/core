/**
 * Plugin Bridge - Host side message handler for iframe plugins
 *
 * Handles communication between iframe plugins and host application via postMessage.
 * Provides bridges for: modals, toasts, theme, resize
 */

import { writable } from 'svelte/store';
import { addToast } from '../stores/toasts.svelte.js';

// Store for modal requests from plugins
export const pluginModalRequests = writable([]);

// Message type constants
export const MESSAGE_TYPES = {
  // Plugin → Host
  READY: 'plugin:ready',
  RESIZE: 'plugin:resize',
  SHOW_MODAL: 'plugin:showModal',
  SHOW_CONFIRM: 'plugin:showConfirm',
  SHOW_TOAST: 'plugin:showToast',
  GET_THEME: 'plugin:getTheme',
  SHOW_USER_PICKER: 'plugin:showUserPicker',

  // Host → Plugin
  THEME_UPDATE: 'host:themeUpdate',
  MODAL_RESULT: 'host:modalResult',
  USER_PICKER_RESULT: 'host:userPickerResult',
};

/**
 * Initialize plugin bridge for an iframe
 * @param {HTMLIFrameElement} iframe - The iframe element
 * @param {Object} options - Configuration options
 * @returns {Object} Bridge API
 */
export function createPluginBridge(iframe, options = {}) {
  const {
    onReady = () => {},
    onResize = () => {},
    onShowUserPicker = () => {},
    pluginName = 'unknown',
  } = options;

  // Track active modals by ID
  const activeModals = new Map();

  // Expected origin for plugin messages (same origin since plugins are served from same server)
  const expectedOrigin = window.location.origin;

  /**
   * Send message to plugin iframe
   */
  function sendToPlugin(message) {
    if (iframe?.contentWindow) {
      iframe.contentWindow.postMessage(message, expectedOrigin);
    }
  }

  /**
   * Handle messages from plugin
   */
  function handleMessage(event) {
    // Validate message origin to prevent cross-origin attacks
    if (event.origin !== expectedOrigin) {
      return;
    }
    const message = event.data;

    if (!message || !message.type) return;

    switch (message.type) {
      case MESSAGE_TYPES.READY:
        onReady();
        // Send initial theme
        sendThemeUpdate();
        break;

      case MESSAGE_TYPES.RESIZE:
        if (typeof message.height === 'number') {
          onResize(message.height);
        }
        break;

      case MESSAGE_TYPES.SHOW_MODAL:
        handleModalRequest(message);
        break;

      case MESSAGE_TYPES.SHOW_CONFIRM:
        handleConfirmRequest(message);
        break;

      case MESSAGE_TYPES.SHOW_TOAST:
        handleToastRequest(message);
        break;

      case MESSAGE_TYPES.GET_THEME:
        sendThemeUpdate();
        break;

      case MESSAGE_TYPES.SHOW_USER_PICKER:
        onShowUserPicker(message.currentUserId);
        break;
    }
  }

  /**
   * Handle modal request from plugin
   */
  function handleModalRequest(message) {
    const modalId = message.id || `modal-${Date.now()}`;

    const modalRequest = {
      id: modalId,
      title: message.title || 'Modal',
      content: message.content || {},
      maxWidth: message.maxWidth || 'max-w-3xl',
      pluginName,
      onClose: (result) => {
        // Send result back to plugin
        sendToPlugin({
          type: MESSAGE_TYPES.MODAL_RESULT,
          id: modalId,
          result,
        });

        // Remove from store
        pluginModalRequests.update((modals) => modals.filter((m) => m.id !== modalId));
        activeModals.delete(modalId);
      },
    };

    activeModals.set(modalId, modalRequest);

    // Add to store so PluginModalContainer can render it
    pluginModalRequests.update((modals) => [...modals, modalRequest]);
  }

  /**
   * Handle confirm dialog request from plugin
   */
  function handleConfirmRequest(message) {
    const modalId = message.id || `confirm-${Date.now()}`;

    const confirmRequest = {
      id: modalId,
      title: message.title || 'Confirm',
      message: message.message || 'Are you sure?',
      confirmText: message.confirmText || 'Confirm',
      cancelText: message.cancelText || 'Cancel',
      variant: message.variant || 'primary', // primary, danger
      pluginName,
      isConfirm: true,
      onClose: (result) => {
        sendToPlugin({
          type: MESSAGE_TYPES.MODAL_RESULT,
          id: modalId,
          result,
        });

        pluginModalRequests.update((modals) => modals.filter((m) => m.id !== modalId));
        activeModals.delete(modalId);
      },
    };

    activeModals.set(modalId, confirmRequest);
    pluginModalRequests.update((modals) => [...modals, confirmRequest]);
  }

  /**
   * Handle toast request from plugin
   */
  function handleToastRequest(message) {
    addToast({
      message: message.message || '',
      variant: message.variant || 'info', // success, error, warning, info
      duration: message.duration || 3000,
    });
  }

  /**
   * Send theme variables to plugin
   */
  function sendThemeUpdate() {
    // Get CSS variables from document
    const computedStyle = getComputedStyle(document.documentElement);
    const themeVariables = {};

    // Extract --ds-* variables
    const styles = document.styleSheets;
    for (const sheet of styles) {
      try {
        for (const rule of sheet.cssRules || []) {
          if (rule.style) {
            for (let i = 0; i < rule.style.length; i++) {
              const prop = rule.style[i];
              if (prop.startsWith('--ds-')) {
                themeVariables[prop] = computedStyle.getPropertyValue(prop).trim();
              }
            }
          }
        }
      } catch (_e) {
        // CORS may prevent access to some stylesheets
      }
    }

    sendToPlugin({
      type: MESSAGE_TYPES.THEME_UPDATE,
      variables: themeVariables,
    });
  }

  // Listen for messages from plugin
  window.addEventListener('message', handleMessage);

  // Return API for host to interact with plugin
  return {
    destroy: () => {
      window.removeEventListener('message', handleMessage);
      // Clean up any active modals from this plugin
      activeModals.forEach((modal) => modal.onClose(null));
      activeModals.clear();
    },
    sendThemeUpdate,
    sendToPlugin,
  };
}
