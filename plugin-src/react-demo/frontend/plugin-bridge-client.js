/**
 * Plugin Bridge Client SDK
 *
 * This SDK allows iframe plugins to communicate with the host application.
 * Usage: import { pluginBridge } from './plugin-bridge-client.js';
 */

const MESSAGE_TYPES = {
	// Plugin → Host
	READY: 'plugin:ready',
	RESIZE: 'plugin:resize',
	SHOW_MODAL: 'plugin:showModal',
	SHOW_CONFIRM: 'plugin:showConfirm',
	SHOW_TOAST: 'plugin:showToast',
	GET_THEME: 'plugin:getTheme',

	// Host → Plugin
	THEME_UPDATE: 'host:themeUpdate',
	MODAL_RESULT: 'host:modalResult',
};

class PluginBridge {
	constructor() {
		this.themeCallbacks = [];
		this.modalCallbacks = new Map();
		this.isReady = false;

		// Listen for messages from host
		window.addEventListener('message', this._handleMessage.bind(this));

		// Auto-resize observer
		this._setupAutoResize();
	}

	/**
	 * Notify host that plugin is ready
	 */
	ready() {
		this.isReady = true;
		this._sendToHost({ type: MESSAGE_TYPES.READY });
		console.log('[PluginBridge] Plugin ready, requested theme');
	}

	/**
	 * Send resize height to host
	 */
	resize(height) {
		this._sendToHost({
			type: MESSAGE_TYPES.RESIZE,
			height
		});
	}

	/**
	 * Show a modal in the host
	 * @param {Object} options - Modal options
	 * @param {string} options.title - Modal title
	 * @param {Object} options.content - Modal content data
	 * @param {string} options.maxWidth - Max width class (e.g., 'max-w-3xl')
	 * @returns {Promise<any>} Resolves with result when modal is closed
	 */
	showModal({ title, content, maxWidth = 'max-w-3xl' }) {
		return new Promise((resolve) => {
			const id = `modal-${Date.now()}-${Math.random()}`;

			this.modalCallbacks.set(id, resolve);

			this._sendToHost({
				type: MESSAGE_TYPES.SHOW_MODAL,
				id,
				title,
				content,
				maxWidth
			});
		});
	}

	/**
	 * Show a confirm dialog in the host
	 * @param {Object} options - Confirm options
	 * @param {string} options.title - Dialog title
	 * @param {string} options.message - Dialog message
	 * @param {string} options.confirmText - Confirm button text
	 * @param {string} options.cancelText - Cancel button text
	 * @param {string} options.variant - Button variant ('primary' or 'danger')
	 * @returns {Promise<boolean>} Resolves with true if confirmed, false if canceled
	 */
	showConfirm({
		title = 'Confirm',
		message,
		confirmText = 'Confirm',
		cancelText = 'Cancel',
		variant = 'primary'
	}) {
		return new Promise((resolve) => {
			const id = `confirm-${Date.now()}-${Math.random()}`;

			this.modalCallbacks.set(id, (result) => {
				resolve(result === 'confirm');
			});

			this._sendToHost({
				type: MESSAGE_TYPES.SHOW_CONFIRM,
				id,
				title,
				message,
				confirmText,
				cancelText,
				variant
			});
		});
	}

	/**
	 * Show a toast notification in the host
	 * @param {string} message - Toast message
	 * @param {string} variant - Toast variant ('success', 'error', 'warning', 'info')
	 * @param {number} duration - Duration in milliseconds (default: 3000)
	 */
	showToast(message, variant = 'info', duration = 3000) {
		this._sendToHost({
			type: MESSAGE_TYPES.SHOW_TOAST,
			message,
			variant,
			duration
		});
	}

	/**
	 * Register a callback for theme updates
	 * @param {Function} callback - Called with theme variables object
	 * @returns {Function} Unsubscribe function
	 */
	onTheme(callback) {
		this.themeCallbacks.push(callback);

		// Request current theme
		this._sendToHost({ type: MESSAGE_TYPES.GET_THEME });

		// Return unsubscribe function
		return () => {
			const index = this.themeCallbacks.indexOf(callback);
			if (index > -1) {
				this.themeCallbacks.splice(index, 1);
			}
		};
	}

	/**
	 * Apply theme variables to document
	 * @param {Object} variables - CSS variable object
	 */
	applyTheme(variables) {
		Object.entries(variables).forEach(([key, value]) => {
			document.documentElement.style.setProperty(key, value);
		});
	}

	/**
	 * Send message to host
	 */
	_sendToHost(message) {
		if (window.parent) {
			window.parent.postMessage(message, '*'); // In production, specify target origin
		}
	}

	/**
	 * Handle messages from host
	 */
	_handleMessage(event) {
		// In production, verify event.origin
		const message = event.data;

		if (!message || !message.type) return;

		switch (message.type) {
			case MESSAGE_TYPES.THEME_UPDATE:
				console.log('[PluginBridge] Received theme update:', message.variables);
				this.themeCallbacks.forEach(cb => cb(message.variables));
				// Auto-apply theme
				this.applyTheme(message.variables);
				break;

			case MESSAGE_TYPES.MODAL_RESULT:
				const callback = this.modalCallbacks.get(message.id);
				if (callback) {
					callback(message.result);
					this.modalCallbacks.delete(message.id);
				}
				break;
		}
	}

	/**
	 * Setup automatic resize observer
	 */
	_setupAutoResize() {
		if (typeof ResizeObserver === 'undefined') {
			// Fallback for browsers without ResizeObserver
			setInterval(() => {
				this.resize(document.body.scrollHeight);
			}, 500);
			return;
		}

		const observer = new ResizeObserver(() => {
			// Add some buffer to prevent scrollbars
			const height = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight);
			this.resize(height);
		});

		observer.observe(document.body);
		observer.observe(document.documentElement);
	}
}

// Export singleton instance
export const pluginBridge = new PluginBridge();

// Auto-notify ready after a short delay to ensure everything is loaded
setTimeout(() => {
	pluginBridge.ready();
}, 100);
