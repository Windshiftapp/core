<script>
  import { statuses as statusesApi } from '../api/workflows.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Circle from '@lucide/svelte/icons/circle';
  import GitBranch from '@lucide/svelte/icons/git-branch';
  import ColorDot from '../components/ColorDot.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { getHexFromColorName } from '../utils/colors.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { loadStatusManagerData } from './statusManagerData.js';
  import { objectDisplayName, objectDisplayValue } from '../utils/systemLabels.js';
  import StatusModal from '../dialogs/StatusModal.svelte';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let statusCategories = $state([]);
  let workflowTransitions = $state([]);
  let editingId = $state(null);
  let editingObject = $state(null);
  let translationEditor = $state(null);

  let formData = $state({
    name: '',
    description: '',
    category_id: null,
    is_default: false,
  });

  function getStatusDisplayValue(status, field) {
    return objectDisplayValue(status, field);
  }

  function getStatusCategoryDisplayName(category) {
    return objectDisplayName(category);
  }

  function isProtectedStatus(status) {
    return status?.builtin_key === 'open' || status?.builtin_key === 'done';
  }

  async function loadStatuses() {
    const data = await loadStatusManagerData();
    statusCategories = data.statusCategories;
    workflowTransitions = data.workflowTransitions;
    return data.statuses;
  }

  function openCreate() {
    formData = {
      name: '',
      description: '',
      category_id: statusCategories.length > 0 ? statusCategories[0].id : null,
      is_default: false,
    };
    editingId = null;
    editingObject = null;
  }

  function openEdit(status) {
    editingObject = status;
    formData = {
      name: status.name || '',
      description: status.description || '',
      category_id: status.category_id,
      is_default: status.is_default || false,
    };
    editingId = status.id;
  }

  async function saveStatus() {
    if (editingId) {
      translationEditor?.validate();
      await statusesApi.update(editingId, formData);
      await translationEditor?.save();
    } else {
      await statusesApi.create(formData);
    }
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }

  async function deleteStatus(status) {
    if (isProtectedStatus(status)) return;
    if (status.transitionCount > 0) {
      errorToast(
        t('dialogs.alerts.statusInUseByTransitions', {
          name: getStatusDisplayValue(status, 'name'),
          count: status.transitionCount,
        })
      );
      return;
    }
    await statusesApi.delete(status.id);
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }

  function getCategoryColor(categoryId) {
    const category = statusCategories.find((cat) => cat.id === categoryId);
    if (!category) return '#6b7280';
    return category.color.startsWith('#') ? category.color : getHexFromColorName(category.color);
  }

  function getCategoryName(categoryId) {
    const category = statusCategories.find((cat) => cat.id === categoryId);
    return category ? getStatusCategoryDisplayName(category) : t('common.unknown');
  }

  const statusColumns = [
    { key: 'status_info', label: t('common.status'), slot: 'status' },
    { key: 'category_info', label: t('common.category'), slot: 'category' },
    {
      key: 'description',
      label: t('common.description'),
      render: (status) => getStatusDisplayValue(status, 'description') || '—',
      textColor: 'var(--ds-text-subtle)',
    },
    {
      key: 'transitions',
      label: t('workflows.transitions'),
      render: (status) =>
        t('statuses.transitionCount', { count: status.transitionCount || 0 }),
      textColor: 'var(--ds-text-subtle)',
    },
    { key: 'actions', label: t('common.actions') },
  ];

  function buildStatusDropdownItems(status, { edit, remove }) {
    const isProtected = isProtectedStatus(status);
    const inUse = status.transitionCount > 0;

    const items = [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        testid: `status-edit-${status.id}`,
        onClick: () => edit(status),
      },
    ];

    if (!isProtected) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        testid: `status-delete-${status.id}`,
        onClick: () => remove(status),
        disabled: inUse,
      });
    }

    return items;
  }
</script>

<EntityManagerPage
  icon={GitBranch}
  title={t('statuses.title')}
  subtitle={t('statuses.subtitle')}
  addLabel={t('statuses.createStatus')}
  addTestid="status-add"
  hotkeyScope="statuses"
  emptyMessage={t('statuses.noStatuses')}
  emptyIcon={Circle}
  columns={statusColumns}
  load={loadStatuses}
  create={saveStatus}
  update={saveStatus}
  remove={deleteStatus}
  onOpenCreate={openCreate}
  onOpenEdit={openEdit}
  confirmDeleteMessage={(status) =>
    t('dialogs.confirmations.deleteItem', { name: getStatusDisplayValue(status, 'name') })}
  rowTestid={(status) => `status-row-${status.id}`}
  actionItems={buildStatusDropdownItems}
  actionTriggerTestid={(status) => `status-actions-${status.id}`}
  errorLabels={{ save: t('dialogs.alerts.failedToSave', { error: '' }) }}
  cellSnippets={{ status: statusNameCell, category: statusCategoryCell }}
  formModal={statusFormModal}
>
</EntityManagerPage>

{#snippet statusNameCell(status)}
  <div class="flex items-center gap-3">
    <h3 class="font-medium" style="color: var(--ds-text);">{getStatusDisplayValue(status, 'name')}</h3>
    {#if status.is_default}
      <Lozenge color="green" text={t('common.default')} />
    {/if}
  </div>
{/snippet}

{#snippet statusCategoryCell(status)}
  <div class="flex items-center gap-2">
    <ColorDot
      color={getCategoryColor(status.category_id)}
      class="w-4 h-4 border border-[var(--ds-border)]"
    />
    <span class="font-medium" style="color: var(--ds-text);">{getCategoryName(status.category_id)}</span>
  </div>
{/snippet}

{#snippet statusFormModal({ save, close, saving })}
  <StatusModal
    isOpen={true}
    bind:formData
    categories={statusCategories}
    isEditing={editingId !== null}
    objectId={editingId}
    displayName={editingObject?.display_name || editingObject?.name}
    displayDescription={editingObject?.display_description || editingObject?.description}
    bind:translationEditor
    {saving}
    onsave={save}
    oncancel={close}
  />
{/snippet}
