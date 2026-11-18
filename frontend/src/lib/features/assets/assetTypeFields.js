/**
 * Load and deduplicate field metadata for many asset types with bounded
 * concurrency. Scheduling stops as soon as `isStale()` turns true, so a
 * context switch does not keep firing requests for the previous set.
 *
 * Field order follows the type order and the first occurrence of each
 * `custom_field_id` wins, matching the previous serial loader's output.
 *
 * @param {Array<{ id: number }>} types
 * @param {(typeId: number) => Promise<any[]>} getFields
 * @param {{ concurrency?: number, isStale?: () => boolean }} [options]
 * @returns {Promise<any[]>}
 */
export async function loadAssetTypeFields(types, getFields, options = {}) {
  const concurrency = Math.max(1, options.concurrency ?? 4);
  const isStale = options.isStale ?? (() => false);
  if (types.length === 0) return [];

  const results = new Array(types.length);
  let nextIndex = 0;

  const worker = async () => {
    while (true) {
      if (isStale()) return;
      const index = nextIndex;
      nextIndex += 1;
      if (index >= types.length) return;
      results[index] = await getFields(types[index].id);
    }
  };

  await Promise.all(Array.from({ length: Math.min(concurrency, types.length) }, () => worker()));

  const fields = [];
  const seenFieldIds = new Set();
  for (const typeFields of results) {
    for (const field of typeFields || []) {
      if (!seenFieldIds.has(field.custom_field_id)) {
        seenFieldIds.add(field.custom_field_id);
        fields.push(field);
      }
    }
  }
  return fields;
}

/**
 * Build the filter-picker identifier for an asset custom field. The stable
 * numeric id keeps two fields that share a name distinct; the name form is
 * only a fallback for legacy rows without an id.
 *
 * @param {{ custom_field_id?: number, field_name?: string, name?: string }} field
 * @returns {string}
 */
export function assetCustomFieldIdentifier(field) {
  return field.custom_field_id
    ? `cfid_${field.custom_field_id}`
    : `cf_${field.field_name || field.name}`;
}
