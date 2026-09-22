/** Command-palette display buckets. Item-key queries promote search results
 * above global navigation; commands within a bucket use score then insertion order. */

/** @type {Readonly<Record<string, string>>} */
export const BUCKET = Object.freeze({
  RECENT: 'recent',
  ITEM_ACTIONS: 'item-actions',
  WORKSPACE_ACTIONS: 'workspace-actions',
  WORKSPACE_NAVIGATION: 'workspace-navigation',
  MODULE_ACTIONS: 'module-actions',
  CREATE: 'create',
  ADMIN: 'admin',
  SEARCH_RESULTS: 'search-results',
  GLOBAL_NAVIGATION: 'global-navigation',
  SYSTEM: 'system',
});

const DEFAULT_ORDER = [
  BUCKET.RECENT,
  BUCKET.ITEM_ACTIONS,
  BUCKET.WORKSPACE_ACTIONS,
  BUCKET.WORKSPACE_NAVIGATION,
  BUCKET.MODULE_ACTIONS,
  BUCKET.CREATE,
  BUCKET.ADMIN,
  BUCKET.GLOBAL_NAVIGATION,
  BUCKET.SEARCH_RESULTS,
  BUCKET.SYSTEM,
];

const ITEM_KEY_RE = /^[A-Z][A-Z0-9]*-\d+$/i;

/** Return a query-aware bucket rank (lower appears earlier). */
export function bucketRank(bucket, query) {
  let order = DEFAULT_ORDER;
  if (query && ITEM_KEY_RE.test(query.trim())) {
    order = DEFAULT_ORDER.slice();
    const search = order.indexOf(BUCKET.SEARCH_RESULTS);
    const global = order.indexOf(BUCKET.GLOBAL_NAVIGATION);
    if (search > global) {
      order.splice(search, 1);
      order.splice(global, 0, BUCKET.SEARCH_RESULTS);
    }
  }
  const idx = order.indexOf(bucket);
  return idx === -1 ? order.length : idx;
}

export const BUCKET_LABELS = Object.freeze({
  [BUCKET.RECENT]: 'Recent',
  [BUCKET.ITEM_ACTIONS]: 'Item',
  [BUCKET.WORKSPACE_ACTIONS]: 'Workspace',
  [BUCKET.WORKSPACE_NAVIGATION]: 'Workspace',
  [BUCKET.MODULE_ACTIONS]: 'Tools',
  [BUCKET.CREATE]: 'Create',
  [BUCKET.ADMIN]: 'Admin',
  [BUCKET.SEARCH_RESULTS]: 'Search',
  [BUCKET.GLOBAL_NAVIGATION]: 'Navigation',
  [BUCKET.SYSTEM]: 'System',
});

// Localized consumers resolve these keys at render time so an open palette
// follows locale changes. Keep BUCKET_LABELS for existing consumers.
export const BUCKET_LABEL_KEYS = Object.freeze({
  [BUCKET.RECENT]: 'mobile.myWork.recent',
  [BUCKET.ITEM_ACTIONS]: 'mobile.palette.item',
  [BUCKET.WORKSPACE_ACTIONS]: 'common.workspace',
  [BUCKET.WORKSPACE_NAVIGATION]: 'common.workspace',
  [BUCKET.MODULE_ACTIONS]: 'mobile.palette.tools',
  [BUCKET.CREATE]: 'common.create',
  [BUCKET.ADMIN]: 'mobile.palette.admin',
  [BUCKET.SEARCH_RESULTS]: 'common.search',
  [BUCKET.GLOBAL_NAVIGATION]: 'mobile.palette.navigation',
  [BUCKET.SYSTEM]: 'mobile.palette.system',
});

// Per-bucket cap is set so empty-query palette renders all workspace views
// (up to ~12) and global nav items (up to ~10). Total cap keeps the list
// scannable without burying admin / system commands.
export const PER_BUCKET_CAP = 8;
export const TOTAL_CAP = 20;
