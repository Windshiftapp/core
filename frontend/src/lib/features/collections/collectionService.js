import { api } from '../../api.js';

/**
 * Fetches items for a collection (or all workspace items if no collection).
 * Handles QL query resolution and correct API parameter naming.
 * @param {string|number} workspaceId
 * @param {string|number|null} collectionId
 * @param {{ page?: number, limit?: number, sub_ql?: string, [key: string]: any }} [options]
 */
export async function fetchCollectionItems(
  workspaceId,
  collectionId,
  { page, limit, sub_ql, ...extraFilters } = {}
) {
  let collectionName = 'Default';
  let collection = null;
  const filters = { ...extraFilters };
  if (page) filters.page = page;
  if (limit) filters.limit = limit;
  if (sub_ql) filters.sub_ql = sub_ql;

  if (collectionId) {
    collection = await getCollection(collectionId);
    if (collection) {
      collectionName = collection.name;
      // collection_id overrides workspace_id — let backend resolve the QL query
      filters.collection_id = collectionId;
    } else {
      filters.workspace_id = workspaceId;
    }
  } else {
    filters.workspace_id = workspaceId;
  }

  const response = await api.items.getAll(filters);
  const items = response?.items ?? (Array.isArray(response) ? response : []);
  const pagination = response?.pagination ?? null;
  const sortableFields = response?.sortable_fields ?? [];

  const publicSlug = (collection?.is_public && collection?.public_slug) ? collection.public_slug : null;
  return { items, collectionName, pagination, sortableFields, publicSlug };
}

/**
 * Fetches backlog items for a collection.
 * @param {string|number} workspaceId
 * @param {string|number|null} collectionId
 * @param {{ page?: number, limit?: number }} [options]
 */
export async function fetchCollectionBacklog(workspaceId, collectionId, { page, limit } = {}) {
  let collectionName = 'Default';

  if (collectionId) {
    const collection = await getCollection(collectionId);
    if (collection) {
      collectionName = collection.name;
    }
  }

  const response = await api.items.getBacklog(workspaceId, null, collectionId || null, {
    page,
    limit,
  });
  const items = response?.items ?? (Array.isArray(response) ? response : []);
  const pagination = response?.pagination ?? null;
  return { items, collectionName, pagination };
}

/**
 * Fetches a collection by ID (always fresh from server).
 * @param {string|number} collectionId - The collection ID
 * @returns {Promise<Object|null>} The collection object or null if not found
 */
export async function getCollection(collectionId) {
  if (!collectionId) return null;

  try {
    return await api.collections.get(String(collectionId));
  } catch (error) {
    console.error(`Failed to load collection ${collectionId}:`, error);
    return null;
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
