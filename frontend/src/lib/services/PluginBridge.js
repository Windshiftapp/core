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

  // Editor provider protocol (editor.markdown). Every plugin → host editor
  // message must echo the sessionId the host sent in host:editorInit; the host
  // drops mismatches so two editors on one page cannot cross-talk.
  EDITOR_READY: 'plugin:editorReady',
  EDITOR_CHANGE: 'plugin:editorChange',
  EDITOR_SAVE: 'plugin:editorSave',
  EDITOR_CANCEL: 'plugin:editorCancel',
  EDITOR_ERROR: 'plugin:editorError',
  EDITOR_INIT: 'host:editorInit',
  EDITOR_UPDATE: 'host:editorUpdate',
  EDITOR_SAVE_REQUEST: 'host:editorSaveRequest',
  EDITOR_CANCEL_REQUEST: 'host:editorCancelRequest',
  EDITOR_DISPOSE: 'host:editorDispose',
};

const EDITOR_MESSAGE_TYPES = new Set([
  MESSAGE_TYPES.EDITOR_READY,
  MESSAGE_TYPES.EDITOR_CHANGE,
  MESSAGE_TYPES.EDITOR_SAVE,
  MESSAGE_TYPES.EDITOR_CANCEL,
  MESSAGE_TYPES.EDITOR_ERROR,
]);

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
    // sessionId is set for editor providers. Editor messages that do not echo
    // it are ignored.
    sessionId = null,
    onEditorReady = () => {},
    onEditorChange = () => {},
    onEditorSave = () => {},
    onEditorCancel = () => {},
    onEditorError = () => {},
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
    // Only accept messages from this bridge's own iframe. Two plugins (or two
    // editor instances) on one page would otherwise receive each other's
    // messages.
    if (!iframe?.contentWindow || event.source !== iframe.contentWindow) {
      return;
    }
    const message = event.data;

    if (!message?.type) return;

    if (EDITOR_MESSAGE_TYPES.has(message.type)) {
      if (sessionId == null || message.sessionId !== sessionId) {
        return;
      }
      switch (message.type) {
        case MESSAGE_TYPES.EDITOR_READY:
          onEditorReady(message);
          break;
        case MESSAGE_TYPES.EDITOR_CHANGE:
          onEditorChange(message);
          break;
        case MESSAGE_TYPES.EDITOR_SAVE:
          onEditorSave(message);
          break;
        case MESSAGE_TYPES.EDITOR_CANCEL:
          onEditorCancel(message);
          break;
        case MESSAGE_TYPES.EDITOR_ERROR:
          onEditorError(message);
          break;
      }
      return;
    }

    switch (message.type) {
      case MESSAGE_TYPES.READY:
        onReady();
        // Send initial theme
        sendThemeUpdate();
        break;

      case MESSAGE_TYPES.RESIZE:
        if (typeof message.height === 'number') {
          console.log(`[PluginBridge] Received plugin:resize height=${message.height}`);
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
        for (const rawRule of sheet.cssRules || []) {
          const rule = /** @type {CSSStyleRule} */ (rawRule);
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

  // Editor provider protocol. The host is the only writer: it holds the
  // markdown and persists it through the normal item/page APIs.
  function sendEditorInit(payload) {
    sendToPlugin({ type: MESSAGE_TYPES.EDITOR_INIT, sessionId, ...payload });
  }

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
    sendEditorInit,
    sendEditorUpdate: (markdown) =>
      sendToPlugin({ type: MESSAGE_TYPES.EDITOR_UPDATE, sessionId, markdown }),
    sendEditorSaveRequest: () =>
      sendToPlugin({ type: MESSAGE_TYPES.EDITOR_SAVE_REQUEST, sessionId }),
    sendEditorCancelRequest: () =>
      sendToPlugin({ type: MESSAGE_TYPES.EDITOR_CANCEL_REQUEST, sessionId }),
    sendEditorDispose: () => sendToPlugin({ type: MESSAGE_TYPES.EDITOR_DISPOSE, sessionId }),
  };
}
