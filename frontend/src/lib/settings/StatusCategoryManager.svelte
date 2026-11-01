<script>
  import { statusCategories as statusCategoriesApi, statuses as statusesApi } from '../api/workflows.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Palette from '@lucide/svelte/icons/palette';
  import Folder from '@lucide/svelte/icons/folder';
  import ColorDot from '../components/ColorDot.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { objectDisplayValue } from '../utils/systemLabels.js';
  import StatusCategoryModal from '../dialogs/StatusCategoryModal.svelte';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let editingId = $state(null);
  let editingObject = $state(null);
  let translationEditor = $state(null);

  let formData = $state({
    name: '',
    color: '#3b82f6',
    description: '',
    is_default: false,
    is_completed: false,
  });

  function getCategoryDisplayValue(category, field) {
    return objectDisplayValue(category, field);
  }

  async function loadStatusCategories() {
    const [categories, statuses] = await Promise.all([
      statusCategoriesApi.getAll() || [],
      statusesApi.getAll() || [],
    ]);
    return categories.map((category) => ({
      ...category,
      statusCount: statuses.filter((status) => status.category_id === category.id).length,
    }));
  }

  function openCreate() {
    formData = {
      name: '',
      color: '#ef4444',
      description: '',
      is_default: false,
      is_completed: false,
    };
    editingId = null;
    editingObject = null;
  }

  function openEdit(category) {
    editingObject = category;
    formData = {
      name: category.name || '',
      color: category.color || '#3b82f6',
      description: category.description || '',
      is_default: category.is_default || false,
      is_completed: category.is_completed || false,
    };
    editingId = category.id;
  }

  async function saveCategory() {
    if (editingId) {
      translationEditor?.validate();
      await statusCategoriesApi.update(editingId, formData);
      await translationEditor?.save();
    } else {
      await statusCategoriesApi.create(formData);
    }
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }

  async function deleteCategory(category) {
    await statusCategoriesApi.delete(category.id);
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }

  function handleError(err, phase, item) {
    if (phase === 'delete') {
      errorToast(
        err?.status === 409
          ? t('settings.statusCategories.inUseByStatuses', {
              name: getCategoryDisplayValue(item, 'name'),
            })
          : t('settings.statusCategories.failedToDelete')
      );
      return true;
    }
    if (phase === 'save') {
      errorToast(t('settings.statusCategories.failedToSave'));
      return true;
    }
    return false;
  }

  const categoryColumns = [
    { key: 'category_info', label: t('common.name'), slot: 'category' },
    { key: 'color', label: t('settings.statusCategories.color'), slot: 'color' },
    {
      key: 'description',
      label: t('settings.statusCategories.description'),
      render: (category) => getCategoryDisplayValue(category, 'description') || '—',
      textColor: 'var(--ds-text-subtle)',
    },
    {
      key: 'status_count',
      label: t('statuses.title'),
      render: (category) =>
        t('settings.statusCategories.statusCount', { count: category.statusCount || 0 }),
      textColor: 'var(--ds-text-subtle)',
    },
    { key: 'actions', label: t('common.actions') },
  ];

  function buildCategoryDropdownItems(category, { edit, remove }) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        testid: `status-category-edit-${category.id}`,
        onClick: () => edit(category),
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        testid: `status-category-delete-${category.id}`,
        onClick: () => remove(category),
        disabled: category.statusCount > 0,
      },
    ];
  }
</script>

<EntityManagerPage
  icon={Folder}
  title={t('settings.statusCategories.title')}
  subtitle={t('settings.statusCategories.subtitle')}
  addLabel={t('settings.statusCategories.addStatusCategory')}
  addTestid="status-category-add"
  hotkeyScope="statusCategories"
  emptyMessage={t('settings.statusCategories.empty')}
  emptyIcon={Palette}
  columns={categoryColumns}
  load={loadStatusCategories}
  create={saveCategory}
  update={saveCategory}
  remove={deleteCategory}
  onOpenCreate={openCreate}
  onOpenEdit={openEdit}
  confirmDeleteMessage={(category) =>
    t('settings.statusCategories.confirmDelete', { name: getCategoryDisplayValue(category, 'name') })}
  rowTestid={(category) => `status-category-row-${category.id}`}
  actionItems={buildCategoryDropdownItems}
  actionTriggerTestid={(category) => `status-category-actions-${category.id}`}
  onError={handleError}
  cellSnippets={{ category: categoryNameCell, color: categoryColorCell }}
  formModal={statusCategoryFormModal}
>
</EntityManagerPage>

{#snippet categoryNameCell(category)}
  <div class="flex items-center gap-3">
    <h3 class="font-medium" style="color: var(--ds-text);">{getCategoryDisplayValue(category, 'name')}</h3>
    {#if category.is_default}
      <Lozenge color="blue" text={t('settings.statusCategories.default')} />
    {/if}
    {#if category.is_completed}
      <Lozenge color="emerald" text={t('settings.statusCategories.completed')} />
    {/if}
  </div>
{/snippet}

{#snippet categoryColorCell(category)}
  <div class="flex items-center gap-2">
    <ColorDot color={category.color} class="w-4 h-4 border border-[var(--ds-border)]" />
    <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">{category.color}</span>
  </div>
{/snippet}

{#snippet statusCategoryFormModal({ save, close })}
  <StatusCategoryModal
    isOpen={true}
    bind:formData
    bind:translationEditor
    isEditing={!!editingId}
    objectId={editingId}
    displayName={editingObject?.display_name || editingObject?.name}
    displayDescription={editingObject?.display_description || editingObject?.description}
    onsave={save}
    oncancel={close}
  />
{/snippet}
