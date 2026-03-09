<script>
  import { createEventDispatcher } from 'svelte';
  import { createPopover, melt } from '@melt-ui/svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { Columns3, GripVertical, Check, ChevronDown, X, Plus, Lock, MoreHorizontal } from 'lucide-svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import SearchInput from '../../components/SearchInput.svelte';

  const dispatch = createEventDispatcher();

  let {
    columns = [],
    customFieldDefinitions = [],
    canConfigure = true,
    hasGradient = false
  } = $props();

  // Available system fields
  const systemFields = [
    { identifier: 'key', name: 'Key', required: true },
    { identifier: 'title', name: 'Title', required: true },
    { identifier: 'status', name: 'Status', required: false },
    { identifier: 'priority', name: 'Priority', required: false },
    { identifier: 'assignee', name: 'Assignee', required: false },
    { identifier: 'milestone', name: 'Milestone', required: false },
    { identifier: 'iteration', name: 'Iteration', required: false },
    { identifier: 'due_date', name: 'Due Date', required: false },
    { identifier: 'created_at', name: 'Created', required: false },
    { identifier: 'project', name: 'Project', required: false }
  ];

  // Width options
  const widthOptions = [
    { value: 1, label: 'S' },
    { value: 2, label: 'M' },
    { value: 3, label: 'L' },
    { value: 4, label: 'XL' }
  ];

  // Create popover
  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    forceVisible: true,
    positioning: {
      placement: 'bottom-end'
    },
    portal: 'body'
  });

  const MAX_VISIBLE_CUSTOM_FIELDS = 5;

  // Drag state
  let draggedIndex = $state(null);
  let dragOverIndex = $state(null);

  // Custom field modal state
  let showCustomFieldModal = $state(false);
  let customFieldSearchQuery = $state('');
  let modalDirty = $state(false);

  // Local copy of columns for editing
  let editableColumns = $state([...columns]);

  // Sync when columns prop changes
  $effect(() => {
    editableColumns = [...columns];
  });

  // Get visible columns (those currently configured)
  let visibleColumnIds = $derived(new Set(editableColumns.map(c => c.field_identifier)));

  // Get available columns to add (not already visible)
  let availableSystemFields = $derived(
    systemFields.filter(f => !visibleColumnIds.has(f.identifier))
  );

  let availableCustomFields = $derived(
    customFieldDefinitions.filter(f => !visibleColumnIds.has(String(f.id)))
  );

  let displayedCustomFields = $derived(
    availableCustomFields.slice(0, MAX_VISIBLE_CUSTOM_FIELDS)
  );

  let hasMoreCustomFields = $derived(
    availableCustomFields.length > MAX_VISIBLE_CUSTOM_FIELDS
  );

  let filteredModalCustomFields = $derived(
    customFieldSearchQuery.trim() === ''
      ? availableCustomFields
      : availableCustomFields.filter(f =>
          f.name.toLowerCase().includes(customFieldSearchQuery.trim().toLowerCase())
        )
  );

  // Handle drag start
  function handleDragStart(e, index) {
    // Don't allow dragging required columns (key, title)
    const col = editableColumns[index];
    if (col.field_identifier === 'key' || col.field_identifier === 'title') {
      e.preventDefault();
      return;
    }
    draggedIndex = index;
    e.dataTransfer.effectAllowed = 'move';
  }

  // Handle drag over
  function handleDragOver(e, index) {
    e.preventDefault();
    // Don't allow dropping before key or title
    if (index < 2) {
      dragOverIndex = 2;
    } else {
      dragOverIndex = index;
    }
  }

  // Handle drop
  function handleDrop(e, index) {
    e.preventDefault();
    if (draggedIndex !== null && draggedIndex !== index) {
      // Don't allow dropping before key or title
      const targetIndex = index < 2 ? 2 : index;
      const items = [...editableColumns];
      const [draggedItem] = items.splice(draggedIndex, 1);
      items.splice(targetIndex, 0, draggedItem);

      // Update display_order
      editableColumns = items.map((col, i) => ({ ...col, display_order: i }));
    }
    draggedIndex = null;
    dragOverIndex = null;
  }

  // Handle drag end
  function handleDragEnd() {
    draggedIndex = null;
    dragOverIndex = null;
  }

  // Toggle column visibility
  function toggleColumn(fieldIdentifier) {
    const index = editableColumns.findIndex(c => c.field_identifier === fieldIdentifier);
    if (index !== -1) {
      // Remove column
      editableColumns = editableColumns.filter(c => c.field_identifier !== fieldIdentifier);
      // Update display_order
      editableColumns = editableColumns.map((col, i) => ({ ...col, display_order: i }));
    }
  }

  // Add column
  function addColumn(field, fieldType) {
    const newColumn = {
      field_identifier: fieldType === 'custom' ? String(field.id) : field.identifier,
      field_type: fieldType,
      display_order: editableColumns.length,
      width: 2 // Default to medium
    };
    editableColumns = [...editableColumns, newColumn];
  }

  // Change column width
  function changeWidth(fieldIdentifier, newWidth) {
    editableColumns = editableColumns.map(col =>
      col.field_identifier === fieldIdentifier
        ? { ...col, width: newWidth }
        : col
    );
  }

  // Save changes
  function saveChanges() {
    dispatch('change', { columns: editableColumns });
    open.set(false);
  }

  // Cancel changes
  function cancelChanges() {
    editableColumns = [...columns];
    open.set(false);
  }

  // Get field name for display
  function getFieldName(column) {
    if (column.field_type === 'system') {
      const field = systemFields.find(f => f.identifier === column.field_identifier);
      return field?.name || column.field_identifier;
    } else {
      const field = customFieldDefinitions.find(f => String(f.id) === column.field_identifier);
      return field?.name || column.field_identifier;
    }
  }

  // Check if field is required
  function isRequired(fieldIdentifier) {
    return fieldIdentifier === 'key' || fieldIdentifier === 'title';
  }

  function openCustomFieldModal() {
    customFieldSearchQuery = '';
    showCustomFieldModal = true;
  }

  function closeCustomFieldModal() {
    showCustomFieldModal = false;
    customFieldSearchQuery = '';
    if (modalDirty) {
      dispatch('change', { columns: editableColumns });
      modalDirty = false;
    }
  }

  function addCustomFieldFromModal(field) {
    addColumn(field, 'custom');
    modalDirty = true;
  }
