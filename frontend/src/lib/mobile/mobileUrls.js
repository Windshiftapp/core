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
  if (url === '/search') return '/m/search';
  if (url === '/notifications') return '/m/notifications';
  if (url === '/personal') return '/m/personal';
  if (url === '/time' || url.startsWith('/time/')) return '/m/timer';

  let m = url.match(/^\/workspaces\/(\d+)\/items\/(\d+)/);
  if (m) return `/m/items/${m[2]}`;

  m = url.match(/^\/workspaces\/(\d+)\/pages\/(\d+)/);
  if (m) return `/m/pages/${m[1]}/${m[2]}`;

  return url;
}
