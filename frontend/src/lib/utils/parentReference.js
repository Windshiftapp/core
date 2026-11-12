/**
 * Parent-reference helpers shared by the parent chip, board cards, and the
 * collection tree.
 *
 * The server populates `parent_key` with the parent's permission-checked key
 * and clears `parent_workspace_item_number`/`parent_title` when the parent is
 * not visible. Older hydrated payloads omit `parent_key`, so derive the key
 * from the child's workspace as a fallback.
 */

/**
 * @param {{ parent_key?: string, parent_workspace_item_number?: number|null, workspace_key?: string }|null|undefined} item
 * @param {string|null|undefined} workspaceKey
 * @returns {string} the parent key, or '' when no parent key is available
 */
export function resolveParentKey(item, workspaceKey = item?.workspace_key) {
  if (!item) return '';
  if (item.parent_key) return item.parent_key;
  if (item.parent_workspace_item_number != null && workspaceKey) {
    return `${workspaceKey}-${item.parent_workspace_item_number}`;
  }
  return '';
}

/**
 * Whether the item carries a parent reference worth rendering. A masked parent
 * keeps `parent_id` but drops the key and title, so it must not render.
 *
 * @param {{ parent_id?: number|null, parent_title?: string, parent_key?: string, parent_workspace_item_number?: number|null, workspace_key?: string }|null|undefined} item
 * @param {string|null|undefined} workspaceKey
 * @returns {boolean}
 */
export function hasParentReference(item, workspaceKey = item?.workspace_key) {
  if (item?.parent_id == null) return false;
  return Boolean(resolveParentKey(item, workspaceKey) || item.parent_title);
}
