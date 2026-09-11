// Keep admin settings out of the initial admin bundle. Each tab is loaded only
// when its route is active.
export const ADMIN_COMPONENT_LOADERS = {
  'custom-fields': () => import('../settings/CustomFields.svelte'),
  workspaces: () => import('../workspaces/Workspaces.svelte'),
  screens: () => import('../pages/Screens.svelte'),
  statuses: () => import('../features/workflows/StatusContainer.svelte'),
  workflows: () => import('../features/workflows/WorkflowBuilder.svelte'),
  'configuration-sets': () => import('../settings/ConfigurationSetManager.svelte'),
  'condition-sets': () => import('../settings/ConditionSetManager.svelte'),
  'approval-sets': () => import('../settings/ApprovalSetManager.svelte'),
  'notification-settings': () => import('../settings/NotificationSettings.svelte'),
  'email-templates': () => import('../settings/EmailTemplateManager.svelte'),
  channels: () => import('../features/channels/Channels.svelte'),
  'link-types': () => import('../settings/LinkTypeManager.svelte'),
  users: () => import('../settings/UserManager.svelte'),
  groups: () => import('../settings/GroupManager.svelte'),
  permissions: () => import('../layout/PermissionsContainer.svelte'),
  'workspace-roles': () => import('../settings/RoleManager.svelte'),
  attachments: () => import('../settings/AttachmentSettings.svelte'),
  modules: () => import('../settings/ModuleSettings.svelte'),
  themes: () => import('../settings/ThemeManager.svelte'),
  'hierarchy-levels': () => import('../settings/HierarchyLevelManager.svelte'),
  'item-types': () => import('../settings/ItemTypeManager.svelte'),
  priorities: () => import('../settings/PriorityManager.svelte'),
  sso: () => import('../settings/SSOContainer.svelte'),
  'scm-providers': () => import('../settings/SCMProviderManager.svelte'),
  'integration-providers': () => import('../settings/IntegrationsManager.svelte'),
  'llm-connections': () => import('../settings/AIContainer.svelte'),
  'action-capabilities': () => import('../settings/ActionCapabilitiesManager.svelte'),
  'system-import': () => import('../jira-import/SystemImportPage.svelte'),
  security: () => import('../settings/SecuritySettings.svelte'),
  assets: () => import('../features/assets/AssetManager.svelte'),
  diagnostics: () => import('../settings/Diagnostics.svelte'),
  'permission-set-detail': () => import('../settings/PermissionSetEdit.svelte'),
  'configuration-set-detail': () => import('../settings/ConfigurationSetDetail.svelte'),
  'condition-set-detail': () => import('../settings/ConditionSetDetail.svelte'),
  'approval-set-detail': () => import('../settings/ApprovalSetDetail.svelte'),
  'form-channel': () => import('../features/channels/FormChannelPage.svelte'),
  'portal-channel': () => import('../features/channels/PortalChannelPage.svelte'),
};

export function getAdminComponentProps(componentKey, loadedExtensions) {
  if (componentKey === 'workspaces') return { noPadding: true };
  if (componentKey === 'channels') return { embedded: true };
  if (componentKey === 'sso') return { extensions: loadedExtensions };
  return {};
}
