<script>
  import { del, get, post } from '../api/core.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { navigate } from '../router.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Shield from '@lucide/svelte/icons/shield';
  import TextareaField from '../components/TextareaField.svelte';
  import TextField from '../components/TextField.svelte';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let formData = $state({ name: '', description: '' });

  async function loadPermissionSets() {
    return (await get('/permission-sets')) || [];
  }

  function openCreate() {
    formData = { name: '', description: '' };
  }

  class ValidationError extends Error {}

  async function createPermissionSet() {
    if (!formData.name.trim()) {
      errorToast(t('validation.requiredField', { field: t('common.name') }));
      throw new ValidationError();
    }
    await post('/permission-sets', {
      name: formData.name,
      description: formData.description,
      permission_ids: [],
    });
  }

  async function deletePermissionSet(permSet) {
    await del(`/permission-sets/${permSet.id}`);
  }

  function handleError(err, phase) {
    if (err instanceof ValidationError) return true;
    const prefix =
      phase === 'delete'
        ? t('settings.permissionSets.failedToDelete')
        : t('settings.permissionSets.failedToCreate');
    errorToast(prefix + (err?.message || err));
    return true;
  }

  const columns = [
    { key: 'name', label: t('common.name'), sortable: true },
    { key: 'description', label: t('common.description') },
    {
      key: 'is_system',
      label: t('common.type'),
      render: (item) =>
        item.is_system
          ? t('settings.permissionSets.system')
          : t('settings.permissionSets.custom'),
      sortable: true,
    },
    { key: 'actions', label: '', width: 'w-16' },
  ];

  function buildPermSetDropdownItems(permSet, { remove }) {
    const items = [
      {
        id: 'edit',
        title: t('common.edit'),
        icon: Edit,
        iconColor: '#3b82f6',
        onClick: () => navigate(`/admin/permission-sets/${permSet.id}`),
      },
    ];

    if (!permSet.is_system) {
      items.push({
        id: 'delete',
        title: t('common.delete'),
        icon: Trash2,
        iconColor: 'var(--ds-icon-danger)',
        color: 'var(--ds-text-danger)',
        onClick: () => remove(permSet),
      });
    }

    return items;
  }
</script>

<EntityManagerPage
  icon={Shield}
  title={t('settings.permissionSets.title')}
  subtitle={t('settings.permissionSets.subtitle')}
  addLabel={t('settings.permissionSets.createPermissionSet')}
  hotkeyScope="permissionSets"
  emptyMessage={t('settings.permissionSets.noPermissionSets')}
  columns={columns}
  load={loadPermissionSets}
  create={createPermissionSet}
  remove={deletePermissionSet}
  onOpenCreate={openCreate}
  confirmDeleteMessage={(permSet) =>
    `${t('settings.permissionSets.confirmDelete')} "${permSet.name}"? ${t('common.cannotUndo')}`}
  onError={handleError}
  actionItems={buildPermSetDropdownItems}
  formFields={permissionSetForm}
>
</EntityManagerPage>

{#snippet permissionSetForm()}
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
{/snippet}
