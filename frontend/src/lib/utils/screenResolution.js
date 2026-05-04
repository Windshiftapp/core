/**
 * Resolve the screen id to use for an item in a given context.
 *
 * Fallback chain (most specific to least), where M is the requested mode
 * (`'create' | 'edit' | 'view'`):
 *   1. item-type override for mode M
 *   2. item-type override for create  (create_screen acts as the item-type's
 *      canonical screen if no mode-specific override exists)
 *   3. config-set default for mode M
 *   4. config-set default for create  (universal fallback — admins typically
 *      configure ONE screen and want it used everywhere)
 *
 * Returns null if nothing is configured at any level. Callers decide what
 * to do with null (typically: a hardcoded last-resort screen id, or skip
 * the screen filter entirely).
 *
 * @param {object|null|undefined} configSet  Configuration set with optional
 *   `create_screen_id`, `edit_screen_id`, `view_screen_id`, and
 *   `item_type_configs[]` (each with the same three optional fields).
 * @param {number|null|undefined} itemTypeId  Item type to look up an override for.
 * @param {'create'|'edit'|'view'} mode  The screen context.
 * @returns {number|null}
 */
export function resolveScreenId(configSet, itemTypeId, mode) {
  if (!configSet) return null;
  const modeKey = `${mode}_screen_id`;

  if (itemTypeId != null) {
    const itc = configSet.item_type_configs?.find((c) => c.item_type_id === itemTypeId);
    if (itc) {
      if (itc[modeKey]) return itc[modeKey];
      if (mode !== 'create' && itc.create_screen_id) return itc.create_screen_id;
    }
  }

  if (configSet[modeKey]) return configSet[modeKey];
  if (mode !== 'create' && configSet.create_screen_id) return configSet.create_screen_id;

  return null;
}
