<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { BadgeCheck, Eye, CheckCircle, Plus, Trash2 } from 'lucide-svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Button from '../components/Button.svelte';
  import Label from '../components/Label.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Textarea from '../components/Textarea.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  let roles = $state([]);
  let loading = $state(true);
  let selectedRole = $state(null);
  let rolePermissions = $state([]);

  // Add-custom-role modal state
  let creating = $state(false);
  let creatingBusy = $state(false);
  let newName = $state('');
  let newDescription = $state('');

  const columns = $derived([
    { key: 'name', label: t('roles.roleName'), sortable: true },
    { key: 'description', label: t('common.description') },
    {
      key: 'kind',
      label: t('common.type'),
      // Combined column: system / custom + permissions-enabled flag.
      // Rendered via the renderBadge snippet on each row.
      render: (item) => item.is_system ? t('common.default') : t('common.custom'),
      sortable: false
    },
    {
      key: 'permissions_enabled',
      label: '',
      render: (item) =>
        item.permissions_enabled
          ? t('settings.workspaceRoles.permissionBearingBadge')
          : t('settings.workspaceRoles.labelOnlyBadge'),
      sortable: false
    },
    { key: 'actions', label: '', width: 'w-16' }
  ]);

  onMount(async () => {
    await loadRoles();
  });

  async function loadRoles() {
    try {
      loading = true;
      const data = await api.workspaceRoles.getAll();
      roles = data || [];
    } catch (error) {
      console.error('Failed to load workspace roles:', error);
      roles = [];
    } finally {
      loading = false;
    }
  }

  async function viewRoleDetails(role) {
    try {
      const fullRole = await api.workspaceRoles.get(role.id);
      selectedRole = fullRole;
      rolePermissions = fullRole.permissions || [];
    } catch (error) {
      console.error('Failed to load role details:', error);
      errorToast(t('dialogs.alerts.failedToLoad', { error: error.message || error }));
    }
  }

  function closeDetails() {
    selectedRole = null;
    rolePermissions = [];
  }

  function startCreating() {
    newName = '';
    newDescription = '';
    creating = true;
  }

  function closeCreate() {
    creating = false;
    newName = '';
    newDescription = '';
  }

  async function submitCreate() {
    const name = newName.trim();
    if (!name) {
      errorToast(t('settings.workspaceRoles.nameRequired'));
      return;
    }
    try {
      creatingBusy = true;
      await api.workspaceRoles.create({ name, description: newDescription });
      successToast(t('settings.workspaceRoles.addCustomTitle'));
      closeCreate();
      await loadRoles();
    } catch (error) {
      console.error('Failed to create custom role:', error);
      errorToast(error.message || JSON.stringify(error));
    } finally {
      creatingBusy = false;
    }
  }

  async function deleteRole(role) {
    if (role.is_system) {
      errorToast(t('settings.workspaceRoles.cannotDeleteSystem'));
      return;
    }
    const ok = await confirm({
      title: t('common.delete'),
      message: t('settings.workspaceRoles.deleteConfirm'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.workspaceRoles.delete(role.id);
      successToast(t('common.deleted'));
      await loadRoles();
    } catch (error) {
      console.error('Failed to delete role:', error);
      errorToast(error.message || JSON.stringify(error));
    }
  }

  function buildRoleDropdownItems(role) {
    const items = [
      {
        id: 'view',
        title: t('common.view'),
        icon: Eye,
        onClick: () => viewRoleDetails(role)
      }
    ];
    if (!role.is_system) {
      items.push({
        id: 'delete',
        title: t('common.delete'),
        icon: Trash2,
        color: 'var(--ds-text-danger)',
        onClick: () => deleteRole(role)
      });
    }
    return items;
  }
</script>

{#snippet headerActions()}
  <Button
    variant="primary"
    icon={Plus}
    onclick={startCreating}
    keyboardHint="A"
    hotkeyConfig={{ key: toHotkeyString('workspaceRoles', 'add') }}
    dataTestid="workspace-role-add"
  >
    {t('settings.workspaceRoles.addCustom')}
  </Button>
{/snippet}

<div class="space-y-6">
  <PageHeader
    title={t('roles.title')}
    description={t('roles.subtitle')}
    icon={BadgeCheck}
    actions={headerActions}
  />

  <!-- View / details modal -->
  <Modal
    isOpen={selectedRole !== null}
    onclose={closeDetails}
    maxWidth="max-w-2xl"
  >
    <ModalHeader
      title={selectedRole?.name}
      subtitle={selectedRole?.description}
      icon={BadgeCheck}
      onClose={closeDetails}
    />
    <div class="px-6 py-4">
      {#if selectedRole?.permissions_enabled}
        <h4 class="font-medium mb-3" style="color: var(--ds-text);">{t('roles.permissions')}</h4>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3 max-h-96 overflow-y-auto">
          {#each rolePermissions as permission}
            <div class="flex items-start space-x-2 p-3 rounded-md" style="background-color: var(--ds-interactive-subtle);">
              <CheckCircle class="w-5 h-5 mt-0.5 flex-shrink-0" style="color: var(--ds-text-success);" />
              <div>
                <div class="font-medium text-sm" style="color: var(--ds-text);">{permission.permission_name}</div>
                <div class="text-xs" style="color: var(--ds-text-subtle);">{permission.description}</div>
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

  <!-- Add custom role modal. Modal handles Esc-to-close + Cmd/Ctrl+Enter via
       onSubmit; the submitHint flows to DialogFooter so the user sees the
       chip on the confirm button. -->
  <Modal
    isOpen={creating}
    onclose={closeCreate}
    maxWidth="max-w-md"
    onSubmit={submitCreate}
    submitDisabled={creatingBusy || !newName.trim()}
  >
    {#snippet children(submitHint)}
      <ModalHeader
        title={t('settings.workspaceRoles.addCustomTitle')}
        icon={BadgeCheck}
        onClose={closeCreate}
      />
      <div class="px-6 py-4 space-y-4">
        <AlertBox variant="info">
          <p class="text-sm">{t('settings.workspaceRoles.labelOnlyNotice')}</p>
        </AlertBox>

        <div>
          <Label required>{t('roles.roleName')}</Label>
          <input
            type="text"
            class="w-full px-3 py-2 border rounded text-sm"
            style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text);"
            placeholder={t('settings.workspaceRoles.namePlaceholder')}
            bind:value={newName}
            data-testid="workspace-role-name"
          />
        </div>
        <div>
          <Label>{t('common.description')}</Label>
          <Textarea
            placeholder={t('settings.workspaceRoles.descriptionPlaceholder')}
            bind:value={newDescription}
            rows={2}
          />
        </div>
      </div>
      <DialogFooter
        onCancel={closeCreate}
        onConfirm={submitCreate}
        confirmLabel={t('common.create')}
        loading={creatingBusy}
        loadingLabel={t('common.saving')}
        disabled={!newName.trim()}
        showKeyboardHint={true}
        confirmKeyboardHint={submitHint}
        confirmTestid="workspace-role-submit"
      />
    {/snippet}
  </Modal>

  <DataTable
    data={roles}
    {columns}
    {loading}
    actionItems={buildRoleDropdownItems}
    emptyMessage={t('roles.noRoles')}
  />

  <AlertBox variant="info">
    <p class="text-sm">
      {t('settings.workspaceRoles.labelOnlyNotice')}
    </p>
  </AlertBox>
</div>
