<script>
  import { actionCredentials } from '../api/ai.js';
  import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
  import ActionCredentialsManager from '../settings/ActionCredentialsManager.svelte';

  let { workspaceId = null } = $props();

  const canManage = $derived(
    workspacePermissions.hasPermission(workspaceId, 'action.credential.manage')
  );

  const api = {
    list: () => actionCredentials.getForWorkspace(workspaceId),
    create: (data) => actionCredentials.createForWorkspace(workspaceId, data),
    update: (id, data) => actionCredentials.updateForWorkspace(workspaceId, id, data),
    rotate: (id, secret) => actionCredentials.rotateForWorkspace(workspaceId, id, secret),
    remove: (id) => actionCredentials.deleteForWorkspace(workspaceId, id),
  };

  const i18n = {
    add: 'settings.adminOperations.actionCredentials.add',
    empty: 'workspaceSettings.actionCredentials.empty',
    addFirst: 'settings.adminOperations.actionCredentials.addFirst',
    description: 'workspaceSettings.actionCredentials.description',
    noPermissionTitle: 'workspaceSettings.actionCredentials.noPermissionTitle',
    noPermissionBody: 'workspaceSettings.actionCredentials.noPermissionBody',
  };

  const testids = {
    add: 'workspace-credential-add',
    empty: 'workspace-credentials-empty',
    accessDenied: 'workspace-credentials-access-denied',
    nameInput: 'workspace-credential-name-input',
    secretInput: 'workspace-credential-secret-input',
    rotateInput: 'workspace-credential-rotate-input',
    rowPrefix: 'workspace-credential',
  };
</script>

<ActionCredentialsManager
  mode="workspace"
  {workspaceId}
  {canManage}
  {api}
  {i18n}
  {testids}
/>
