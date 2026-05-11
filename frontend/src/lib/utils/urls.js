/**
 * Central URL builders for routed entities used as `href` on `<a>` / `<Link>`.
 *
 * Builders return a path (not an absolute URL). Pass straight into
 * `<a href={...}>`, `<Link href={...}>`, or `navigate(...)`.
 */

function qs(params) {
  const entries = Object.entries(params).filter(
    ([, v]) => v !== undefined && v !== null && v !== ''
  );
  if (entries.length === 0) return '';
  const usp = new URLSearchParams();
  for (const [k, v] of entries) usp.set(k, String(v));
  return `?${usp.toString()}`;
}

/**
 * @param {object} [opts]
 * @param {string|number} [opts.workspaceId]
 * @param {string|number} [opts.itemId]
 * @param {string|number} [opts.collectionId]
 * @param {boolean} [opts.isPersonal] - use /personal/items/:id
 * @param {string} [opts.tab] - appended as ?tab=...
 */
export function itemUrl({ workspaceId, itemId, collectionId, isPersonal = false, tab } = {}) {
  let path;
  if (isPersonal) {
    path = `/personal/items/${itemId}`;
  } else if (collectionId) {
    path = `/workspaces/${workspaceId}/collections/${collectionId}/items/${itemId}`;
  } else {
    path = `/workspaces/${workspaceId}/items/${itemId}`;
  }
  return path + qs({ tab });
}

export function portalUrl(slug) {
  return `/portal/${slug}`;
}

export function portalRequestUrl(slug, itemId) {
  return `/portal/${slug}${qs({ view: 'requests', id: itemId })}`;
}

export function portalRequestTypeUrl(slug, requestTypeId) {
  return `/portal/${slug}${qs({ 'request-type': requestTypeId })}`;
}
