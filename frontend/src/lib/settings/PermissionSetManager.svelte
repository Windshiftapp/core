<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { navigate } from '../router.js';
  import { Plus, Edit, Trash2, Shield } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

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

  function handleModalKeydown(event) {
    // Enter to submit (only if not in textarea)
    if (event.key === 'Enter' && !event.shiftKey && !event.ctrlKey && !event.metaKey) {
      const target = event.target;
      if (target.tagName !== 'TEXTAREA') {
        event.preventDefault();
        createPermissionSet();
      }
    }
    // Escape to cancel
    if (event.key === 'Escape') {
      event.preventDefault();
      cancelCreate();
    }
  }

  async function loadPermissionSets() {
    try {
      loading = true;
      const data = await api.get('/permission-sets') || [];
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
        alert(t('validation.requiredField', { field: t('common.name') }));
        return;
      }

      const created = await api.post('/permission-sets', {
        name: formData.name,
        description: formData.description,
        permission_ids: []
      });
      permissionSets = [...permissionSets, created];
      cancelCreate();
    } catch (error) {
      console.error('Failed to create permission set:', error);
      alert(t('settings.permissionSets.failedToCreate') + (error.message || error));
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
      await api.delete(`/permission-sets/${permSet.id}`);
      permissionSets = permissionSets.filter(ps => ps.id !== permSet.id);
    } catch (error) {
      console.error('Failed to delete permission set:', error);
      alert(t('settings.permissionSets.failedToDelete') + (error.message || error));
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
        iconColor: '#dc2626',
        onClick: () => deletePermissionSet(permSet),
        color: '#dc2626'
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
  {#if showCreateModal}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="fixed inset-0 flex items-center justify-center p-4 z-50"
      style="background-color: rgba(0, 0, 0, 0.3); backdrop-filter: blur(2px);"
      onkeydown={handleModalKeydown}
    >
      <div class="rounded shadow-xl max-w-lg w-full p-6" style="background-color: var(--ds-surface-overlay)">
        <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text)">{t('settings.permissionSets.createPermissionSet')}</h3>

        <div class="space-y-4">
          <div>
            <Label for="permset-name" color="default" required class="mb-1">{t('common.name')}</Label>
            <!-- svelte-ignore a11y_autofocus -->
            <input
              type="text"
              id="permset-name"
              bind:value={formData.name}
              class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text)"
              placeholder={t('settings.permissionSets.namePlaceholder')}
              autofocus
            />
          </div>

          <div>
            <Label for="permset-description" color="default" class="mb-1">{t('common.description')}</Label>
            <Textarea
              id="permset-description"
              bind:value={formData.description}
              rows={2}
              placeholder={t('settings.permissionSets.descriptionPlaceholder')}
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
    </div>
  {/if}

  <DataTable
    data={permissionSets}
    {columns}
    {loading}
    actionItems={buildPermSetDropdownItems}
    emptyMessage={t('settings.permissionSets.noPermissionSets')}
  />
</div>
