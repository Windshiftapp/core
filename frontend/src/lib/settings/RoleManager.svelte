<script>
  import { workspaceRoles } from '../api/workspaces.js';
  import BadgeCheck from '@lucide/svelte/icons/badge-check';
  import Eye from '@lucide/svelte/icons/eye';
  import CheckCircle from '@lucide/svelte/icons/check-circle';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import AlertBox from '../components/AlertBox.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Button from '../components/Button.svelte';
  import TextareaField from '../components/TextareaField.svelte';
  import TextField from '../components/TextField.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import {
    systemPermissionDescription,
    systemPermissionName,
    objectDisplayDescription,
    objectDisplayName,
  } from '../utils/systemLabels.js';
  import LocalizedObjectFields from './LocalizedObjectFields.svelte';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let selectedRole = $state(null);
  let rolePermissions = $state([]);
  let translationEditor = $state(null);
  let savingTranslations = $state(false);

  let newName = $state('');
  let newDescription = $state('');

  const columns = [
    { key: 'name', label: t('roles.roleName'), sortable: true, slot: 'name' },
    { key: 'description', label: t('common.description'), slot: 'description' },
    {
      key: 'kind',
      label: t('common.type'),
      render: (item) => (item.is_system ? t('common.default') : t('common.custom')),
      sortable: false,
    },
    {
      key: 'permissions_enabled',
      label: '',
      render: (item) =>
        item.permissions_enabled
          ? t('settings.workspaceRoles.permissionBearingBadge')
          : t('settings.workspaceRoles.labelOnlyBadge'),
      sortable: false,
    },
    { key: 'actions', label: '', width: 'w-16' },
  ];

  async function loadRoles() {
    return (await workspaceRoles.getAll()) || [];
  }

  async function viewRoleDetails(role) {
    try {
      const fullRole = await workspaceRoles.get(role.id);
      selectedRole = fullRole;
      rolePermissions = fullRole.permissions || [];
    } catch (error) {
      errorToast(t('dialogs.alerts.failedToLoad', { error: error.message || error }));
    }
  }

  function closeDetails() {
    selectedRole = null;
    rolePermissions = [];
  }

  async function saveRoleTranslations() {
    try {
      savingTranslations = true;
      translationEditor?.validate();
      await workspaceRoles.update(selectedRole.id, {
        name: selectedRole.name,
        description: selectedRole.description,
      });
      await translationEditor?.save();
      selectedRole = await workspaceRoles.get(selectedRole.id);
      await loadRoles();
      successToast(t('common.saved'));
    } catch (error) {
      errorToast(error?.message || t('dialogs.alerts.failedToSave'));
    } finally {
      savingTranslations = false;
    }
  }

  function openCreate() {
    newName = '';
    newDescription = '';
  }

  async function createRole() {
    await workspaceRoles.create({ name: newName.trim(), description: newDescription });
    successToast(t('settings.workspaceRoles.addCustomTitle'));
  }

  async function deleteRole(role) {
    if (role.is_system) {
      errorToast(t('settings.workspaceRoles.cannotDeleteSystem'));
      return;
    }
    await workspaceRoles.delete(role.id);
    successToast(t('common.deleted'));
  }

  function handleError(err, phase) {
    if (phase === 'delete' && err?.isSystem) return true;
    errorToast(err?.message || JSON.stringify(err));
    return true;
  }

  function buildRoleDropdownItems(role, { remove }) {
    const items = [
      {
        id: 'view',
        title: t('common.view'),
        icon: Eye,
        onClick: () => viewRoleDetails(role),
      },
    ];
    if (!role.is_system) {
      items.push({
        id: 'delete',
        title: t('common.delete'),
        icon: Trash2,
        color: 'var(--ds-text-danger)',
        onClick: () => remove(role),
      });
    }
    return items;
  }
</script>

