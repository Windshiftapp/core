/**
 * Map a desktop-internal URL to its mobile-shell equivalent. Command-palette
 * entries, sidebar hrefs, and recently-viewed rows are all authored against
 * desktop routes; on the phone surface those destinations must land in the
 * /m shell instead of rendering desktop chrome on a handset. URLs without a
 * mobile equivalent are returned unchanged.
 *
 * @param {string} url
 * @returns {string}
 */
export function toMobileUrl(url) {
  if (!url) return url;
  // Split off the query/hash so the patterns below match paths only, then
  // re-attach whatever followed the path.
  const match = url.match(/^([^?#]*)([?#].*)?$/);
  const path = match[1];
  const rest = match[2] ?? '';
  const rewrite = (mobilePath) => mobilePath + rest;

  if (path === '/search') return rewrite('/m/search');
  if (path === '/notifications') return rewrite('/m/notifications');
  if (path === '/personal') return rewrite('/m/personal');
  if (path === '/time' || path.startsWith('/time/')) return rewrite('/m/timer');

  let m = path.match(/^\/workspaces\/(\d+)\/items\/(\d+)/);
  if (m) return rewrite(`/m/items/${m[2]}`);

  m = path.match(/^\/workspaces\/(\d+)\/pages\/(\d+)/);
  if (m) return rewrite(`/m/pages/${m[1]}/${m[2]}`);

  return url;
}
