/**
 * Helpers around the JSON-encoded gherkin.ScenarioSpec stored on BDD test
 * cases and frozen into run snapshots.
 */

/**
 * Parse a stored spec string. Returns null when the spec is missing or
 * unreadable so callers can fall back to raw Gherkin rendering.
 * @param {string | null | undefined} specJson
 * @returns {object | null}
 */
export function parseScenarioSpec(specJson) {
  if (!specJson) return null;
  try {
    const spec = JSON.parse(specJson);
    return spec && Array.isArray(spec.steps) ? spec : null;
  } catch {
    return null;
  }
}

/**
 * Flatten a spec's Examples blocks into a single ordered list. The backend
 * indexes example results across blocks in document order, so the global
 * index here matches the example_index used by the run endpoints.
 *
 * A plain Scenario has no Examples blocks; the backend still creates one
 * implicit execution row (index 0) for it, so an empty spec yields that row
 * to keep plain scenarios executable in the run views.
 *
 * @param {object | null | undefined} spec parsed ScenarioSpec
 */
export function flattenExamples(spec) {
  if (!spec) return [];
  const blocks = Array.isArray(spec.examples) ? spec.examples : [];
  const rows = [];
  blocks.forEach((block, blockIndex) => {
    const header = Array.isArray(block.header) ? block.header : [];
    (Array.isArray(block.rows) ? block.rows : []).forEach((row, rowIndex) => {
      rows.push({
        exampleIndex: rows.length,
        blockIndex,
        rowIndex,
        blockName: block.name || '',
        tags: Array.isArray(block.tags) ? block.tags : [],
        header,
        row,
      });
    });
  });
  if (rows.length === 0) {
    rows.push({
      exampleIndex: 0,
      blockIndex: -1,
      rowIndex: -1,
      blockName: '',
      tags: [],
      header: [],
      row: [],
    });
  }
  return rows;
}

/**
 * Format one example's row values as a compact "header: value" label for
 * result lists.
 * @param {Record<string, string> | undefined} rowValues
 */
export function formatExampleRow(rowValues) {
  if (!rowValues) return '';
  return Object.entries(rowValues)
    .map(([key, value]) => `${key}: ${value}`)
    .join(', ');
}

/**
 * Substitute <param> placeholders in a step text with one example row's
 * values. Unmatched placeholders are left untouched.
 * @param {string} text
 * @param {Record<string, string> | undefined} rowValues
 */
export function applyExampleRow(text, rowValues) {
  if (!text || !rowValues) return text || '';
  return text.replace(/<([^<>]+)>/g, (placeholder, key) => {
    const value = rowValues[key];
    return value === undefined ? placeholder : value;
  });
}