<EntityManagerPage
  icon={BadgeCheck}
  title={t('roles.title')}
  subtitle={t('roles.subtitle')}
  addLabel={t('settings.workspaceRoles.addCustom')}
  addTestid="workspace-role-add"
  hotkeyScope="workspaceRoles"
  emptyMessage={t('roles.noRoles')}
  columns={columns}
  load={loadRoles}
  create={createRole}
  remove={deleteRole}
  onOpenCreate={openCreate}
  confirmDeleteMessage={() => t('settings.workspaceRoles.deleteConfirm')}
  submitDisabled={() => !newName.trim()}
  confirmTestid="workspace-role-submit"
  onError={handleError}
  actionItems={buildRoleDropdownItems}
  cellSnippets={{ name: roleNameCell, description: roleDescriptionCell }}
  formFields={roleForm}
>
</EntityManagerPage>

<AlertBox variant="info" class="mt-6">
  <p class="text-sm">{t('settings.workspaceRoles.labelOnlyNotice')}</p>
</AlertBox>

{#snippet roleNameCell(item)}
  {objectDisplayName(item, 'workspace_role')}
{/snippet}

{#snippet roleDescriptionCell(item)}
  {objectDisplayDescription(item, 'workspace_role')}
{/snippet}

{#snippet roleForm()}
  <AlertBox variant="info">
    <p class="text-sm">{t('settings.workspaceRoles.labelOnlyNotice')}</p>
  </AlertBox>

  <div>
    <TextField
      label={t('roles.roleName')}
      required
      type="text"
      size="small"
      placeholder={t('settings.workspaceRoles.namePlaceholder')}
      dataTestid="workspace-role-name"
      bind:value={newName}
    />
  </div>
  <div>
    <TextareaField
      label={t('common.description')}
      placeholder={t('settings.workspaceRoles.descriptionPlaceholder')}
      rows={2}
      bind:value={newDescription}
    />
  </div>
{/snippet}

<!-- View / details modal -->
<Modal isOpen={selectedRole !== null} onclose={closeDetails} maxWidth="max-w-2xl">
  <ModalHeader
    title={objectDisplayName(selectedRole, 'workspace_role')}
    subtitle={objectDisplayDescription(selectedRole, 'workspace_role')}
    icon={BadgeCheck}
    onClose={closeDetails}
  />
  <div class="px-6 py-4">
    {#if selectedRole}
      {#key selectedRole.id}
        <LocalizedObjectFields
          bind:this={translationEditor}
          objectType="workspace_role"
          objectId={selectedRole.id}
          bind:canonicalName={selectedRole.name}
          bind:canonicalDescription={selectedRole.description}
          displayName={selectedRole.display_name || selectedRole.name}
          displayDescription={selectedRole.display_description || selectedRole.description}
        />
      {/key}
      <div class="mt-4 flex justify-end">
        <Button variant="primary" onclick={saveRoleTranslations} loading={savingTranslations}>
          {t('common.save')}
        </Button>
      </div>
    {/if}
    {#if selectedRole?.permissions_enabled}
      <h4 class="font-medium mb-3" style="color: var(--ds-text);">{t('roles.permissions')}</h4>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3 max-h-96 overflow-y-auto">
        {#each rolePermissions as permission}
          <div class="flex items-start space-x-2 p-3 rounded-md" style="background-color: var(--ds-interactive-subtle);">
            <CheckCircle class="w-5 h-5 mt-0.5 flex-shrink-0" style="color: var(--ds-text-success);" />
            <div>
              <div class="font-medium text-sm" style="color: var(--ds-text);">{systemPermissionName(permission)}</div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">{systemPermissionDescription(permission)}</div>
              <div class="text-xs mt-0.5" style="color: var(--ds-text-subtlest);">{permission.permission_key}</div>
            </div>
          </div>
        {/each}
      </div>
    {:else}
      <AlertBox variant="info">
        <p class="text-sm">{t('settings.workspaceRoles.labelOnlyNotice')}</p>
      </AlertBox>
    {/if}
  </div>
</Modal>
