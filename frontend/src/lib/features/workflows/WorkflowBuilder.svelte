<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { Plus, Edit, Trash2, Workflow, ArrowRight } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Label from '../../components/Label.svelte';
  import { matchesShortcut } from '../../utils/keyboardShortcuts.js';

  let workflows = [];
  let statuses = [];
  let loading = true;
  let loadingStatuses = true;
  let creating = false;
  let editingId = null;
  let nameInput;

  // Form state
  let newWorkflow = {
    name: '',
    description: '',
    is_default: false
  };

  let editWorkflow = {
    name: '',
    description: '',
    is_default: false
  };

  onMount(async () => {
    await loadStatuses();
    await loadWorkflows();
    
    // Add global keydown listener for 'a' shortcut
    document.addEventListener('keydown', handleGlobalKeydown);
    
    // Cleanup on unmount
    return () => {
      document.removeEventListener('keydown', handleGlobalKeydown);
    };
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
      alert('Please enter a workflow name');
      return;
    }

    try {
      const created = await api.post('/workflows', newWorkflow);
      workflows = [...workflows, created];
      creating = false;
      newWorkflow = { name: '', description: '', is_default: false };
    } catch (error) {
      console.error('Failed to create workflow:', error);
      alert('Failed to create workflow: ' + (error.message || error));
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
      alert('Please enter a workflow name');
      return;
    }

    try {
      const updated = await api.put(`/workflows/${editingId}`, editWorkflow);
      workflows = workflows.map(w => w.id === editingId ? updated : w);
      editingId = null;
    } catch (error) {
      console.error('Failed to update workflow:', error);
      alert('Failed to update workflow: ' + (error.message || error));
    }
  }

  async function deleteWorkflow(workflow) {
    if (!confirm(`Are you sure you want to delete "${workflow.name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.delete(`/workflows/${workflow.id}`);
      workflows = workflows.filter(wf => wf.id !== workflow.id);
      await loadWorkflows();
    } catch (error) {
      console.error('Failed to delete workflow:', error);
      alert('Failed to delete workflow: ' + (error.message || error));
    }
  }

  function handleGlobalKeydown(event) {
    // Check for 'a' key to add new workflow
    if (matchesShortcut(event, { key: 'a' }) && !creating && !editingId) {
      const target = event.target;
      if (target.tagName !== 'INPUT' && target.tagName !== 'TEXTAREA' && !target.contentEditable.includes('true')) {
        event.preventDefault();
        startCreate();
      }
    }
  }

  function buildWorkflowDropdownItems(workflow) {
    return [
      {
        id: 'design',
        type: 'regular',
        icon: ArrowRight,
        title: 'Design',
        hoverClass: 'hover:bg-blue-50',
        onClick: () => navigate(`/workflows/${workflow.id}/design`)
      },
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover-bg',
        onClick: () => startEdit(workflow)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteWorkflow(workflow)
      }
    ];
  }

  // Table column definitions
  const workflowColumns = [
    {
      key: 'workflow',
      label: 'Workflow',
      slot: 'workflow',
      render: (workflow) => workflow.name // Fallback if slot doesn't work
    },
    {
      key: 'description',
      label: 'Description',
      render: (workflow) => workflow.description || '—',
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
    icon={Workflow} 
    title="Workflows" 
    subtitle="Design and manage workflow transitions between statuses."
    count="{workflows.length} workflows"
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        onclick={startCreate}
        disabled={statuses.length === 0}
        keyboardHint="A"
      >
        Create Workflow
      </Button>
    {/snippet}
  </PageHeader>

  {#if statuses.length === 0 && !loadingStatuses}
    <div class="rounded-xl border shadow-sm p-12" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <EmptyState
        icon={Workflow}
        title="No statuses available"
        description="You need to create statuses before you can create workflows."
      >
        {#snippet action()}
          <Button variant="primary" onclick={() => navigate('/admin/statuses')}>
            Manage Statuses
          </Button>
        {/snippet}
      </EmptyState>
    </div>
  {:else}
    <!-- Workflows Table -->
    <DataTable
      columns={workflowColumns}
      data={workflows}
      keyField="id"
      emptyMessage="No workflows found. Create your first workflow to get started."
      emptyIcon={Workflow}
      actionItems={buildWorkflowDropdownItems}
      loading={loading}
    >
      <div slot="workflow" let:item={workflow} class="flex items-center gap-3">
        <h3 class="font-medium" style="color: var(--ds-text);">{workflow.name}</h3>
        {#if workflow.is_default}
          <Lozenge color="green" text="Default" />
        {/if}
      </div>
    </DataTable>
  {/if}
</div>

<!-- Create Workflow Modal -->
<Modal isOpen={creating} onclose={cancelCreate} maxWidth="max-w-lg">
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      Create Workflow
    </h3>
  </div>

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); createWorkflow(); }}>
      <div class="space-y-4">
        <div>
          <Label color="default" required class="mb-2">Name</Label>
          <input
            type="text"
            bind:value={newWorkflow.name}
            bind:this={nameInput}
            placeholder="e.g., Default Workflow"
            class="w-full px-3 py-2 rounded focus:outline-none focus:ring-2"
            style="border: 1px solid var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
            required
          />
        </div>

        <div>
          <Label color="default" class="mb-2">Description</Label>
          <Textarea
            bind:value={newWorkflow.description}
            placeholder="Optional description for this workflow"
            rows={2}
          />
        </div>

        <div class="flex items-center gap-2">
          <input
            type="checkbox"
            bind:checked={newWorkflow.is_default}
            id="new-default"
            class="rounded"
            style="border-color: var(--ds-border);"
          />
          <label for="new-default" class="text-sm" style="color: var(--ds-text);">Set as default workflow</label>
        </div>
      </div>

      <div class="flex justify-end gap-3 mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
        <Button type="button" onclick={cancelCreate}>
          Cancel
        </Button>
        <Button variant="primary" type="submit">
          Create Workflow
        </Button>
      </div>
    </form>
  </div>
</Modal>

<!-- Edit Workflow Modal -->
<Modal isOpen={editingId !== null} onclose={cancelEdit} maxWidth="max-w-lg">
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      Edit Workflow
    </h3>
  </div>

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); updateWorkflow(); }}>
      <div class="space-y-4">
        <div>
          <Label color="default" required class="mb-2">Name</Label>
          <input
            type="text"
            bind:value={editWorkflow.name}
            class="w-full px-3 py-2 rounded focus:outline-none focus:ring-2"
            style="border: 1px solid var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
            required
          />
        </div>

        <div>
          <Label color="default" class="mb-2">Description</Label>
          <Textarea
            bind:value={editWorkflow.description}
            rows={2}
          />
        </div>

        <div class="flex items-center gap-2">
          <input
            type="checkbox"
            bind:checked={editWorkflow.is_default}
            id="edit-default"
            class="rounded"
            style="border-color: var(--ds-border);"
          />
          <label for="edit-default" class="text-sm" style="color: var(--ds-text);">Set as default workflow</label>
        </div>
      </div>

      <div class="flex justify-end gap-3 mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
        <Button type="button" onclick={cancelEdit}>
          Cancel
        </Button>
        <Button variant="primary" type="submit">
          Save Changes
        </Button>
      </div>
    </form>
  </div>
</Modal>

