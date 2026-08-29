/**
 * Parse custom field options from the canonical ID-based format or a legacy label array.
 * Canonical format: {"next_id": 5, "items": [{"id": 1, "label": "Critical"}, ...]}
 * @param {string|null|undefined} optionsStr - JSON string of options
 * @returns {{ nextId: number, items: Array<{id: number, label: string}> }}
 */
export function parseFieldOptions(optionsStr) {
  if (!optionsStr) return { nextId: 1, items: [] };

  try {
    const parsed = JSON.parse(optionsStr);

    if (Array.isArray(parsed) && parsed.every((label) => typeof label === 'string')) {
      return {
        nextId: parsed.length + 1,
        items: parsed.map((label, index) => ({ id: index + 1, label })),
      };
    }

    if (parsed && typeof parsed === 'object' && parsed.next_id && Array.isArray(parsed.items)) {
      return {
        nextId: parsed.next_id,
        items: parsed.items.map((item) => ({ id: item.id, label: item.label })),
      };
    }
  } catch {
    // ignore parse errors
  }

  return { nextId: 1, items: [] };
}

/**
 * Resolve a single option ID to its label.
 * @param {string|null|undefined} optionsStr - JSON string of options
 * @param {number|string} value - Option ID (numeric)
 * @returns {string} The label, or the value cast to string if not found
 */
export function resolveOptionLabel(optionsStr, value) {
  if (value === null || value === undefined || value === '') return '';

  const { items } = parseFieldOptions(optionsStr);

  const numId = typeof value === 'number' ? value : parseInt(value, 10);
  if (!Number.isNaN(numId)) {
    const found = items.find((item) => item.id === numId);
    if (found) return found.label;
  }

  return String(value);
}

/**
 * Resolve multiple option IDs/values to labels.
 * @param {string|null|undefined} optionsStr - JSON string of options
 * @param {Array<number|string>} values - Array of option IDs
 * @returns {string[]} Array of resolved labels
 */
export function resolveOptionLabels(optionsStr, values) {
  if (!Array.isArray(values)) return [];
  return values.map((v) => resolveOptionLabel(optionsStr, v));
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
