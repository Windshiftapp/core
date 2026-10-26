// Personal saved board filters (WI-96). The lightweight slice keeps named
// SubFilterBar filter sets in browser storage, scoped to the user's board or
// collection. Shared, collection-scoped named views are a separate feature.

const MAX_NAME_LENGTH = 60;

/** Storage key for the personal saved filters of a board/collection scope. */
export function savedFiltersStorageKey(workspaceId, collectionId) {
  const scope = collectionId ? `collection-${collectionId}` : `workspace-${workspaceId}`;
  return `board-saved-filters:${scope}`;
}

/**
 * Normalizes saved filter sets from a stored JSON string or an already-parsed
 * value (the server stores the same shape). Drops malformed entries instead of
 * failing the whole board when one row is corrupt.
 * @param {string|unknown[]|null|undefined} value
 */
export function normalizeSavedFilters(value) {
  if (!value) return [];
  let parsed = value;
  if (typeof value === 'string') {
    try {
      parsed = JSON.parse(value);
    } catch {
      return [];
    }
  }
  if (!Array.isArray(parsed)) return [];
  return /** @type {any[]} */ (parsed)
    .filter((entry) => entry && typeof entry === 'object' && typeof entry.name === 'string')
    .map((entry, index) => ({
      id: String(entry.id ?? `saved-${index}`),
      name: entry.name,
      ql: typeof entry.ql === 'string' ? entry.ql : '',
      rows: Array.isArray(entry.rows) ? entry.rows : [],
      pinned: entry.pinned === true,
    }))
    .filter((entry) => entry.ql);
}

/** Parses a stored JSON string of saved filters. */
export function parseSavedFilters(raw) {
  return normalizeSavedFilters(raw);
}

/** Serializes saved filters back to storage, keeping only owned fields. */
export function serializeSavedFilters(filters) {
  return JSON.stringify(
    filters.map(({ id, name, ql, rows, pinned }) => ({
      id,
      name,
      ql,
      rows,
      pinned: Boolean(pinned),
    }))
  );
}

function normalizeName(name) {
  return String(name ?? '').trim();
}

/**
 * Adds a named filter set. Returns the next list plus an error code, so the
 * caller can surface validation without throwing.
 * @param {Array} filters
 * @param {{name: string, ql: string, rows?: unknown[]}} entry
 * @param {() => string} [idFactory]
 */
export function addSavedFilter(filters, { name, ql, rows }, idFactory) {
  const trimmed = normalizeName(name);
  if (!trimmed) return { filters, error: 'name_required' };
  if (trimmed.length > MAX_NAME_LENGTH) return { filters, error: 'name_too_long' };
  if (!ql) return { filters, error: 'filter_required' };
  if (filters.some((entry) => entry.name.toLowerCase() === trimmed.toLowerCase())) {
    return { filters, error: 'duplicate_name' };
  }
  const id = idFactory
    ? idFactory()
    : `filter-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  return {
    filters: [...filters, { id, name: trimmed, ql, rows: rows ?? [], pinned: false }],
    error: null,
  };
}

export function removeSavedFilter(filters, id) {
  return filters.filter((entry) => entry.id !== id);
}

export function togglePinnedSavedFilter(filters, id) {
  return filters.map((entry) => (entry.id === id ? { ...entry, pinned: !entry.pinned } : entry));
}

/** Pinned filters first so the quick-access chips stay stable. */
export function orderedSavedFilters(filters) {
  return filters.slice().sort((a, b) => Number(b.pinned) - Number(a.pinned));
}

export const SAVED_FILTER_MAX_NAME_LENGTH = MAX_NAME_LENGTH;
