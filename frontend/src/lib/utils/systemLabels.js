import { t } from '../stores/i18n.svelte.js';

export function builtinLocaleKey(recordOrKey) {
  const key = typeof recordOrKey === 'string' ? recordOrKey : recordOrKey?.builtin_key;
  return key?.replace(/_([a-z])/g, (_, letter) => letter.toUpperCase()) || '';
}

function permissionLocaleKey(permissionKey) {
  return permissionKey.replaceAll('.', '_');
}

export function systemRoleName(role) {
  const key = role?.is_system ? builtinLocaleKey(role) : '';
  return key ? t(`workspaceMembers.roles.${key}.name`) : role?.name;
}

export function systemRoleDescription(role) {
  const key = role?.is_system ? builtinLocaleKey(role) : '';
  return key ? t(`workspaceMembers.roles.${key}.description`) : role?.description;
}

export function systemStatusName(status) {
  const key = typeof status === 'object' ? builtinLocaleKey(status) : '';
  return key
    ? t(`systemCatalog.statuses.${key}`)
    : typeof status === 'string'
      ? status
      : status?.name;
}

export function systemStatusCategoryName(category) {
  const key = typeof category === 'object' ? builtinLocaleKey(category) : '';
  return key
    ? t(`systemCatalog.statusCategories.${key}`)
    : typeof category === 'string'
      ? category
      : category?.name;
}

export function systemPriorityName(priority) {
  const key = typeof priority === 'object' ? builtinLocaleKey(priority) : '';
  return key ? t(`priorities.${key}`) : typeof priority === 'string' ? priority : priority?.name;
}

export function systemPermissionName(permission) {
  const key = permission?.permission_key;
  return permission?.is_system && key
    ? t(`systemCatalog.permissions.${permissionLocaleKey(key)}.name`)
    : permission?.permission_name;
}

export function systemPermissionDescription(permission) {
  const key = permission?.permission_key;
  return permission?.is_system && key
    ? t(`systemCatalog.permissions.${permissionLocaleKey(key)}.description`)
    : permission?.description;
}
