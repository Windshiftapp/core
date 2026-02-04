<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { Plus, Edit, Trash2, Palette, Circle, Folder } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import StatusCategoryModal from '../dialogs/StatusCategoryModal.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  let statusCategories = $state([]);
  let loading = $state(true);
  let showModal = $state(false);
  let editingId = $state(null);

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
        alert(t('settings.statusCategories.nameRequired'));
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
      alert(t('settings.statusCategories.failedToSave') + ' ' + (error.message || error));
    }
  }

  async function deleteCategory(category) {
    if (!confirm(`Are you sure you want to delete the status category "${category.name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.delete(`/status-categories/${category.id}`);
      statusCategories = statusCategories.filter(cat => cat.id !== category.id);
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (error) {
      console.error('Failed to delete status category:', error);
      
      if (error.message && error.message.includes('Cannot delete status category that is in use')) {
        alert(
          `Cannot delete "${category.name}" because it's being used by one or more statuses.\n\n` +
          `To delete this category:\n` +
          `1. Go to Status Management\n` +
          `2. Delete or reassign all statuses in this category\n` +
          `3. Then try deleting the category again`
        );
      } else {
        alert('Failed to delete status category: ' + (error.message || error));
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
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
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
      render: (category) => category.description || '—',
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'status_count',
      label: t('statuses.title'),
      render: (category) => `${category.statusCount || 0} status${category.statusCount === 1 ? '' : 'es'}`,
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
    count="{statusCategories.length} categories"
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
    emptyMessage="No status categories found. Create your first status category to get started."
    emptyIcon={Palette}
    actionItems={buildCategoryDropdownItems}
  >
    <div slot="category" let:item={category} class="flex items-center gap-3">
      <h3 class="font-medium" style="color: var(--ds-text);">{category.name}</h3>
      {#if category.is_default}
        <Lozenge color="blue" text={t('settings.statusCategories.default')} />
      {/if}
      {#if category.is_completed}
        <Lozenge color="emerald" text={t('settings.statusCategories.completed')} />
      {/if}
    </div>
    
    <div slot="color" let:item={category} class="flex items-center gap-2">
      <div
        class="w-4 h-4 rounded border border-gray-300"
        style="background-color: {category.color};"
      ></div>
      <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">{category.color}</span>
    </div>
  </DataTable>

  <!-- Status Category Modal -->
  <StatusCategoryModal
    isOpen={showModal}
    bind:formData
    isEditing={!!editingId}
    on:save={saveCategory}
    on:cancel={cancelForm}
  />
</div>
