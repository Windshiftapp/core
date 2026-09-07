import { api } from '../../api.js';

function collectionPagination(pagination) {
  if (!pagination) return null;
  // Store continuations use limit; v2 responses name the effective size page_size.
  return { ...pagination, limit: pagination.page_size };
}

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
  { page, limit, sub_ql, collection: preloadedCollection, ...extraFilters } = {}
) {
  let collectionName = 'Default';
  let collection = null;
  const filters = { ...extraFilters };
  if (page) filters.page = page;
  if (limit) filters.limit = limit;
  if (sub_ql) filters.sub_ql = sub_ql;
  // Collection views render cards/rows from metadata only. Avoid shipping large
  // markdown/TipTap descriptions for every loaded item; item detail fetches
  // still retrieve the full description on demand.
  filters.omit_descriptions = true;
  filters.include_watermark = true;

  if (collectionId) {
    // Reuse a collection the caller already fetched this load cycle (one board
    // load otherwise issued the same /collections/{id} request 2-3 times).
    collection =
      preloadedCollection !== undefined ? preloadedCollection : await getCollection(collectionId);
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
  const items = response?.data ?? [];
  const pagination = collectionPagination(response?.pagination);
  const sortableFields = response?.meta?.sortable_fields ?? [];
  const watermark = response?.meta?.watermark ?? 0;

  const publicSlug =
    collection?.is_public && collection?.public_slug ? collection.public_slug : null;
  return { items, collectionName, pagination, sortableFields, publicSlug, watermark };
}

/**
 * Fetches backlog items for a collection.
 * @param {string|number} workspaceId
 * @param {string|number|null} collectionId
 * @param {{ page?: number, limit?: number, sub_ql?: string, collection?: any }} [options]
 */
export async function fetchCollectionBacklog(
  workspaceId,
  collectionId,
  { page, limit, sub_ql, collection: preloadedCollection } = {}
) {
  let collectionName = 'Default';

  if (collectionId) {
    const collection =
      preloadedCollection !== undefined ? preloadedCollection : await getCollection(collectionId);
    if (collection) {
      collectionName = collection.name;
    }
  }

  const response = await api.items.getBacklog(workspaceId, null, collectionId || null, {
    page,
    limit,
    sub_ql,
    omit_descriptions: true,
    include_watermark: true,
  });
  const items = response?.data ?? [];
  const pagination = collectionPagination(response?.pagination);
  const watermark = response?.meta?.watermark ?? 0;
  return { items, collectionName, pagination, watermark };
}

/**
 * Fetches cheap item deltas for the current workspace/collection view.
 * @param {string|number|null} workspaceId
 * @param {string|number|null} collectionId
 * @param {{ since?: number|string, sub_ql?: string }} [options]
 */
export async function fetchCollectionItemChanges(
  workspaceId,
  collectionId,
  { since, sub_ql } = {}
) {
  const filters = {};
  if (workspaceId) filters.workspace_id = workspaceId;
  if (collectionId) filters.collection_id = collectionId;
  if (since !== undefined && since !== null) filters.since = since;
  if (sub_ql) filters.sub_ql = sub_ql;
  return api.items.getChanges(filters);
}

/**
 * Fetches a set of items by ID. This is intentionally a client helper rather
 * than a full collection reload so live updates can patch only loaded rows.
 * @param {Array<number>} ids
 */
export async function fetchItemsById(ids) {
  if (!ids?.length) return [];
  return api.items.getMany(ids);
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
    const items = response?.data ?? [];

    // Check if the item is in the results
    return items.some((item) => item.id === itemId);
  } catch (error) {
    console.error(`Failed to check visibility for item ${itemId}:`, error);
    // If there's an error, assume the item is visible to avoid confusing the user
    return true;
  }
}
