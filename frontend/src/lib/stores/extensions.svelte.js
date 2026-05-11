// Extensions store for plugin extension registry
// Uses Svelte 5 runes for proper reactivity tracking

let extensionsData = $state({});

export const extensions = {
  get value() {
    return extensionsData;
  },
};

/**
 * Load extensions from the API
 * @returns {Promise<any>}
 */
export async function loadExtensions() {
  try {
    const response = await fetch('/api/plugins/extensions', {
      credentials: 'include',
    });

    if (!response.ok) {
      console.error('Failed to load extensions:', response.statusText);
      return;
    }

    extensionsData = await response.json();
    return extensionsData;
  } catch (error) {
    console.error('Error loading extensions:', error);
  }
}

/**
 * Get extensions for a specific extension point
 * @param {object} data - The extensions data from the store
 * @param {string} point - Extension point name (e.g., "admin.tab")
 * @returns {Array} Array of extensions for the given point
 */
export function getExtensionsForPoint(data, point) {
  return data[point] || [];
}
