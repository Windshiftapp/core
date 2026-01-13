<script>
  import { createEventDispatcher } from 'svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';

  const dispatch = createEventDispatcher();

  // Props
  export let isOpen = false;
  export let formData = {
    customer_id: '',
    category_id: '',
    name: '',
    description: '',
    status: 'Active',
    color: '',
    hourly_rate: 0,
    active: true
  };
  export let customers = [];
  export let categories = [];
  export let statusOptions = ['Active', 'On Hold', 'Completed', 'Archived'];
  export let isEditing = false;

  // Color options - matching IconSelector's palette
  const colorOptions = [
    '#7c3aed', '#2563eb', '#059669', '#dc2626', '#ea580c',
    '#6b7280', '#8b5cf6', '#3b82f6', '#10b981', '#ef4444',
    '#f59e0b', '#84cc16', '#06b6d4', '#ec4899', '#f97316',
    '#64748b', '#7c2d12', '#1e40af', '#065f46', '#991b1b',
    '#92400e', '#365314', '#0e7490', '#be185d', '#9a3412'
  ];

  function selectColor(color) {
    formData.color = color;
  }

  function handleSubmit() {
    if (formData.name.trim()) {
      dispatch('save');
    }
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

{#if isOpen}
  <Modal
    {isOpen}
    onSubmit={handleSubmit}
    submitDisabled={!formData.name.trim()}
    maxWidth="max-w-2xl"
    onclose={handleCancel}
    let:submitHint
  >
    <div class="p-6">
      <h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
        {isEditing ? 'Edit Project' : 'New Project'}
      </h3>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <Label required class="mb-2">Project Name</Label>
          <Input bind:value={formData.name} required />
        </div>

        <div>
          <Label class="mb-2">Status</Label>
          <Select bind:value={formData.status}>
            {#each statusOptions as status}
              <option value={status}>{status}</option>
            {/each}
          </Select>
        </div>

        <div>
          <Label class="mb-2">Customer (Optional)</Label>
          <Select bind:value={formData.customer_id}>
            <option value="">None</option>
            {#each customers.filter(c => c.active) as customer}
              <option value={customer.id}>{customer.name}</option>
            {/each}
          </Select>
        </div>

        <div>
          <Label class="mb-2">Category (Optional)</Label>
          <Select bind:value={formData.category_id}>
            <option value="">None</option>
            {#each categories as category}
              <option value={category.id}>{category.name}</option>
            {/each}
          </Select>
        </div>
      </div>

      <div class="mt-6">
        <Label class="mb-2">Hourly Rate ($)</Label>
        <Input type="number" bind:value={formData.hourly_rate} min="0" step="0.01" />
      </div>

      <!-- Color Picker -->
      <div class="mt-6">
        <Label class="mb-2">Project Color</Label>

        <!-- Color Preview -->
        {#if formData.color}
          <div class="flex items-center gap-3 mb-3 p-3 rounded border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
            <div class="w-8 h-8 rounded flex-shrink-0" style="background-color: {formData.color};"></div>
            <div class="flex-1">
              <div class="text-sm font-medium" style="color: var(--ds-text);">{formData.color}</div>
            </div>
            <button
              onclick={() => formData.color = ''}
              class="text-sm px-3 py-1 rounded hover:bg-gray-100 transition-colors"
              style="color: var(--ds-text-subtle);"
              type="button"
            >
              Clear
            </button>
          </div>
        {/if}

        <!-- Color Grid -->
        <div class="color-grid">
          {#each colorOptions as color}
            <button
              type="button"
              class="color-option"
              class:selected={formData.color === color}
              style="background-color: {color}"
              onclick={() => selectColor(color)}
              title={color}
            ></button>
          {/each}
        </div>
      </div>

      <div class="mt-6">
        <Label class="mb-2">Description</Label>
        <Textarea bind:value={formData.description} rows={3} />
      </div>

      <div class="mt-6 flex items-center">
        <input
          type="checkbox"
          bind:checked={formData.active}
          id="active"
          class="mr-3 w-4 h-4 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
        />
        <label for="active" class="text-sm font-medium" style="color: var(--ds-text);">Active Project</label>
      </div>

      <div class="mt-8 flex gap-3">
        <Button
          variant="primary"
          onclick={handleSubmit}
          disabled={!formData.name.trim()}
          size="medium"
          keyboardHint={submitHint}
        >
          {isEditing ? 'Update' : 'Create'} Project
        </Button>
        <Button
          variant="default"
          onclick={handleCancel}
          size="medium"
          keyboardHint="Esc"
        >
          Cancel
        </Button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .color-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(32px, 1fr));
    gap: 8px;
    padding: 4px;
  }

  .color-option {
    width: 32px;
    height: 32px;
    border: 2px solid transparent;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s;
    position: relative;
  }

  .color-option:hover {
    transform: scale(1.1);
    border-color: rgba(255, 255, 255, 0.8);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  }

  .color-option.selected {
    border-color: #374151;
    box-shadow: 0 0 0 2px #3b82f6;
  }
</style>
