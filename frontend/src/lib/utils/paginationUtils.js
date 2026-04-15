/**
 * Calculates whether there are more pages to load based on pagination info.
 * @param {Object|null} pagination - Pagination object with page and total_pages
 * @returns {boolean} True if there are more pages
 */
export function calcHasMore(pagination) {
  if (!pagination) return false;
  return pagination.page < pagination.total_pages;
}
