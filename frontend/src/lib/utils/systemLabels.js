import { t } from '../stores/i18n.svelte.js';

const SYSTEM_ROLE_KEYS = {
  Viewer: 'viewer',
  Editor: 'editor',
  Tester: 'tester',
  Administrator: 'administrator',
};

const SYSTEM_STATUS_KEYS = {
  Open: 'open',
  'In Progress': 'inProgress',
  Done: 'done',
};

const SYSTEM_STATUS_CATEGORY_KEYS = {
  'To Do': 'toDo',
  'In Progress': 'inProgress',
  Done: 'done',
};

const SYSTEM_PRIORITY_KEYS = {
  Critical: 'critical',
  High: 'high',
  Medium: 'medium',
  Low: 'low',
  Lowest: 'lowest',
};

const SYSTEM_PERMISSION_KEYS = new Set([
  'system.admin',
  'workspace.create',
  'milestone.create',
  'iteration.manage',
  'user.list',
  'asset.manage',
  'customers.manage',
  'project.manage',
  'workspace.admin',
  'item.view',
  'item.create',
  'item.edit',
  'item.delete',
  'item.comment',
  'comment.edit_others',
  'test.view',
  'test.execute',
  'test.manage',
  'action.manage',
  'action.credential.manage',
  'action.set_actor',
  'teams.manage',
  'public_board.manage',
]);

function permissionLocaleKey(permissionKey) {
  return permissionKey.replaceAll('.', '_');
}

export function systemRoleName(role) {
  const key = role?.is_system ? SYSTEM_ROLE_KEYS[role.name] : null;
  return key ? t(`workspaceMembers.roles.${key}.name`) : role?.name;
}

export function systemRoleDescription(role) {
  const key = role?.is_system ? SYSTEM_ROLE_KEYS[role.name] : null;
  return key ? t(`workspaceMembers.roles.${key}.description`) : role?.description;
}

export function systemStatusName(status) {
  const name = typeof status === 'string' ? status : status?.name;
  const key = SYSTEM_STATUS_KEYS[name];
  return key ? t(`systemCatalog.statuses.${key}`) : name;
}

export function systemStatusCategoryName(category) {
  const name = typeof category === 'string' ? category : category?.name;
  const key = SYSTEM_STATUS_CATEGORY_KEYS[name];
  return key ? t(`systemCatalog.statusCategories.${key}`) : name;
}

export function systemPriorityName(priority) {
  const name = typeof priority === 'string' ? priority : priority?.name;
  const key = SYSTEM_PRIORITY_KEYS[name];
  return key ? t(`priorities.${key}`) : name;
}

export function systemPermissionName(permission) {
  const key = permission?.permission_key;
  return SYSTEM_PERMISSION_KEYS.has(key)
    ? t(`systemCatalog.permissions.${permissionLocaleKey(key)}.name`)
    : permission?.permission_name;
}

export function systemPermissionDescription(permission) {
  const key = permission?.permission_key;
  return SYSTEM_PERMISSION_KEYS.has(key)
    ? t(`systemCatalog.permissions.${permissionLocaleKey(key)}.description`)
    : permission?.description;
}
