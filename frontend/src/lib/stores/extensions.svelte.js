// Extensions store for plugin extension registry
// Uses Svelte 5 runes for proper reactivity tracking

export const EDITOR_MARKDOWN_POINT = 'editor.markdown';

let extensionsData = $state({});
// Single in-flight load shared by every consumer. The editor host mounts many
// times per page (item description, comment composers, page editors), so it
// cannot fetch per mount.
let loadPromise = null;

export const extensions = {
  get value() {
    return extensionsData;
  },
};

/**
 * Load extensions from the API once, sharing the in-flight request and caching
 * the result. Pass `{ force: true }` to bypass the cache.
 * @param {{ force?: boolean }} [options]
 * @returns {Promise<any>}
 */
export async function ensureExtensionsLoaded({ force = false } = {}) {
  if (!force && Object.keys(extensionsData).length > 0) return extensionsData;
  if (!force && loadPromise) return loadPromise;

  loadPromise = (async () => {
    try {
      const response = await fetch('/api/plugins/extensions', {
        credentials: 'include',
      });

      if (!response.ok) {
        console.error('Failed to load extensions:', response.statusText);
        return extensionsData;
      }

      extensionsData = await response.json();
      return extensionsData;
    } catch (error) {
      console.error('Error loading extensions:', error);
      return extensionsData;
    } finally {
      loadPromise = null;
    }
  })();

  return loadPromise;
}

/**
 * Load extensions from the API
 * @returns {Promise<any>}
 */
export async function loadExtensions() {
  return ensureExtensionsLoaded({ force: true });
}

/**
 * Get extensions for a specific extension point
 * @param {object} data - The extensions data from the store
 * @param {string} point - Extension point name (e.g., "admin.tab")
 * @returns {Array} Array of extensions for the given point
 */
export function getExtensionsForPoint(data, point) {
  return data?.[point] || [];
}

/**
 * Get the editor providers a surface may use, ordered by `order`.
 *
 * A provider that declares no `entityTypes` applies to every surface; one that
 * declares a list applies only to those.
 *
 * @param {object} data - The extensions data from the store
 * @param {string|null} entityType - "item" | "page" | "comment"
 * @returns {Array} Ordered editor providers
 */
export function getEditorProviders(data, entityType) {
  return getExtensionsForPoint(data, EDITOR_MARKDOWN_POINT)
    .filter(
      (extension) =>
        !entityType || !extension.entityTypes?.length || extension.entityTypes.includes(entityType)
    )
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
}
