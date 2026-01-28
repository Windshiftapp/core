<script>
  import { onMount } from 'svelte';
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
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import { t } from '../stores/i18n.svelte.js';

  // System-protected status IDs (cannot be deleted)
  const PROTECTED_STATUS_IDS = [1, 6]; // Open and Closed

  let statuses = $state([]);
  let statusCategories = $state([]);
  let workflowTransitions = $state([]);
  let loading = $state(true);
  let loadingCategories = $state(true);
  let showCreateForm = $state(false);
  let editingId = $state(null);

  // Form state
  let formData = $state({
    name: '',
    description: '',
    category_id: null,
    is_default: false
  });

  onMount(async () => {
    await loadStatusCategories();
    await loadStatuses();
  });

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
        alert(t('dialogs.alerts.nameRequired'));
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
      alert(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  async function deleteStatus(status) {
    // Protect system-critical statuses
    if (PROTECTED_STATUS_IDS.includes(status.id)) {
      return; // Silently ignore - button should already be disabled
    }

    if (status.transitionCount > 0) {
      alert(t('dialogs.alerts.statusInUseByTransitions', {
        name: status.name,
        count: status.transitionCount
      }));
      return;
    }

    if (!confirm(t('dialogs.confirmations.deleteItem', { name: status.name }))) {
      return;
    }

    try {
      await api.delete(`/statuses/${status.id}`);
      statuses = statuses.filter(s => s.id !== status.id);
    } catch (error) {
      console.error('Failed to delete status:', error);
      alert(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
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
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(status)
      }
    ];

    // Only show delete option for non-protected statuses
    if (!isProtected) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteStatus(status),
        disabled: inUse
      });
    }

    return items;
  }

  // Table column definitions
  const statusColumns = $derived([
    {
      key: 'status_info',
      label: t('common.status'),
      slot: 'status'
    },
    {
      key: 'category_info',
      label: t('common.category'),
      slot: 'category'
    },
    {
      key: 'description',
      label: t('common.description'),
      render: (status) => status.description || '—',
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'transitions',
      label: t('workflows.transitions'),
      render: (status) => `${status.transitionCount || 0} transition${status.transitionCount === 1 ? '' : 's'}`,
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
    icon={GitBranch}
    title={t('statuses.title')}
    subtitle={t('statuses.subtitle')}
    count={t('statuses.statuses', { count: statuses.length })}
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        onclick={startCreate}
        disabled={statusCategories.length === 0}
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('statuses', 'add'), guard: () => !showCreateForm }}
      >
        {t('statuses.createStatus')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if statusCategories.length === 0 && !loadingCategories}
    <div class="rounded-xl border shadow-sm p-12 text-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <Circle class="w-12 h-12 text-gray-400 mx-auto mb-4" />
      <h3 class="text-lg font-medium text-gray-900 mb-2">{t('categories.noCategories')}</h3>
      <p class="text-gray-500 mb-6">{t('statuses.noStatuses')}</p>
      <Button href="/admin/status-categories" variant="primary">
        {t('categories.title')}
      </Button>
    </div>
  {:else}
    <DataTable
      columns={statusColumns}
      data={statuses}
      keyField="id"
      emptyMessage={t('statuses.noStatuses')}
      emptyIcon={Circle}
      actionItems={buildStatusDropdownItems}
    >
      <div slot="status" let:item={status} class="flex items-center gap-3">
        <h3 class="font-medium" style="color: var(--ds-text);">{status.name}</h3>
        {#if status.is_default}
          <Lozenge color="green" text={t('common.default')} />
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

  <Modal isOpen={showCreateForm} onclose={cancelForm} maxWidth="max-w-lg" onSubmit={saveStatus} let:submitHint>
    <!-- Modal header -->
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
        {editingId ? t('statuses.editStatus') : t('statuses.createStatus')}
      </h3>
    </div>

    <!-- Modal content -->
    <div class="px-6 py-4">
      <form onsubmit={(e) => { e.preventDefault(); saveStatus(); }}>
        <div class="form-group">
          <label for="name">{t('common.name')} *</label>
          <input
            type="text"
            id="name"
            placeholder="e.g. Open, In Progress, Resolved"
            bind:value={formData.name}
            required
          />
        </div>

        <div class="form-group">
          <label for="category">{t('common.category')} *</label>
          <BasePicker
            bind:value={formData.category_id}
            items={statusCategories}
            placeholder={t('categories.selectCategory')}
            getValue={(item) => item.id}
            getLabel={(item) => item.name}
          />
        </div>

        <div class="form-group">
          <label for="description">{t('common.description')}</label>
          <Textarea
            id="description"
            placeholder={t('placeholders.optionalDescription')}
            bind:value={formData.description}
            rows={2}
          />
        </div>

        <div class="mb-6">
          <Toggle
            bind:checked={formData.is_default}
            label={t('common.default')}
            size="small"
          />
        </div>

        <!-- Modal footer -->
        <DialogFooter
          onCancel={cancelForm}
          onConfirm={saveStatus}
          confirmLabel={editingId ? t('common.update') : t('common.create')}
          showKeyboardHint={true}
          confirmKeyboardHint={submitHint}
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