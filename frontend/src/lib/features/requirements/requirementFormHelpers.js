import { getRequirementStarterContent } from './requirementStarterTemplates.js';

export const DEFAULT_REQUIREMENT_TYPE = 'use_case';
export const DEFAULT_REQUIREMENT_STATUS = 'draft';

/** @param {string} type @param {(key: string) => string} t */
export function applyStarterTemplate(type, t) {
  return getRequirementStarterContent(type, t);
}

/** @param {string} content @param {boolean} contentTouched */
export function shouldConfirmTemplateReplace(content, contentTouched) {
  return Boolean(content?.trim() && contentTouched);
}

/** @param {import('../../stores/workspacePermissions.svelte.js').workspacePermissions} workspacePermissions @param {number} workspaceId */
export function canCreateRequirement(workspacePermissions, workspaceId) {
  return (
    workspacePermissions.isSystemAdmin ||
    workspacePermissions.hasPermission(workspaceId, 'page.create') ||
    workspacePermissions.hasPermission(workspaceId, 'page.admin') ||
    workspacePermissions.hasPermission(workspaceId, 'workspace.admin')
  );
}

/** @param {import('../../stores/workspacePermissions.svelte.js').workspacePermissions} workspacePermissions @param {number} workspaceId */
export function canEditRequirement(workspacePermissions, workspaceId) {
  return (
    workspacePermissions.isSystemAdmin ||
    workspacePermissions.hasPermission(workspaceId, 'page.edit') ||
    workspacePermissions.hasPermission(workspaceId, 'page.admin') ||
    workspacePermissions.hasPermission(workspaceId, 'workspace.admin')
  );
}

/**
 * @param {import('../../stores/workspacePermissions.svelte.js').workspacePermissions} workspacePermissions
 * @param {Array<{ id: number }>} workspaces
 */
export function canCreateRequirementInAnyWorkspace(workspacePermissions, workspaces) {
  if (workspacePermissions.isSystemAdmin) return true;
  return workspaces.some((ws) => canCreateRequirement(workspacePermissions, ws.id));
}

/** @param {{ first_name?: string, last_name?: string, username?: string, email?: string }} user */
export function formatRequirementOwnerLabel(user) {
  if (!user) return '';
  const name = [user.first_name, user.last_name].filter(Boolean).join(' ');
  return name || user.username || user.email || '';
}
