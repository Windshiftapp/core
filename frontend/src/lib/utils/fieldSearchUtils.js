/**
 * Creates a search-filtered getter for a list of fields.
 * @param {() => any[]} getFields - Function that returns the fields to filter
 * @param {() => string} getSearchQuery - Function that returns the search query
 * @returns {any[]} Filtered fields array
 */
export function createSearchFilteredFields(getFields, getSearchQuery) {
  return getFields().filter((field) => {
    if (!getSearchQuery().trim()) return true;
    const query = getSearchQuery().toLowerCase();
    return (
      field.name.toLowerCase().includes(query) || field.identifier.toLowerCase().includes(query)
    );
  });
}
