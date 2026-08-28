<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { Plus, Edit, Trash2, Palette, Folder } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import ColorDot from '../components/ColorDot.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import StatusCategoryModal from '../dialogs/StatusCategoryModal.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  let statusCategories = $state([]);
  let loading = $state(true);
  let showModal = $state(false);
  let editingId = $state(null);

  const systemStatusCategories = {
    'To Do': {
      key: 'todo',
      color: '#d1d5db',
      description: "Work that hasn't been started",
      isDefault: false,
      isCompleted: false
    },
    'In Progress': {
      key: 'inProgress',
      color: '#3b82f6',
      description: 'Work that is actively being done',
      isDefault: true,
      isCompleted: false
    },
    Done: {
      key: 'done',
      color: '#22c55e',
      description: 'Work that has been completed',
      isDefault: false,
      isCompleted: true
    }
  };

  function getSystemCategoryDefinition(category) {
    const definition = systemStatusCategories[category.name];
    if (
      !definition ||
      category.color?.toLowerCase() !== definition.color ||
      category.description !== definition.description ||
      Boolean(category.is_default) !== definition.isDefault ||
      Boolean(category.is_completed) !== definition.isCompleted
    ) {
      return null;
    }
    return definition;
  }

  function getCategoryDisplayValue(category, field) {
    const definition = getSystemCategoryDefinition(category);
    return definition
      ? t(`settings.statusCategories.defaults.${definition.key}.${field}`)
      : category[field];
  }

  // Form state
  let formData = $state({
    name: '',
    color: '#3b82f6',
    description: '',
    is_default: false,
    is_completed: false
  });

  onMount(async () => {
    await loadStatusCategories();
  });

  async function loadStatusCategories() {
    try {
      loading = true;
      const [categories, statuses] = await Promise.all([
        api.get('/status-categories') || [],
        api.get('/statuses') || []
      ]);

      // Add status count to each category
      statusCategories = categories.map(category => ({
        ...category,
        statusCount: statuses.filter(status => status.category_id === category.id).length
      }));
    } catch (error) {
      console.error('Failed to load status categories:', error);
      statusCategories = [];
    } finally {
      loading = false;
    }
  }

  function startCreate() {
    formData = {
      name: '',
      color: '#ef4444',
      description: '',
      is_default: false,
      is_completed: false
    };
    editingId = null;
    showModal = true;
  }

  function startEdit(category) {
    formData = {
      name: category.name || '',
      color: category.color || '#3b82f6',
      description: category.description || '',
      is_default: category.is_default || false,
      is_completed: category.is_completed || false
    };
    editingId = category.id;
    showModal = true;
  }

  function cancelForm() {
    showModal = false;
    editingId = null;
    formData = {
      name: '',
      color: '#3b82f6',
      description: '',
      is_default: false,
      is_completed: false
    };
  }

  async function saveCategory() {
    try {
      if (!formData.name.trim()) {
        errorToast(t('settings.statusCategories.nameRequired'));
        return;
      }

      if (editingId) {
        const updated = await api.put(`/status-categories/${editingId}`, formData);
        statusCategories = statusCategories.map(cat => 
          cat.id === editingId ? { ...updated, statusCount: cat.statusCount } : cat
        );
      } else {
        const created = await api.post('/status-categories', formData);
        statusCategories = [...statusCategories, { ...created, statusCount: 0 }];
      }
      
      cancelForm();
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (error) {
      console.error('Failed to save status category:', error);
      errorToast(t('settings.statusCategories.failedToSave'));
    }
  }

  async function deleteCategory(category) {
    const displayName = getCategoryDisplayValue(category, 'name');
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('settings.statusCategories.confirmDelete', { name: displayName }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.delete(`/status-categories/${category.id}`);
      statusCategories = statusCategories.filter(cat => cat.id !== category.id);
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (error) {
      console.error('Failed to delete status category:', error);
      
      if (error.status === 409) {
        errorToast(t('settings.statusCategories.inUseByStatuses', { name: displayName }));
      } else {
        errorToast(t('settings.statusCategories.failedToDelete'));
      }
    }
  }

  function buildCategoryDropdownItems(category) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(category)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deleteCategory(category),
        disabled: category.statusCount > 0
      }
    ];
  }

  // Table column definitions
  const categoryColumns = $derived([
    {
      key: 'category_info',
      label: t('common.name'),
      slot: 'category'
    },
    {
      key: 'color',
      label: t('settings.statusCategories.color'),
      slot: 'color'
    },
    {
      key: 'description',
      label: t('settings.statusCategories.description'),
      render: (category) => getCategoryDisplayValue(category, 'description') || '—',
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'status_count',
      label: t('statuses.title'),
      render: (category) => t('settings.statusCategories.statusCount', {
        count: category.statusCount || 0
      }),
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ]);
</script>

<div style="background-color: var(--ds-surface); min-height: 100vh;">
  <PageHeader
    icon={Folder}
    title={t('settings.statusCategories.title')}
    subtitle={t('settings.statusCategories.subtitle')}
    count={t('settings.statusCategories.count', { count: statusCategories.length })}
  >
    {#snippet actions()}
      <Button variant="primary" icon={Plus} onclick={startCreate} keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('statusCategories', 'addCategory'), guard: () => !showModal }}>
        {t('settings.statusCategories.addStatusCategory')}
      </Button>
    {/snippet}
  </PageHeader>

  <DataTable
    columns={categoryColumns}
    data={statusCategories}
    keyField="id"
    emptyMessage={t('settings.statusCategories.empty')}
    emptyIcon={Palette}
    actionItems={buildCategoryDropdownItems}
  >
    {#snippet category(category)}
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

    {#snippet color(category)}
      <div class="flex items-center gap-2">
        <ColorDot color={category.color} class="w-4 h-4 border border-[var(--ds-border)]" />
        <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">{category.color}</span>
      </div>
    {/snippet}
  </DataTable>

  <!-- Status Category Modal -->
  <StatusCategoryModal
    isOpen={showModal}
    bind:formData
    isEditing={!!editingId}
    onsave={saveCategory}
    oncancel={cancelForm}
  />
</div>
