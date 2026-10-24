import { groups as groupsApi, permissions as permissionsApi } from '../api/permissions.js';
import { getUsers } from '../api/users.js';
/** Global permissions that the permission manager can assign to users and groups. */
export function selectGlobalPermissions(permissions) {
  return (permissions || []).filter((permission) => permission?.scope === 'global');
}

/** Build a subject-id → permission-id Set map from compact assignment rows. */
export function buildPermissionAssignmentMap(rows, subjectKey) {
  const assignments = new Map();
  if (!Array.isArray(rows)) return assignments;

  for (const row of rows) {
    const subjectId = row?.[subjectKey];
    const permissionId = row?.permission_id;
    if (subjectId == null || permissionId == null) continue;
    if (!assignments.has(subjectId)) assignments.set(subjectId, new Set());
    assignments.get(subjectId).add(permissionId);
  }
  return assignments;
}

/**
 * Load the permission-manager request graph without per-user requests.
 * @param {{ permissions?: any[] }} [options]
 */
export async function loadPermissionManagerData(options = {}) {
  const seededPermissions = options.permissions;
  const [permissions, users, groups, userGrants, groupGrants] = await Promise.all([
    Array.isArray(seededPermissions) ? Promise.resolve(seededPermissions) : permissionsApi.getAll(),
    getUsers(),
    groupsApi.getAll(),
    permissionsApi.getAllUserGlobalPermissions(),
    permissionsApi.getAllGroupPermissions(),
  ]);

  return {
    permissions: Array.isArray(permissions) ? permissions : [],
    users: Array.isArray(users) ? users : [],
    groups: Array.isArray(groups) ? groups : [],
    userPermissions: buildPermissionAssignmentMap(userGrants, 'user_id'),
    groupPermissions: buildPermissionAssignmentMap(groupGrants, 'group_id'),
  };
}
