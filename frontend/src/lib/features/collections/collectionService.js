import { api } from '../../api.js';

// Cache for collection data to avoid redundant API calls
const collectionCache = new Map();

/**
 * Fetches a collection by ID with caching
 * @param {string|number} collectionId - The collection ID
 * @returns {Promise<Object|null>} The collection object or null if not found
 */
export async function getCollection(collectionId) {
  if (!collectionId) return null;

  const id = String(collectionId);

  // Check cache first
  if (collectionCache.has(id)) {
    return collectionCache.get(id);
  }

  try {
    const collection = await api.collections.get(id);
    collectionCache.set(id, collection);
    return collection;
  } catch (error) {
    console.error(`Failed to load collection ${id}:`, error);
    return null;
  }
}

/**
 * Clears the collection cache
 */
export function clearCollectionCache() {
  collectionCache.clear();
}

/**
 * Removes a specific collection from cache
 * @param {string|number} collectionId - The collection ID
 */
export function invalidateCollection(collectionId) {
  if (collectionId) {
    collectionCache.delete(String(collectionId));
  }
}

/**
 * Checks if an item would be visible given a set of filters (e.g., collection filters)
 * @param {number} itemId - The item ID to check
 * @param {Object} filters - The filters to apply (same format as api.items.getAll)
 * @returns {Promise<boolean>} True if the item is visible with the given filters
 */
export async function checkItemVisibility(itemId, filters) {
  if (!itemId) return false;

  try {
    // Query the API with the same filters + the specific item ID
    const filtersWithId = { ...filters, id: itemId };
    const response = await api.items.getAll(filtersWithId);

    // Handle paginated response
    const items = response?.items || response || [];

    // Check if the item is in the results
    return items.some((item) => item.id === itemId);
  } catch (error) {
    console.error(`Failed to check visibility for item ${itemId}:`, error);
    // If there's an error, assume the item is visible to avoid confusing the user
    return true;
  }
}