</script>

{#if canConfigure}
  <button
    use:melt={$trigger}
    class="flex items-center gap-2 px-3 py-2 text-sm rounded-lg transition-colors"
    style="color: {hasGradient ? 'rgba(255,255,255,0.85)' : 'var(--ds-text-subtle)'}; {hasGradient ? 'background: rgba(255,255,255,0.12);' : ''}"
    title="Configure columns"
  >
    <Columns3 class="w-4 h-4" />
    <span>Columns</span>
    <ChevronDown class="w-3 h-3" />
  </button>
{/if}

{#if $open}
  <div
    use:melt={$content}
    class="w-80 rounded-lg shadow-xl border z-[60]"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    <!-- Header -->
    <div class="px-4 py-3 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
      <span class="font-medium text-sm" style="color: var(--ds-text);">Configure Columns</span>
      <button
        onclick={cancelChanges}
        class="p-1 rounded hover:bg-[var(--ds-surface-hovered)] transition-colors"
        style="color: var(--ds-text-subtle);"
      >
        <X class="w-4 h-4" />
      </button>
    </div>

    <!-- Column List -->
    <div class="p-2 max-h-80 overflow-y-auto">
      <div class="text-xs font-medium px-2 py-1 mb-1" style="color: var(--ds-text-subtle);">
        Visible Columns
      </div>

      {#each editableColumns as column, index (column.field_identifier)}
        {@const required = isRequired(column.field_identifier)}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="flex items-center gap-2 px-2 py-2 rounded transition-colors {dragOverIndex === index ? 'bg-blue-50 dark:bg-blue-900/20' : ''}"
          draggable={!required}
          ondragstart={(e) => handleDragStart(e, index)}
          ondragover={(e) => handleDragOver(e, index)}
          ondrop={(e) => handleDrop(e, index)}
          ondragend={handleDragEnd}
        >
          <!-- Drag Handle -->
          <div class="flex-shrink-0 cursor-move {required ? 'opacity-30' : ''}" style="color: var(--ds-text-subtle);">
            {#if required}
              <Lock class="w-4 h-4" />
            {:else}
              <GripVertical class="w-4 h-4" />
            {/if}
          </div>

          <!-- Field Name -->
          <span class="flex-1 text-sm truncate" style="color: var(--ds-text);">
            {getFieldName(column)}
            {#if required}
              <span class="text-xs ml-1" style="color: var(--ds-text-subtle);">(required)</span>
            {/if}
          </span>

          <!-- Width Selector -->
          <div class="flex items-center gap-0.5">
            {#each widthOptions as opt}
              <button
                onclick={() => changeWidth(column.field_identifier, opt.value)}
                class="w-6 h-6 text-xs rounded transition-colors {column.width === opt.value ? 'bg-blue-500 text-white' : 'hover:bg-[var(--ds-surface-hovered)]'}"
                style="{column.width !== opt.value ? 'color: var(--ds-text-subtle);' : ''}"
                title="Width: {opt.label}"
              >
                {opt.label}
              </button>
            {/each}
          </div>

          <!-- Remove Button (if not required) -->
          {#if !required}
            <button
              onclick={() => toggleColumn(column.field_identifier)}
              class="p-1 rounded hover:bg-red-100 dark:hover:bg-red-900/20 transition-colors"
              style="color: var(--ds-text-subtle);"
              title="Remove column"
            >
              <X class="w-3 h-3" />
            </button>
          {:else}
            <div class="w-5"></div>
          {/if}
        </div>
      {/each}

      <!-- Add Column Section -->
      {#if availableSystemFields.length > 0 || availableCustomFields.length > 0}
        <div class="border-t mt-2 pt-2" style="border-color: var(--ds-border);">
          <div class="text-xs font-medium px-2 py-1 mb-1" style="color: var(--ds-text-subtle);">
            Add Column
          </div>

          {#if availableSystemFields.length > 0}
            <div class="px-2 mb-2">
              <div class="text-xs px-1 py-0.5 mb-1" style="color: var(--ds-text-subtlest);">System Fields</div>
              <div class="flex flex-wrap gap-1">
                {#each availableSystemFields as field}
                  <button
                    onclick={() => addColumn(field, 'system')}
                    class="flex items-center gap-1 px-2 py-1 text-xs rounded-full border transition-colors hover:bg-[var(--ds-surface-hovered)]"
                    style="border-color: var(--ds-border); color: var(--ds-text);"
                  >
                    <Plus class="w-3 h-3" />
                    {field.name}
                  </button>
                {/each}
              </div>
            </div>
          {/if}

          {#if availableCustomFields.length > 0}
            <div class="px-2">
              <div class="text-xs px-1 py-0.5 mb-1" style="color: var(--ds-text-subtlest);">Custom Fields</div>
              <div class="flex flex-wrap gap-1">
                {#each displayedCustomFields as field}
                  <button
                    onclick={() => addColumn(field, 'custom')}
                    class="flex items-center gap-1 px-2 py-1 text-xs rounded-full border transition-colors hover:bg-[var(--ds-surface-hovered)]"
                    style="border-color: var(--ds-border); color: var(--ds-text);"
                  >
                    <Plus class="w-3 h-3" />
                    {field.name}
                  </button>
                {/each}
                {#if hasMoreCustomFields}
                  <button
                    onclick={openCustomFieldModal}
                    class="flex items-center gap-1 px-2 py-1 text-xs rounded-full border transition-colors hover:bg-[var(--ds-surface-hovered)]"
                    style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
                    title="Browse all custom fields"
                  >
                    <MoreHorizontal class="w-3 h-3" />
                  </button>
                {/if}
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div class="px-4 py-3 border-t flex items-center justify-end gap-2" style="border-color: var(--ds-border);">
      <button
        onclick={cancelChanges}
        class="px-3 py-1.5 text-sm rounded transition-colors hover:bg-[var(--ds-surface-hovered)]"
        style="color: var(--ds-text-subtle);"
      >
        Cancel
      </button>
      <button
        onclick={saveChanges}
        class="px-3 py-1.5 text-sm rounded bg-blue-500 text-white transition-colors hover:bg-blue-600"
      >
        Apply
      </button>
    </div>
  </div>
{/if}

<Modal isOpen={showCustomFieldModal} onclose={closeCustomFieldModal} maxWidth="max-w-md">
  <ModalHeader
    title="Add Custom Field Column"
    subtitle="{availableCustomFields.length} fields available"
    onClose={closeCustomFieldModal}
  />
  <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
    <SearchInput bind:value={customFieldSearchQuery} placeholder="Search custom fields..." size="small" />
  </div>
  <div class="max-h-80 overflow-y-auto">
    {#if filteredModalCustomFields.length === 0}
      <div class="px-4 py-8 text-center text-sm" style="color: var(--ds-text-subtle);">
        No custom fields match "{customFieldSearchQuery}"
      </div>
    {:else}
      {#each filteredModalCustomFields as field}
        <button
          onclick={() => addCustomFieldFromModal(field)}
          class="w-full flex items-center gap-2 px-4 py-2.5 text-sm text-left transition-colors hover:bg-[var(--ds-surface-hovered)]"
          style="color: var(--ds-text);"
        >
          <Plus class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
          <span class="truncate">{field.name}</span>
        </button>
      {/each}
    {/if}
  </div>
</Modal>
