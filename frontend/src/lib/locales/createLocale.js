/**
 * Merges all locale modules into a single flat translations object.
 * @param {Record<string, object>} modules - Named imports from locale-specific files
 * @returns {object} Merged translations
 */
export function createLocale(modules) {
  return Object.assign({}, ...Object.values(modules));
}
