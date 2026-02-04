<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { Plus, Edit, Trash2, Workflow, ArrowRight } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import Button from '../../components/Button.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Label from '../../components/Label.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';

  let workflows = $state([]);
  let searchQuery = $state('');
  let statuses = $state([]);
  let loading = $state(true);
  let loadingStatuses = $state(true);
  let creating = $state(false);
  let editingId = $state(null);
  let nameInput;

  // Form state
  let newWorkflow = $state({
    name: '',
    description: '',
    is_default: false
  });

  let editWorkflow = $state({
    name: '',
    description: '',
    is_default: false
  });

  onMount(async () => {
    await loadStatuses();
    await loadWorkflows();
  });

  async function loadStatuses() {
    try {
      statuses = await api.get('/statuses');
      loadingStatuses = false;
    } catch (error) {
      console.error('Failed to load statuses:', error);
      loadingStatuses = false;
    }
  }

  async function loadWorkflows() {
    try {
      workflows = await api.get('/workflows');
      loading = false;
    } catch (error) {
      console.error('Failed to load workflows:', error);
      loading = false;
    }
  }

  function startCreate() {
    creating = true;
    newWorkflow = {
      name: '',
      description: '',
      is_default: false
    };
    // Focus the name input after the form is rendered
    setTimeout(() => {
      if (nameInput) {
        nameInput.focus();
      }
    }, 0);
  }

  function cancelCreate() {
    creating = false;
  }

  async function createWorkflow() {
    if (!newWorkflow.name.trim()) {
      alert(t('workflows.enterWorkflowName'));
      return;
    }

    try {
      const created = await api.post('/workflows', newWorkflow);
      workflows = [...workflows, created];
      creating = false;
      newWorkflow = { name: '', description: '', is_default: false };
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (error) {
      console.error('Failed to create workflow:', error);
      alert(t('dialogs.alerts.failedToCreate', { error: error.message || error }));
    }
  }

  function startEdit(workflow) {
    editingId = workflow.id;
    editWorkflow = {
      name: workflow.name,
      description: workflow.description || '',
      is_default: workflow.is_default || false
    };
  }

  function cancelEdit() {
    editingId = null;
  }

  async function updateWorkflow() {
    if (!editWorkflow.name.trim()) {
      alert(t('workflows.enterWorkflowName'));
      return;
    }

    try {
      const updated = await api.put(`/workflows/${editingId}`, editWorkflow);
      workflows = workflows.map(w => w.id === editingId ? updated : w);
      editingId = null;
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (error) {
      console.error('Failed to update workflow:', error);
      alert(t('dialogs.alerts.failedToUpdate', { error: error.message || error }));
    }
  }

  async function deleteWorkflow(workflow) {
    if (!confirm(t('workflows.confirmDeleteWorkflow', { name: workflow.name }))) {
      return;
    }

    try {
      await api.delete(`/workflows/${workflow.id}`);
      workflows = workflows.filter(wf => wf.id !== workflow.id);
      await loadWorkflows();
      window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
    } catch (error) {
      console.error('Failed to delete workflow:', error);
      alert(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  }

  function buildWorkflowDropdownItems(workflow) {
    return [
      {
        id: 'design',
        type: 'regular',
        icon: ArrowRight,
        title: t('workflows.design'),
        hoverClass: 'hover:bg-blue-50',
        onClick: () => navigate(`/workflows/${workflow.id}/design`)
      },
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(workflow)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteWorkflow(workflow)
      }
    ];
  }

  // Search filtering
  const filteredWorkflows = $derived(workflows.filter(wf => {
    if (!searchQuery.trim()) return true;
    const query = searchQuery.toLowerCase();
    return (
      wf.name?.toLowerCase().includes(query) ||
      wf.description?.toLowerCase().includes(query)
    );
  }));

  // Table column definitions (use $derived to make it reactive to language changes)
  let workflowColumns = $derived([
    {
      key: 'workflow',
      label: t('workflows.workflow'),
      slot: 'workflow',
      render: (workflow) => workflow.name // Fallback if slot doesn't work
    },
    {
      key: 'description',
      label: t('common.description'),
      render: (workflow) => workflow.description || '—',
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
    icon={Workflow}
    title={t('workflows.title')}
    subtitle={t('workflows.subtitle')}
  >
    {#snippet actions()}
      <div class="flex items-center gap-3">
        <SearchInput
          bind:value={searchQuery}
          placeholder={t('workflows.searchWorkflows')}
          class="w-64"
        />
        <Button
          variant="primary"
          icon={Plus}
          onclick={startCreate}
          disabled={statuses.length === 0}
          keyboardHint="A"
          hotkeyConfig={{ key: toHotkeyString('workflow', 'add'), guard: () => !creating && !editingId }}
        >
          {t('workflows.createWorkflow')}
        </Button>
      </div>
    {/snippet}
  </PageHeader>

  {#if statuses.length === 0 && !loadingStatuses}
    <div class="rounded-xl border shadow-sm p-12" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <EmptyState
        icon={Workflow}
        title={t('workflows.noStatusesAvailable')}
        description={t('workflows.createStatusesFirst')}
      >
        {#snippet action()}
          <Button variant="primary" onclick={() => navigate('/admin/statuses')}>
            {t('workflows.manageStatuses')}
          </Button>
        {/snippet}
      </EmptyState>
    </div>
  {:else}
    <!-- Workflows Table -->
    <DataTable
      columns={workflowColumns}
      data={filteredWorkflows}
      keyField="id"
      emptyMessage={t('workflows.noWorkflowsFound')}
      emptyIcon={Workflow}
      actionItems={buildWorkflowDropdownItems}
      loading={loading}
      pagination={true}
      pageSize={15}
    >
      <div slot="workflow" let:item={workflow} class="flex items-center gap-3">
        <h3 class="font-medium" style="color: var(--ds-text);">{workflow.name}</h3>
        {#if workflow.is_default}
          <Lozenge color="green" text={t('common.default')} />
        {/if}
      </div>
    </DataTable>
  {/if}
</div>

<!-- Create Workflow Modal -->
<Modal isOpen={creating} onclose={cancelCreate} maxWidth="max-w-lg">
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {t('workflows.createWorkflow')}
    </h3>
  </div>

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); createWorkflow(); }}>
      <div class="space-y-4">
        <div>
          <Label color="default" required class="mb-2">{t('common.name')}</Label>
          <input
            type="text"
            bind:value={newWorkflow.name}
            bind:this={nameInput}
            placeholder={t('workflows.workflowNamePlaceholder')}
            class="w-full px-3 py-2 rounded focus:outline-none focus:ring-2"
            style="border: 1px solid var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
            required
          />
        </div>

        <div>
          <Label color="default" class="mb-2">{t('common.description')}</Label>
          <Textarea
            bind:value={newWorkflow.description}
            placeholder={t('workflows.descriptionPlaceholder')}
            rows={2}
          />
        </div>

        <Checkbox
          bind:checked={newWorkflow.is_default}
          label={t('workflows.setAsDefault')}
          size="small"
        />
      </div>

      <div class="flex justify-end gap-3 mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
        <Button type="button" onclick={cancelCreate}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" type="submit">
          {t('workflows.createWorkflow')}
        </Button>
      </div>
    </form>
  </div>
</Modal>

<!-- Edit Workflow Modal -->
<Modal isOpen={editingId !== null} onclose={cancelEdit} maxWidth="max-w-lg">
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {t('workflows.editWorkflow')}
    </h3>
  </div>

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); updateWorkflow(); }}>
      <div class="space-y-4">
        <div>
          <Label color="default" required class="mb-2">{t('common.name')}</Label>
          <input
            type="text"
            bind:value={editWorkflow.name}
            class="w-full px-3 py-2 rounded focus:outline-none focus:ring-2"
            style="border: 1px solid var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
            required
          />
        </div>

        <div>
          <Label color="default" class="mb-2">{t('common.description')}</Label>
          <Textarea
            bind:value={editWorkflow.description}
            rows={2}
          />
        </div>

        <Checkbox
          bind:checked={editWorkflow.is_default}
          label={t('workflows.setAsDefault')}
          size="small"
        />
      </div>

      <div class="flex justify-end gap-3 mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
        <Button type="button" onclick={cancelEdit}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" type="submit">
          {t('common.saveChanges')}
        </Button>
      </div>
    </form>
  </div>
</Modal>

