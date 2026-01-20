import { writable } from 'svelte/store';

// Store for extension registry
export const extensions = writable({});

/**
 * Load extensions from the API
 * @returns {Promise<void>}
 */
export async function loadExtensions() {
	try {
		const response = await fetch('/api/plugins/extensions', {
			credentials: 'include'
		});

		if (!response.ok) {
			console.error('Failed to load extensions:', response.statusText);
			return;
		}

		const data = await response.json();
		extensions.set(data);

		console.log('Loaded plugin extensions:', data);
	} catch (error) {
		console.error('Error loading extensions:', error);
	}
}

/**
 * Get extensions for a specific extension point
 * @param {object} extensionsData - The extensions data from the store
 * @param {string} point - Extension point name (e.g., "admin.tab")
 * @returns {Array} Array of extensions for the given point
 */
export function getExtensionsForPoint(extensionsData, point) {
	return extensionsData[point] || [];
}
