/**
 * Parent-reference helpers shared by the parent chip, board cards, and the
 * collection tree.
 *
 * The server sets `parent_key` to the parent's permission-checked key whenever
 * it keeps `parent_workspace_item_number`, and clears both when the parent is
 * not visible. There is no workspace-relative shape to derive, so a missing
 * key means the parent must not render.
 */

/**
 * @param {{ parent_key?: string }|null|undefined} item
 * @returns {string}
 */
export function resolveParentKey(item) {
  return item?.parent_key || '';
}

/**
 * Whether the item carries a parent reference worth rendering. A masked parent
 * keeps `parent_id` but drops the key and title, so it must not render.
 *
 * @param {{ parent_id?: number|null, parent_title?: string, parent_key?: string }|null|undefined} item
 * @returns {boolean}
 */
export function hasParentReference(item) {
  if (item?.parent_id == null) return false;
  return Boolean(resolveParentKey(item) || item.parent_title);
}
