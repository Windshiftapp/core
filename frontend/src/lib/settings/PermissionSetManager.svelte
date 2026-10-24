<script>
  import { onMount } from 'svelte';
  import { del, get, post } from '../api/core.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { navigate } from '../router.js';
  import Plus from '@lucide/svelte/icons/plus';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Shield from '@lucide/svelte/icons/shield';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
	import TextareaField from '../components/TextareaField.svelte';
	import TextField from '../components/TextField.svelte';

  let permissionSets = $state([]);
  let loading = $state(true);
  let showCreateModal = $state(false);

  // Form state for create modal
  let formData = $state({
    name: '',
    description: ''
  });

  onMount(async () => {
    await loadPermissionSets();
  });

  async function loadPermissionSets() {
    try {
      loading = true;
      const data = await get('/permission-sets') || [];
      permissionSets = data;
    } catch (error) {
      console.error('Failed to load permission sets:', error);
      permissionSets = [];
    } finally {
      loading = false;
    }
  }

  function startCreate() {
    formData = {
      name: '',
      description: ''
    };
    showCreateModal = true;
  }

  function startEdit(permSet) {
    navigate(`/admin/permission-sets/${permSet.id}`);
  }

  function cancelCreate() {
    showCreateModal = false;
    formData = {
      name: '',
      description: ''
    };
  }

  async function createPermissionSet() {
    try {
      if (!formData.name.trim()) {
        errorToast(t('validation.requiredField', { field: t('common.name') }));
        return;
      }

      const created = await post('/permission-sets', {
        name: formData.name,
        description: formData.description,
        permission_ids: []
      });
      permissionSets = [...permissionSets, created];
      cancelCreate();
    } catch (error) {
      console.error('Failed to create permission set:', error);
      errorToast(t('settings.permissionSets.failedToCreate') + (error.message || error));
    }
  }

  async function deletePermissionSet(permSet) {
    const confirmed = await confirm({
      title: t('settings.permissionSets.deletePermissionSet'),
      message: t('settings.permissionSets.confirmDelete') + ` "${permSet.name}"? ` + t('common.cannotUndo'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
      icon: Trash2
    });

    if (!confirmed) return;

    try {
      await del(`/permission-sets/${permSet.id}`);
      permissionSets = permissionSets.filter(ps => ps.id !== permSet.id);
    } catch (error) {
      console.error('Failed to delete permission set:', error);
      errorToast(t('settings.permissionSets.failedToDelete') + (error.message || error));
    }
  }

  function buildPermSetDropdownItems(permSet) {
    const items = [
      {
        id: 'edit',
        title: t('common.edit'),
        icon: Edit,
        iconColor: '#3b82f6',
        onClick: () => startEdit(permSet)
      }
    ];

    if (!permSet.is_system) {
      items.push({
        id: 'delete',
        title: t('common.delete'),
        icon: Trash2,
        iconColor: 'var(--ds-icon-danger)',
        onClick: () => deletePermissionSet(permSet),
        color: 'var(--ds-text-danger)'
      });
    }

    return items;
  }

  const columns = $derived([
    { key: 'name', label: t('common.name'), sortable: true },
    { key: 'description', label: t('common.description') },
    {
      key: 'is_system',
      label: t('common.type'),
      render: (item) => item.is_system ? t('settings.permissionSets.system') : t('settings.permissionSets.custom'),
      sortable: true
    },
    { key: 'actions', label: '', width: 'w-16' }
  ]);
</script>

<div class="space-y-6">
  <PageHeader
    title={t('settings.permissionSets.title')}
    subtitle={t('settings.permissionSets.subtitle')}
    icon={Shield}
  >
    {#snippet actions()}
      <Button onclick={startCreate} size="sm" variant="primary" keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('permissionSets', 'add'), guard: () => !showCreateModal }}>
        <Plus class="w-4 h-4 mr-2" />
        {t('settings.permissionSets.createPermissionSet')}
      </Button>
    {/snippet}
  </PageHeader>

  <!-- Create Modal -->
  <Modal
    bind:isOpen={showCreateModal}
    maxWidth="max-w-lg"
    onclose={cancelCreate}
    onSubmit={createPermissionSet}
  >
    <div class="p-6">
      <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text)">{t('settings.permissionSets.createPermissionSet')}</h3>
      <div class="space-y-4">
          <div>
            <TextField
              label={t('common.name')}
              id="permset-name"
              required
              labelColor="default"
              type="text"
              placeholder={t('settings.permissionSets.namePlaceholder')}
              size="small"
              bind:value={formData.name}
            />
          </div>

          <div>
            <TextareaField
              label={t('common.description')}
              id="permset-description"
              labelColor="default"
              rows={2}
              placeholder={t('settings.permissionSets.descriptionPlaceholder')}
              bind:value={formData.description}
            />
          </div>
        </div>

        
      <div class="flex justify-end space-x-3 mt-6">
        <Button variant="secondary" onclick={cancelCreate} keyboardHint="Esc">
          {t('common.cancel')}
        </Button>
        <Button variant="primary" onclick={createPermissionSet} keyboardHint="↵">
          {t('common.create')}
        </Button>
      </div>
    </div>
  </Modal>

  <DataTable
    data={permissionSets}
    {columns}
    {loading}
    actionItems={buildPermSetDropdownItems}
    emptyMessage={t('settings.permissionSets.noPermissionSets')}
  />
</div>
