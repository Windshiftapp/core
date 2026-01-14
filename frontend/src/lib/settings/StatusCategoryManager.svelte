<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Edit, Trash2, Palette, Circle, Folder } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import StatusCategoryModal from '../dialogs/StatusCategoryModal.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { createShortcutHandler, getShortcutDisplay } from '../utils/keyboardShortcuts.js';

  let statusCategories = [];
  let loading = true;
  let showModal = false;
  let editingId = null;

  // Form state
  let formData = {
    name: '',
    color: '#3b82f6',
    description: '',
    is_default: false,
    is_completed: false
  };

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

  // Keyboard shortcuts
  const handleKeydown = createShortcutHandler({
    addCategory: () => {
      if (!showModal) {
        startCreate();
      }
    }
  }, 'statusCategories');

  async function saveCategory() {
    try {
      if (!formData.name.trim()) {
        alert('Name is required');
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
    } catch (error) {
      console.error('Failed to save status category:', error);
      alert('Failed to save status category: ' + (error.message || error));
    }
  }

  async function deleteCategory(category) {
    if (!confirm(`Are you sure you want to delete the status category "${category.name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.delete(`/status-categories/${category.id}`);
      statusCategories = statusCategories.filter(cat => cat.id !== category.id);
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
        title: 'Edit',
        hoverClass: 'hover-bg',
        onClick: () => startEdit(category)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteCategory(category),
        disabled: category.statusCount > 0
      }
    ];
  }

  // Table column definitions
  const categoryColumns = [
    {
      key: 'category_info',
      label: 'Category',
      slot: 'category'
    },
    {
      key: 'color',
      label: 'Color',
      slot: 'color'
    },
    {
      key: 'description',
      label: 'Description',
      render: (category) => category.description || '—',
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'status_count',
      label: 'Statuses',
      render: (category) => `${category.statusCount || 0} status${category.statusCount === 1 ? '' : 'es'}`,
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];
</script>

<!-- Keyboard shortcuts -->
<svelte:window onkeydown={handleKeydown} />

<div style="background-color: var(--ds-surface); min-height: 100vh;">
  <PageHeader
    icon={Folder}
    title="Status Categories"
    subtitle="Manage status categories and their colors. Categories group related statuses together."
    count="{statusCategories.length} categories"
  >
    {#snippet actions()}
      <Button variant="primary" icon={Plus} onclick={startCreate} keyboardHint={getShortcutDisplay('statusCategories', 'addCategory')}>
        Add Category
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
        <Lozenge color="blue" text="Default" />
      {/if}
      {#if category.is_completed}
        <Lozenge color="emerald" text="Marks completion" />
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
