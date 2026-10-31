// Scope and edit rights of a milestone on its detail page.
// Both come from the milestone record (`GET /milestones/{id}`): the progress
// response (`GET /milestones/{id}/progress`) carries neither `is_global` nor
// `workspace_id`.

/** Workspace id of a workspace milestone; null for global or missing ones. */
export function milestoneWorkspaceId(milestone) {
  if (!milestone || milestone.is_global) return null;
  return milestone.workspace_id ?? null;
}

/**
 * Whether the user may edit, release, delete, and link pages to the
 * milestone. Mirrors the server rule: global milestones need the global
 * `milestone.create` permission; workspace milestones need `item.edit` (or
 * workspace admin) in the milestone's own workspace.
 *
 * @param {object | null} milestone milestone record
 * @param {{ isSystemAdmin?: boolean, hasGlobalPermission: (key: string) => boolean, hasWorkspacePermission: (workspaceId: number, key: string) => boolean }} access
 */
export function canManageMilestone(milestone, access) {
  if (!milestone) return false;
  if (access.isSystemAdmin) return true;
  if (milestone.is_global) return access.hasGlobalPermission('milestone.create');
  const workspaceId = milestoneWorkspaceId(milestone);
  if (workspaceId == null) return false;
  return (
    access.hasWorkspacePermission(workspaceId, 'workspace.admin') ||
    access.hasWorkspacePermission(workspaceId, 'item.edit')
  );
}

/**
 * Whether a milestone list row shows the drag grip: global rows
 * need the global reorder right, workspace rows the local one. The caller
 * computes both rights for the page; on the global page `canReorderLocal` is
 * false, because workspace milestones are reordered on their own workspace page.
 *
 * @param {object | null} milestone milestone row
 * @param {{ canReorderGlobal: boolean, canReorderLocal: boolean }} rights
 */
export function canReorderMilestoneRow(milestone, rights) {
  if (!milestone) return false;
  return milestone.is_global ? rights.canReorderGlobal : rights.canReorderLocal;
}
