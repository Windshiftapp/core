/**
 * Parse custom field options from either new ID-based format or legacy string array format.
 * New format: {"next_id": 5, "items": [{"id": 1, "label": "Critical"}, ...]}
 * Legacy format: ["Critical", "High", "Medium", "Low"]
 * @param {string|null|undefined} optionsStr - JSON string of options
 * @returns {{ nextId: number, items: Array<{id: number, label: string}> }}
 */
export function parseFieldOptions(optionsStr) {
  if (!optionsStr) return { nextId: 1, items: [] };

  try {
    const parsed = JSON.parse(optionsStr);

    // New format: object with next_id and items
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed) && parsed.next_id && Array.isArray(parsed.items)) {
      return {
        nextId: parsed.next_id,
        items: parsed.items.map(item => ({ id: item.id, label: item.label }))
      };
    }

    // Legacy format: string array
    if (Array.isArray(parsed)) {
      const items = parsed.map((label, index) => ({ id: index + 1, label: String(label) }));
      return { nextId: items.length + 1, items };
    }
  } catch {
    // ignore parse errors
  }

  return { nextId: 1, items: [] };
}

/**
 * Resolve a single option ID to its label.
 * Handles both numeric IDs (new format) and string values (legacy).
 * @param {string|null|undefined} optionsStr - JSON string of options
 * @param {number|string} value - Option ID or legacy string value
 * @returns {string} The label, or the value cast to string if not found
 */
export function resolveOptionLabel(optionsStr, value) {
  if (value === null || value === undefined || value === '') return '';

  const { items } = parseFieldOptions(optionsStr);

  // Try numeric ID match
  if (typeof value === 'number' || (typeof value === 'string' && /^\d+$/.test(value))) {
    const numId = typeof value === 'number' ? value : parseInt(value, 10);
    const found = items.find(item => item.id === numId);
    if (found) return found.label;
  }

  // Fallback: try matching by label (legacy string values)
  const strVal = String(value);
  const found = items.find(item => item.label === strVal);
  if (found) return found.label;

  return strVal;
}

/**
 * Resolve multiple option IDs/values to labels.
 * @param {string|null|undefined} optionsStr - JSON string of options
 * @param {Array<number|string>} values - Array of option IDs or legacy string values
 * @returns {string[]} Array of resolved labels
 */
export function resolveOptionLabels(optionsStr, values) {
  if (!Array.isArray(values)) return [];
  return values.map(v => resolveOptionLabel(optionsStr, v));
}

/**
 * Serialize option items back to the new JSON format for saving.
 * @param {number} nextId
 * @param {Array<{id: number, label: string}>} items
 * @returns {string} JSON string
 */
export function serializeOptions(nextId, items) {
  return JSON.stringify({ next_id: nextId, items });
}
