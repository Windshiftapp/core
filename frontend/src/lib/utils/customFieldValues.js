/**
 * Read a custom-field value from a CFV map. The numeric field-id key is
 * canonical; the legacy name key is the downgrade mirror (WI-2025). Presence is
 * checked explicitly rather than with `??`, so a deliberately cleared field
 * (explicit null/empty id key) does not fall back to a stale name value.
 */
export function fieldValue(cfv, field) {
  if (!cfv || !field) return undefined;
  const key = String(field.id);
  if (Object.hasOwn(cfv, key)) return cfv[key];
  return cfv[field.name];
}

/**
 * Re-key a stored CFV map to numeric field ids for a form draft. Legacy name
 * keys are dropped; an ambiguous name is copied onto every matching field so no
 * value is lost. The resulting map is what the API expects on write.
 */
export function toIdKeyedCustomFieldValues(cfv, fields) {
  const out = {};
  for (const field of fields ?? []) {
    const value = fieldValue(cfv, field);
    if (value !== undefined) out[String(field.id)] = value;
  }
  return out;
}
