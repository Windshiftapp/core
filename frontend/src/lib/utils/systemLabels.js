import { t } from '../stores/i18n.svelte.js';

function builtinLocaleKey(recordOrKey) {
  const key = typeof recordOrKey === 'string' ? recordOrKey : recordOrKey?.builtin_key;
  return key?.replace(/_([a-z])/g, (_, letter) => letter.toUpperCase()) || '';
}

function objectLocalePath(objectType, key, field) {
  if (!key) return '';
  if (objectType === 'workspace_role') return `workspaceMembers.roles.${key}.${field}`;
  if (field !== 'name') return '';
  if (objectType === 'status') return `systemCatalog.statuses.${key}`;
  if (objectType === 'status_category') return `systemCatalog.statusCategories.${key}`;
  if (objectType === 'priority') return `priorities.${key}`;
  return '';
}

export function objectDisplayValue(record, field = 'name', objectType = '') {
  if (typeof record === 'string') return field === 'name' ? record : '';
  if (!record || typeof record !== 'object') return '';
  const displayField = field === 'name' ? 'display_name' : `display_${field}`;
  if (record[displayField]) return record[displayField];
  const localePath = objectLocalePath(objectType, builtinLocaleKey(record), field);
  return localePath ? t(localePath) : record[field] || '';
}

export function objectDisplayName(record, objectType = '') {
  return objectDisplayValue(record, 'name', objectType);
}

export function objectDisplayDescription(record, objectType = '') {
  return objectDisplayValue(record, 'description', objectType);
}

function permissionLocaleKey(permissionKey) {
  return permissionKey.replaceAll('.', '_');
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
