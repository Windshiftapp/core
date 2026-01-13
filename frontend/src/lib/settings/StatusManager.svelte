<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Edit, Trash2, Save, X, CheckCircle, Circle, MoreHorizontal, GitBranch } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { getHexFromColorName } from '../utils/colors.js';
  import Modal from '../dialogs/Modal.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Toggle from '../components/Toggle.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { matchesShortcut } from '../utils/keyboardShortcuts.js';

  // System-protected status IDs (cannot be deleted)
  const PROTECTED_STATUS_IDS = [1, 6]; // Open and Closed

  let statuses = [];
  let statusCategories = [];
  let workflowTransitions = [];
  let loading = true;
  let loadingCategories = true;
  let showCreateForm = false;
  let editingId = null;

  // Form state
  let formData = {
    name: '',
    description: '',
    category_id: null,
    is_default: false
  };

  onMount(async () => {
    await loadStatusCategories();
    await loadStatuses();

    // Add global keyboard listener
    window.addEventListener('keydown', handleGlobalKeydown);
  });

  onDestroy(() => {
    window.removeEventListener('keydown', handleGlobalKeydown);
  });

  function handleGlobalKeydown(event) {
    // 'a' to add/create new status
    if (matchesShortcut(event, { key: 'a' }) && !showCreateForm) {
      const target = event.target;
      if (target.tagName !== 'INPUT' && target.tagName !== 'TEXTAREA' && !target.contentEditable.includes('true')) {
        event.preventDefault();
        startCreate();
      }
    }
  }

  async function loadStatusCategories() {
    try {
      loadingCategories = true;
      statusCategories = await api.get('/status-categories') || [];
      // Set default category if none selected
      if (statusCategories.length > 0 && !formData.category_id) {
        formData.category_id = statusCategories[0].id;
      }
    } catch (error) {
      console.error('Failed to load status categories:', error);
      statusCategories = [];
    } finally {
      loadingCategories = false;
    }
  }

  async function loadStatuses() {
    try {
      loading = true;
      const [statusesResult, workflows] = await Promise.all([
        api.get('/statuses') || [],
        api.get('/workflows') || []
      ]);
      
      // Get all transitions from all workflows
      const allTransitions = [];
      for (const workflow of workflows) {
        try {
          const transitions = await api.get(`/workflows/${workflow.id}/transitions`) || [];
          allTransitions.push(...transitions);
        } catch (error) {
          console.error(`Failed to load transitions for workflow ${workflow.id}:`, error);
        }
      }
      
      workflowTransitions = allTransitions;
      
      // Add transition count to each status
      statuses = statusesResult.map(status => ({
        ...status,
        transitionCount: allTransitions.filter(t => 
          t.from_status_id === status.id || t.to_status_id === status.id
        ).length
      }));
    } catch (error) {
      console.error('Failed to load statuses:', error);
      statuses = [];
    } finally {
      loading = false;
    }
  }

  function startCreate() {
    formData = {
      name: '',
      description: '',
      category_id: statusCategories.length > 0 ? statusCategories[0].id : null,
      is_default: false
    };
    editingId = null;
    showCreateForm = true;
  }

  function startEdit(status) {
    formData = {
      name: status.name || '',
      description: status.description || '',
      category_id: status.category_id,
      is_default: status.is_default || false
    };
    editingId = status.id;
    showCreateForm = true;
  }

  function cancelForm() {
    showCreateForm = false;
    editingId = null;
    formData = {
      name: '',
      description: '',
      category_id: statusCategories.length > 0 ? statusCategories[0].id : null,
      is_default: false
    };
  }

  async function saveStatus() {
    try {
      if (!formData.name.trim()) {
        alert('Name is required');
        return;
      }

      if (editingId) {
        const updated = await api.put(`/statuses/${editingId}`, formData);
        statuses = statuses.map(status => 
          status.id === editingId ? { ...updated, transitionCount: status.transitionCount } : status
        );
      } else {
        const created = await api.post('/statuses', formData);
        statuses = [...statuses, { ...created, transitionCount: 0 }];
      }
      
      cancelForm();
    } catch (error) {
      console.error('Failed to save status:', error);
      alert('Failed to save status: ' + (error.message || error));
    }
  }

  async function deleteStatus(status) {
    // Protect system-critical statuses
    if (PROTECTED_STATUS_IDS.includes(status.id)) {
      return; // Silently ignore - button should already be disabled
    }

    if (status.transitionCount > 0) {
      alert(
        `Cannot delete "${status.name}" because it's being used in ${status.transitionCount} workflow transition${status.transitionCount === 1 ? '' : 's'}.\n\n` +
        `To delete this status:\n` +
        `1. Go to Workflow Management\n` +
        `2. Remove all transitions that use this status\n` +
        `3. Then try deleting the status again`
      );
      return;
    }

    if (!confirm(`Are you sure you want to delete the status "${status.name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.delete(`/statuses/${status.id}`);
      statuses = statuses.filter(s => s.id !== status.id);
    } catch (error) {
      console.error('Failed to delete status:', error);
      alert('Failed to delete status: ' + (error.message || error));
    }
  }

  function getCategoryColor(categoryId) {
    const category = statusCategories.find(cat => cat.id === categoryId);
    if (!category) return '#6b7280';

    // If color is a hex code, return it directly; otherwise convert from color name
    return category.color.startsWith('#') ? category.color : getHexFromColorName(category.color);
  }

  function getCategoryName(categoryId) {
    const category = statusCategories.find(cat => cat.id === categoryId);
    return category ? category.name : 'Unknown';
  }

  function buildStatusDropdownItems(status) {
    const isProtected = PROTECTED_STATUS_IDS.includes(status.id);
    const inUse = status.transitionCount > 0;

    const items = [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => startEdit(status)
      }
    ];

    // Only show delete option for non-protected statuses
    if (!isProtected) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: inUse ? 'Cannot delete status in use by workflows' : 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteStatus(status),
        disabled: inUse
      });
    }

    return items;
  }

  // Table column definitions
  const statusColumns = [
    {
      key: 'status_info',
      label: 'Status',
      slot: 'status'
    },
    {
      key: 'category_info',
      label: 'Category',
      slot: 'category'
    },
    {
      key: 'description',
      label: 'Description',
      render: (status) => status.description || '—',
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'transitions',
      label: 'Workflow Usage',
      render: (status) => `${status.transitionCount || 0} transition${status.transitionCount === 1 ? '' : 's'}`,
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];
</script>

<div style="background-color: var(--ds-surface); min-height: 100vh;">
  <PageHeader 
    icon={GitBranch} 
    title="Statuses" 
    subtitle="Manage individual statuses. Each status belongs to a category and can be used in workflows."
    count="{statuses.length} statuses"
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        onclick={startCreate}
        disabled={statusCategories.length === 0}
        keyboardHint="A"
      >
        Add Status
      </Button>
    {/snippet}
  </PageHeader>

  {#if statusCategories.length === 0 && !loadingCategories}
    <div class="rounded-xl border shadow-sm p-12 text-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <Circle class="w-12 h-12 text-gray-400 mx-auto mb-4" />
      <h3 class="text-lg font-medium text-gray-900 mb-2">No status categories found</h3>
      <p class="text-gray-500 mb-6">You need to create status categories before you can create statuses.</p>
      <Button href="/admin/status-categories" variant="primary">
        Go to Status Categories
      </Button>
    </div>
  {:else}
    <DataTable
      columns={statusColumns}
      data={statuses}
      keyField="id"
      emptyMessage="No statuses found. Create your first status to get started."
      emptyIcon={Circle}
      actionItems={buildStatusDropdownItems}
    >
      <div slot="status" let:item={status} class="flex items-center gap-3">
        <h3 class="font-medium" style="color: var(--ds-text);">{status.name}</h3>
        {#if status.is_default}
          <Lozenge color="green" text="Default" />
        {/if}
      </div>
      
      <div slot="category" let:item={status} class="flex items-center gap-2">
        <div
          class="w-4 h-4 rounded border border-gray-300"
          style="background-color: {getCategoryColor(status.category_id)};"
        ></div>
        <span class="font-medium" style="color: var(--ds-text);">{getCategoryName(status.category_id)}</span>
      </div>
    </DataTable>
  {/if}

  <Modal isOpen={showCreateForm} onclose={cancelForm} maxWidth="max-w-lg">
    <!-- Modal header -->
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
        {editingId ? 'Edit Status' : 'Create Status'}
      </h3>
    </div>

    <!-- Modal content -->
    <div class="px-6 py-4">
      <form onsubmit={(e) => { e.preventDefault(); saveStatus(); }}>
        <div class="form-group">
          <label for="name">Name *</label>
          <input
            type="text"
            id="name"
            placeholder="e.g. Open, In Progress, Resolved"
            bind:value={formData.name}
            required
          />
        </div>

        <div class="form-group">
          <label for="category">Category *</label>
          <BasePicker
            bind:value={formData.category_id}
            items={statusCategories}
            placeholder="Select category..."
            getValue={(item) => item.id}
            getLabel={(item) => item.name}
          />
        </div>

        <div class="form-group">
          <label for="description">Description</label>
          <Textarea
            id="description"
            placeholder="Optional description for this status"
            bind:value={formData.description}
            rows={2}
          />
        </div>

        <div class="mb-6">
          <Toggle
            bind:checked={formData.is_default}
            label="Set as default status"
            size="small"
          />
        </div>

        <!-- Modal footer -->
        <DialogFooter
          onCancel={cancelForm}
          onConfirm={saveStatus}
          confirmLabel="{editingId ? 'Update' : 'Create'} Status"
          showKeyboardHint={true}
          class="mx-[-1.5rem] mb-[-1rem] mt-0"
        />
      </form>
    </div>
  </Modal>
</div>

<style>
  .form-group {
    margin-bottom: 1.5rem;
  }

  .form-group label {
    display: block;
    margin-bottom: 0.5rem;
    font-weight: 500;
    color: var(--ds-text);
  }

  .form-group input,
  .form-group textarea,
  .form-group select {
    width: 100%;
    padding: 0.75rem;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    font-size: 0.9rem;
    background: var(--ds-surface);
    color: var(--ds-text);
    transition: border-color 0.2s ease;
  }

  .form-group input:focus,
  .form-group textarea:focus,
  .form-group select:focus {
    outline: none;
    border-color: var(--ds-border-focused);
    box-shadow: 0 0 0 3px var(--ds-focus-ring);
  }
</style>