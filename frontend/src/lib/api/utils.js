/**
 * Build a query string from a params object, filtering out null, undefined, and empty string values.
 * @param {Record<string, any>} params
 * @returns {string} Query string with leading '?' or empty string
 */
export function buildQueryString(params) {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== null && value !== undefined && value !== '') {
      query.append(key, value);
    }
  });
  const qs = query.toString();
  return qs ? `?${qs}` : '';
}
