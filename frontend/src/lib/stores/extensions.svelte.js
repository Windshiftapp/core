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
 * @param {{ quiet?: boolean }} [options] quiet: don't log failures, e.g. when every page
 *   loads extensions for the sidebar and plugins are disabled on this instance.
 * @returns {Promise<any>}
 */
export async function loadExtensions({ quiet = false } = {}) {
  try {
    const response = await fetch('/api/plugins/extensions', {
      credentials: 'include',
    });

    if (!response.ok) {
      if (!quiet) console.error('Failed to load extensions:', response.statusText);
      return;
    }

    extensionsData = await response.json();
    return extensionsData;
  } catch (error) {
    if (!quiet) console.error('Error loading extensions:', error);
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
