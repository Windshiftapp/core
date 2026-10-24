import { collections } from '../../api/collections.js';
import { items as itemsApi } from '../../api/items.js';

/**
 * Builds the PATCH payload for associating a collection with a workspace.
 * Association is not publishing, so public-sharing fields are intentionally
 * omitted: an owner without public_board.manage must still be able to move the
 * collection. A workspace association also clears any category, which is only
 * valid for global collections.
 */
export function buildCollectionAssociationUpdate({
  collection,
  qlQuery,
  filterState,
  workspaceId,
}) {
  return {
    name: collection.name,
    description: collection.description || null,
    ql_query: qlQuery,
    filter_state: filterState,
    workspace_id: workspaceId ?? null,
    category_id: workspaceId ? null : (collection.category_id ?? null),
    is_private: collection.is_private ?? false,
  };
}

/**
 * Merge a privacy-toggle PATCH response into the current collection. Marking a
 * collection private unpublishes its public board, and the API omits
 * `public_slug` once it is cleared, so reset both public fields from the
 * response instead of only flipping the local privacy flag.
 */
export function applyCollectionPrivacyToggle(collection, updated, isPrivate) {
  return {
    ...collection,
    ...updated,
    is_private: isPrivate,
    is_public: updated?.is_public ?? false,
    public_slug: updated?.public_slug ?? null,
  };
}

/**
 * Collection-specific list-column configuration is owned by the collection
 * creator; workspace-default configuration is owned by workspace admins. The
 * API enforces the same split, so a collection owner without workspace.admin
 * must still be able to configure their collection's columns.
 */
export function canConfigureListColumns({
  collectionId,
  collectionOwnerId,
  currentUserId,
  canAdminWorkspace,
}) {
  if (collectionId) {
    return (
      collectionOwnerId != null &&
      currentUserId != null &&
      String(collectionOwnerId) === String(currentUserId)
    );
  }
  return Boolean(canAdminWorkspace);
}

function collectionPagination(pagination) {
  if (!pagination) return null;
  // Store continuations use limit; v2 responses name the effective size page_size.
  return { ...pagination, limit: pagination.page_size };
}

// Use the authorized item query without view filters, fetching only one summary.
export async function fetchCollectionTotal(workspaceId, collectionId, requestOptions = {}) {
  const scope = collectionId ? { collection_id: collectionId } : { workspace_id: workspaceId };
  const response = await itemsApi.getAll(
    {
      ...scope,
      page: 1,
      limit: 1,
      omit_descriptions: true,
      include_watermark: true,
    },
    requestOptions
  );
  return {
    total: response?.pagination?.total_items ?? null,
    watermark: response?.meta?.watermark ?? 0,
  };
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
  {
    page,
    limit,
    sub_ql,
    collection: preloadedCollection,
    requestOptions = {},
    ...extraFilters
  } = {}
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

  const response = await itemsApi.getAll(filters, requestOptions);
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
 * @param {{ page?: number, limit?: number, sub_ql?: string, collection?: any, requestOptions?: object }} [options]
 */
export async function fetchCollectionBacklog(
  workspaceId,
  collectionId,
  { page, limit, sub_ql, collection: preloadedCollection, requestOptions = {} } = {}
) {
  let collectionName = 'Default';

  if (collectionId) {
    const collection =
      preloadedCollection !== undefined ? preloadedCollection : await getCollection(collectionId);
    if (collection) {
      collectionName = collection.name;
    }
  }

  const response = await itemsApi.getBacklog(workspaceId, null, collectionId || null, {
    page,
    limit,
    sub_ql,
    omit_descriptions: true,
    include_watermark: true,
    requestOptions,
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
 * @param {{ since?: number|string, sub_ql?: string, requestOptions?: object }} [options]
 */
export async function fetchCollectionItemChanges(
  workspaceId,
  collectionId,
  { since, sub_ql, requestOptions = {} } = {}
) {
  const filters = {};
  if (workspaceId) filters.workspace_id = workspaceId;
  if (collectionId) filters.collection_id = collectionId;
  if (since !== undefined && since !== null) filters.since = since;
  if (sub_ql) filters.sub_ql = sub_ql;
  return itemsApi.getChanges(filters, requestOptions);
}

/**
 * Fetches a set of items by ID. This is intentionally a client helper rather
 * than a full collection reload so live updates can patch only loaded rows.
 * @param {Array<number>} ids
 */
export async function fetchItemsById(ids) {
  if (!ids?.length) return [];
  return itemsApi.getMany(ids);
}

/**
 * Fetches a collection by ID (always fresh from server).
 * @param {string|number} collectionId - The collection ID
 * @returns {Promise<Object|null>} The collection object or null if not found
 */
export async function getCollection(collectionId) {
  if (!collectionId) return null;

  try {
    return await collections.get(String(collectionId));
  } catch (error) {
    console.error(`Failed to load collection ${collectionId}:`, error);
    return null;
  }
}

/**
 * Classify an item's membership in the given filters (e.g. collection QL).
 * An unknown result must never be treated as membership: callers defer to an
 * authoritative reconciliation instead of inserting a possibly out-of-filter
 * row.
 * @param {number} itemId - The item ID to check
 * @param {Object} filters - The filters to apply (same format as itemsApi.getAll)
 * @returns {Promise<'member'|'nonmember'|'unknown'>}
 */
export async function checkItemVisibility(itemId, filters) {
  if (!itemId) return 'nonmember';

  try {
    // Query the API with the same filters + the specific item ID
    const filtersWithId = { ...filters, id: itemId };
    const response = await itemsApi.getAll(filtersWithId);
    const items = response?.data ?? [];
    return items.some((item) => item.id === itemId) ? 'member' : 'nonmember';
  } catch (error) {
    console.error(`Failed to check visibility for item ${itemId}:`, error);
    return 'unknown';
  }
}

/**
 * Decide whether a just-created item belongs in the current collection view.
 * Collection QL and sub-filters depend on server state, so membership is
 * verified rather than inferred from the workspace. An `unknown` result must
 * never be treated as membership; callers reconcile instead.
 *
 * @param {{ id: number, workspace_id?: number|string }} item
 * @param {{ workspaceId?: number|string|null, collectionId?: number|string|null, subFilterQL?: string }} scope
 * @returns {Promise<'member'|'nonmember'|'unknown'>}
 */
export async function resolveCreatedItemMembership(
  item,
  { workspaceId, collectionId, subFilterQL } = {}
) {
  if (collectionId || subFilterQL) {
    const filters = collectionId ? { collection_id: collectionId } : { workspace_id: workspaceId };
    if (subFilterQL) filters.sub_ql = subFilterQL;
    return checkItemVisibility(item.id, filters);
  }
  if (workspaceId) {
    return Number(item.workspace_id) === Number(workspaceId) ? 'member' : 'nonmember';
  }
  return 'member';
}
