// Query specification helpers shared by the widget renderer, the dashboard
// data prefetcher, and the builder preview.
//
// The shape here is the contract with the server validator in
// internal/customwidgets: a slot carries only a CQL string, a sort, a limit,
// and (for assets) a set ID. Scope is never part of the spec.

export const DEFAULT_LIMIT = 100;
export const MAX_LIMIT = 1000;

function prune(value) {
  const out = {};
  for (const [key, entry] of Object.entries(value)) {
    if (entry === undefined || entry === null || entry === '') continue;
    out[key] = entry;
  }
  return out;
}

function sortKeys(value) {
  if (Array.isArray(value)) return value.map(sortKeys);
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .map((key) => [key, sortKeys(value[key])])
    );
  }
  return value;
}

/**
 * Build an item query slot from component props.
 * @param {{ query?: string, sort?: string, limit?: number }} props
 */
export function itemSpec({ query = '', sort = '', limit = DEFAULT_LIMIT } = {}) {
  return { entity: 'item', ...prune({ ql: query, sort, limit }) };
}

/**
 * Build an asset query slot from component props.
 * @param {{ query?: string, sort?: string, limit?: number, assetSetId?: number, linkProjection?: object }} props
 */
export function assetSpec({
  query = '',
  sort = '',
  limit = DEFAULT_LIMIT,
  assetSetId = undefined,
  linkProjection = undefined,
} = {}) {
  return {
    entity: 'asset',
    ...prune({
      ql: query,
      sort,
      limit,
      asset_set_id: assetSetId,
      link_projection: linkProjection,
    }),
  };
}

/**
 * Build a bipartite edge slot: an item seed set, an asset seed set, and the
 * links between them.
 */
export function linksSpec({
  itemQuery = '',
  itemSort = '',
  itemLimit = MAX_LIMIT,
  assetQuery = '',
  assetSort = '',
  assetLimit = MAX_LIMIT,
  assetSetId = undefined,
  maxEdges = undefined,
} = {}) {
  return {
    entity: 'links',
    links: prune({
      item: prune({ ql: itemQuery, sort: itemSort, limit: itemLimit }),
      asset: prune({
        ql: assetQuery,
        sort: assetSort,
        limit: assetLimit,
        asset_set_id: assetSetId,
      }),
      max_edges: maxEdges,
    }),
  };
}

/** Stable cache key for a spec, independent of property order. */
export function sourceKey(spec) {
  return JSON.stringify(sortKeys(prune(spec)));
}

/**
 * Resolve a dotted field path on a normalized row. Object values fall back to
 * their `name` so grouping by `status` works on `{id, name}` objects.
 */
export function fieldValue(row, path) {
  if (!row || !path) return undefined;
  let current = row;
  for (const part of String(path).split('.')) {
    if (current === null || current === undefined) return undefined;
    current = current[part];
  }
  if (Array.isArray(current)) return current;
  if (current && typeof current === 'object') {
    return current.name ?? current.title ?? current.key ?? undefined;
  }
  return current;
}

/** Format a value for display in a table cell. */
export function formatValue(value) {
  if (value === null || value === undefined) return '—';
  if (value instanceof Date) return value.toLocaleDateString();
  if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}T/.test(value)) {
    const date = new Date(value);
    if (!Number.isNaN(date.getTime())) return date.toLocaleDateString();
  }
  if (typeof value === 'boolean') return value ? 'Yes' : 'No';
  return String(value);
}

/** Split a comma-separated column list into field paths. */
export function parseColumns(columns) {
  if (!columns) return [];
  return String(columns)
    .split(',')
    .map((column) => column.trim())
    .filter(Boolean);
}
