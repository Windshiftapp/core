<script>
  import { Globe, Building2, Calendar, Tag, FileText } from 'lucide-svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';

  let {
    iteration = null,
    workspaceId = null,
    iterationTypes = [],
    canManageGlobal = false,
    onsave = () => {},
    oncancel = () => {}
  } = $props();

  let formData = $state({
    name: iteration?.name || '',
    description: iteration?.description || '',
    start_date: iteration?.start_date ? iteration.start_date.split('T')[0] : '',
    end_date: iteration?.end_date ? iteration.end_date.split('T')[0] : '',
    status: iteration?.status || 'planned',
    type_id: iteration?.type_id || null,
    is_global: iteration?.is_global || false,
    workspace_id: iteration?.workspace_id || (workspaceId ? parseInt(workspaceId) : null)
  });

  let error = $state('');
  let saving = $state(false);

  const statusOptions = [
    { value: 'planned', label: 'Planned' },
    { value: 'active', label: 'Active' },
    { value: 'completed', label: 'Completed' },
    { value: 'cancelled', label: 'Cancelled' }
  ];

  function handleCancel() {
    oncancel();
  }

  async function handleSave() {
    error = '';

    // Validation
    if (!formData.name.trim()) {
      error = 'Iteration name is required';
      return;
    }

    if (!formData.start_date) {
      error = 'Start date is required';
      return;
    }

    if (!formData.end_date) {
      error = 'End date is required';
      return;
    }

    if (new Date(formData.end_date) < new Date(formData.start_date)) {
      error = 'End date must be after start date';
      return;
    }

    // Ensure global iterations don't have workspace_id
    const dataToSave = { ...formData };
    if (dataToSave.is_global) {
      dataToSave.workspace_id = null;
    } else {
      dataToSave.workspace_id = workspaceId ? parseInt(workspaceId) : null;
    }

    try {
      saving = true;
      onsave(dataToSave);
    } catch (err) {
      error = err.message || 'Failed to save iteration';
      saving = false;
    }
  }

  function toggleScope() {
    formData.is_global = !formData.is_global;
    if (formData.is_global) {
      formData.workspace_id = null;
    } else {
      formData.workspace_id = workspaceId ? parseInt(workspaceId) : null;
    }
  }

  let canToggleGlobal = $derived(canManageGlobal && (!iteration || iteration.is_global));
</script>

<Modal
  isOpen={true}
  onclose={handleCancel}
  maxWidth="max-w-2xl"
>
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {iteration ? 'Edit Iteration' : 'Create Iteration'}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); handleSave(); }} class="space-y-4">
      <!-- Error Message -->
      {#if error}
        <div class="p-3 rounded" style="background-color: #fee; border: 1px solid #fcc;">
          <p class="text-sm" style="color: #c33;">{error}</p>
        </div>
      {/if}

      <!-- Scope Toggle -->
      <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            {#if formData.is_global}
              <Globe class="w-5 h-5" style="color: var(--ds-interactive);" />
              <div>
                <p class="font-medium text-sm" style="color: var(--ds-text);">Global Iteration</p>
                <p class="text-xs" style="color: var(--ds-text-subtle);">Visible across all workspaces</p>
              </div>
            {:else}
              <Building2 class="w-5 h-5" style="color: var(--ds-interactive);" />
              <div>
                <p class="font-medium text-sm" style="color: var(--ds-text);">Local Iteration</p>
                <p class="text-xs" style="color: var(--ds-text-subtle);">Only visible in this workspace</p>
              </div>
            {/if}
          </div>
          {#if canToggleGlobal}
            <button
              type="button"
              class="px-3 py-1.5 text-sm rounded border transition-colors"
              style="border-color: var(--ds-border); color: var(--ds-interactive);"
              onclick={toggleScope}
            >
              Switch to {formData.is_global ? 'Local' : 'Global'}
            </button>
          {/if}
        </div>
      </div>

      <!-- Name -->
      <div>
        <Label color="default" required class="mb-1.5">Name</Label>
        <Input
          bind:value={formData.name}
          placeholder="e.g., Sprint 24, Q1 2025, Release 2.0"
          required
        />
      </div>

      <!-- Description -->
      <div>
        <Label color="default" class="mb-1.5">Description</Label>
        <Textarea
          bind:value={formData.description}
          placeholder="Optional description or goals for this iteration"
          rows={3}
        />
      </div>

      <!-- Type -->
      <div>
        <Label color="default" class="mb-1.5"><Tag class="w-4 h-4 inline-block mr-1" />Type</Label>
        <Select bind:value={formData.type_id}>
          <option value={null}>No type</option>
          {#each iterationTypes as type}
            <option value={type.id}>{type.name}</option>
          {/each}
        </Select>
      </div>

      <!-- Date Range -->
      <div class="grid grid-cols-2 gap-4">
        <div>
          <Label color="default" required class="mb-1.5"><Calendar class="w-4 h-4 inline-block mr-1" />Start Date</Label>
          <Input
            type="date"
            bind:value={formData.start_date}
            required
          />
        </div>
        <div>
          <Label color="default" required class="mb-1.5"><Calendar class="w-4 h-4 inline-block mr-1" />End Date</Label>
          <Input
            type="date"
            bind:value={formData.end_date}
            required
          />
        </div>
      </div>

      <!-- Status -->
      <div>
        <Label color="default" class="mb-1.5">Status</Label>
        <Select bind:value={formData.status}>
          {#each statusOptions as status}
            <option value={status.value}>{status.label}</option>
          {/each}
        </Select>
      </div>

      <!-- Form Actions -->
      <div class="flex items-center justify-end gap-3 pt-4 border-t" style="border-color: var(--ds-border);">
        <Button
          type="button"
          variant="subtle"
          size="medium"
          onclick={handleCancel}
          disabled={saving}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          variant="primary"
          size="medium"
          disabled={saving}
        >
          {saving ? 'Saving...' : iteration ? 'Update Iteration' : 'Create Iteration'}
        </Button>
      </div>
    </form>
  </div>
</Modal>
