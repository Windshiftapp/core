<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../api.js';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import { Plus, Edit, Trash2, Settings, MoreHorizontal, Circle, Layout } from 'lucide-svelte';
  import { SYSTEM_FIELDS, getSystemFieldName } from '../stores/fieldConfig.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import DropIndicator from '../layout/DropIndicator.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { matchesShortcut } from '../utils/keyboardShortcuts.js';

  let screens = $state([]);
  let customFields = $state([]);
  let showCreateForm = $state(false);
  let editingScreen = $state(null);
  let showFieldEditor = $state(false);
  let editingScreenFields = $state(null);
  let formData = $state({
    name: '',
    description: ''
  });

  let screenFields = $state([]);


  const fieldWidths = [
    { value: 'full', label: 'Full Width' },
    { value: 'half', label: 'Half Width' },
    { value: 'third', label: 'One Third' },
    { value: 'quarter', label: 'One Quarter' }
  ];

  onMount(async () => {
    await loadScreens();

    // Add global keyboard listener
    window.addEventListener('keydown', handleGlobalKeydown);
  });

  onDestroy(() => {
    window.removeEventListener('keydown', handleGlobalKeydown);
  });

  function handleGlobalKeydown(event) {
    // 'a' to add/create new screen
    if (matchesShortcut(event, { key: 'a' }) && !showCreateForm) {
      const target = event.target;
      if (target.tagName !== 'INPUT' && target.tagName !== 'TEXTAREA' && !target.contentEditable.includes('true')) {
        event.preventDefault();
        startCreate();
      }
    }
  }

  async function loadScreens() {
    try {
      const result = await api.screens.getAll();
      screens = result || [];
    } catch (error) {
      console.error('Failed to load screens:', error);
      screens = [];
    }
  }

  async function loadCustomFields() {
    try {
      const result = await api.customFields.getAll();
      customFields = result || [];
    } catch (error) {
      console.error('Failed to load custom fields:', error);
      customFields = [];
    }
  }

  function startCreate() {
    showCreateForm = true;
    editingScreen = null;
    resetForm();
  }

  function startEdit(screen) {
    editingScreen = screen;
    formData = {
      name: screen.name,
      description: screen.description || ''
    };
    showCreateForm = true;
  }

  async function startEditFields(screen) {
    editingScreenFields = screen;
    showFieldEditor = true;
    
    try {
      const fields = await api.screens.getFields(screen.id);
      screenFields = fields || [];
      
      // Ensure Title field is always present and first
      const titleField = screenFields.find(f => f.field_type === 'system' && f.field_identifier === 'title');
      if (!titleField) {
        const newTitleField = {
          screen_id: screen.id,
          field_type: 'system',
          field_identifier: 'title',
          display_order: 0,
          is_required: true,
          field_width: 'full',
          field_name: 'Title',
          field_label: 'Title'
        };
        screenFields = [newTitleField, ...screenFields.map(f => ({ ...f, display_order: f.display_order + 1 }))];
      }

      // Ensure Status field is always present (after title)
      const statusField = screenFields.find(f => f.field_type === 'system' && f.field_identifier === 'status');
      if (!statusField) {
        const newStatusField = {
          screen_id: screen.id,
          field_type: 'system',
          field_identifier: 'status',
          display_order: 1,
          is_required: false,
          field_width: 'half',
          field_name: 'Status',
          field_label: 'Status'
        };
        // Insert after title
        const titleIndex = screenFields.findIndex(f => f.field_type === 'system' && f.field_identifier === 'title');
        const insertIndex = titleIndex >= 0 ? titleIndex + 1 : 0;
        screenFields = [
          ...screenFields.slice(0, insertIndex),
          newStatusField,
          ...screenFields.slice(insertIndex).map(f => ({ ...f, display_order: f.display_order + 1 }))
        ];
      }
      
      await loadCustomFields();
    } catch (error) {
      console.error('Failed to load screen fields:', error);
      screenFields = [];
    }
  }

  function resetForm() {
    formData = {
      name: '',
      description: ''
    };
  }

  function cancelForm() {
    showCreateForm = false;
    editingScreen = null;
    resetForm();
  }

  function cancelFieldEditor() {
    showFieldEditor = false;
    editingScreenFields = null;
    screenFields = [];
    customFields = [];
    fieldSearchQuery = '';
    cleanupDragAndDrop();
  }

  async function saveScreen() {
    try {
      if (editingScreen) {
        await api.screens.update(editingScreen.id, formData);
      } else {
        await api.screens.create(formData);
      }
      
      await loadScreens();
      cancelForm();
    } catch (error) {
      console.error('Failed to save screen:', error);
      alert('Failed to save screen: ' + (error.message || error));
    }
  }

  async function saveScreenFields() {
    try {
      await api.screens.updateFields(editingScreenFields.id, screenFields);
      cancelFieldEditor();
    } catch (error) {
      console.error('Failed to save screen fields:', error);
      alert('Failed to save screen fields: ' + (error.message || error));
    }
  }

  async function deleteScreen(screen) {
    // Prevent deletion of default screen (ID 1)
    if (screen.id === 1) {
      alert('Cannot delete the default screen. This screen is required for workspaces without a configuration set.');
      return;
    }

    if (confirm(`Are you sure you want to delete screen "${screen.name}"? This will affect all workspaces using this screen.`)) {
      try {
        await api.screens.delete(screen.id);
        await loadScreens();
      } catch (error) {
        console.error('Failed to delete screen:', error);
        alert('Failed to delete screen: ' + (error.message || error));
      }
    }
  }


  // Drag and drop state
  let draggedField = $state(null);
  let fieldDragState = $state(new Map()); // Track { closestEdge: 'top'|'bottom'|null } for each field
  let setupCleanups = [];
  let setupTimeout;

  onDestroy(() => {
    cleanupDragAndDrop();
  });

  function cleanupDragAndDrop() {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupCleanups.forEach(fn => fn());
    setupCleanups = [];
    fieldDragState = new Map();
  }

  function setupDragAndDrop() {
    cleanupDragAndDrop();

    // Setup available fields as draggable
    document.querySelectorAll('[data-available-field]').forEach(element => {
      const fieldData = JSON.parse(element.dataset.availableField);

      const cleanup = draggable({
        element,
        getInitialData: () => ({ field: fieldData, type: 'available-field' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => { element.style.opacity = ''; }
      });

      setupCleanups.push(cleanup);
    });

    // Setup screen fields as both draggable and drop targets with edge detection
    document.querySelectorAll('[data-screen-field]').forEach(element => {
      const fieldIndex = parseInt(element.dataset.fieldIndex);
      const fieldId = element.dataset.fieldId;

      fieldDragState.set(fieldId, { closestEdge: null });

      // Make draggable
      const dragHandle = element.querySelector('.cursor-grab');
      const draggableCleanup = draggable({
        element,
        dragHandle: dragHandle || element,
        getInitialData: () => ({ fieldIndex, fieldId, type: 'screen-field' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => {
          element.style.opacity = '';
          fieldDragState.forEach((state, id) => {
            fieldDragState.set(id, { closestEdge: null });
          });
          fieldDragState = new Map(fieldDragState);
        }
      });

      // Make drop target with edge detection
      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'screen-field' && data.fieldIndex === fieldIndex) return false;
          return data.type === 'available-field' || data.type === 'screen-field';
        },
        getData: ({ input, element }) => {
          return attachClosestEdge({}, { input, element, allowedEdges: ['top', 'bottom'] });
        },
        onDragEnter: ({ self }) => {
          const closestEdge = extractClosestEdge(self.data);
          fieldDragState.set(fieldId, { closestEdge });
          fieldDragState = new Map(fieldDragState);
        },
        onDragLeave: () => {
          fieldDragState.set(fieldId, { closestEdge: null });
          fieldDragState = new Map(fieldDragState);
        },
        onDrop: ({ self, source }) => {
          const closestEdge = extractClosestEdge(self.data);
          const data = source.data;

          if (data.type === 'available-field') {
            addFieldAtPosition(data.field, fieldIndex, closestEdge);
          } else if (data.type === 'screen-field') {
            reorderFieldWithEdge(data.fieldIndex, fieldIndex, closestEdge);
          }

          fieldDragState.set(fieldId, { closestEdge: null });
          fieldDragState = new Map(fieldDragState);
        }
      });

      setupCleanups.push(() => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });

    // Setup drop zone for empty area / append to end
    const dropZone = document.querySelector('[data-drop-zone]');
    if (dropZone) {
      const cleanup = dropTargetForElements({
        element: dropZone,
        canDrop: ({ source }) => source.data.type === 'available-field',
        onDragEnter: () => { draggedField = true; },
        onDragLeave: () => { draggedField = null; },
        onDrop: ({ source }) => {
          if (source.data.type === 'available-field') {
            addFieldToScreen(source.data.field);
          }
          draggedField = null;
        }
      });
      setupCleanups.push(cleanup);
    }
  }

  // Re-setup drag and drop when field editor is shown or fields change
  $effect(() => {
    if (showFieldEditor && screenFields && typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => setupDragAndDrop(), 50);
    }
  });

  function addFieldToScreen(fieldData) {
    // Check if field already exists
    if (screenFields.some(f => f.field_type === fieldData.type && f.field_identifier === fieldData.identifier)) {
      return;
    }

    const newField = {
      screen_id: editingScreenFields.id,
      field_type: fieldData.type,
      field_identifier: fieldData.identifier,
      display_order: screenFields.length,
      is_required: fieldData.identifier === 'title',
      field_width: 'full',
      field_name: fieldData.name,
      field_label: fieldData.name
    };

    if (fieldData.type === 'custom') {
      newField.field_config = fieldData.config;
    }

    screenFields = [...screenFields, newField];
  }

  function addFieldAtPosition(fieldData, targetIndex, closestEdge) {
    // Check if field already exists
    if (screenFields.some(f => f.field_type === fieldData.type && f.field_identifier === fieldData.identifier)) {
      return;
    }

    const insertIndex = closestEdge === 'bottom' ? targetIndex + 1 : targetIndex;

    const newField = {
      screen_id: editingScreenFields.id,
      field_type: fieldData.type,
      field_identifier: fieldData.identifier,
      display_order: insertIndex,
      is_required: fieldData.identifier === 'title',
      field_width: 'full',
      field_name: fieldData.name,
      field_label: fieldData.name
    };

    if (fieldData.type === 'custom') {
      newField.field_config = fieldData.config;
    }

    const newFields = [...screenFields];
    newFields.splice(insertIndex, 0, newField);
    screenFields = newFields.map((f, i) => ({ ...f, display_order: i }));
  }

  function reorderFieldWithEdge(fromIndex, toIndex, closestEdge) {
    if (fromIndex === toIndex) return;

    const insertIndex = closestEdge === 'bottom' ? toIndex + 1 : toIndex;
    const adjustedInsertIndex = fromIndex < insertIndex ? insertIndex - 1 : insertIndex;

    const newFields = [...screenFields];
    const [movedField] = newFields.splice(fromIndex, 1);
    newFields.splice(adjustedInsertIndex, 0, movedField);

    screenFields = newFields.map((f, i) => ({ ...f, display_order: i }));
  }

  function removeField(index) {
    const field = screenFields[index];

    // Prevent removing the Title and Status fields
    if (field.field_type === 'system' && (field.field_identifier === 'title' || field.field_identifier === 'status')) {
      return;
    }

    screenFields = screenFields.filter((_, i) => i !== index);
    screenFields = screenFields.map((field, i) => ({ ...field, display_order: i }));
  }

  // Create combined available fields list
  let allAvailableFields = $derived.by(() => [
    // System fields from shared config
    ...SYSTEM_FIELDS.map(field => ({
      ...field,
      type: 'system',
      category: 'System Fields'
    })),
    // Custom fields
    ...customFields.map(field => ({
      identifier: field.id.toString(),
      name: field.field_name || field.name,
      type: 'custom',
      category: 'Custom Fields',
      fieldType: field.field_type,
      config: field.field_config
    }))
  ]);

  let availableFieldsFiltered = $derived.by(() =>
    allAvailableFields.filter(field =>
      !screenFields.some(sf => sf.field_type === field.type && sf.field_identifier === field.identifier)
    ).filter(field =>
      // Filter out Title and Status fields since they're always auto-added
      !(field.type === 'system' && (field.identifier === 'title' || field.identifier === 'status'))
    )
  );

  // Search filter for available fields
  let fieldSearchQuery = $state('');

  let searchFilteredFields = $derived.by(() =>
    availableFieldsFiltered.filter(field => {
      if (!fieldSearchQuery.trim()) return true;
      const query = fieldSearchQuery.toLowerCase();
      return field.name.toLowerCase().includes(query) ||
             field.identifier.toLowerCase().includes(query);
    })
  );

  function getFieldWidthLabel(width) {
    return fieldWidths.find(w => w.value === width)?.label || width;
  }

  function getFieldDisplayName(field) {
    // For system fields, use the shared config
    if (field.field_type === 'system') {
      return getSystemFieldName(field.field_identifier);
    }
    // For custom fields, use the field_name from the API
    return field.field_name || field.field_identifier;
  }

  function buildScreenDropdownItems(screen) {
    const items = [
      {
        id: 'fields',
        type: 'regular',
        icon: Settings,
        title: 'Fields',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => startEditFields(screen)
      },
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => startEdit(screen)
      }
    ];

    // Don't show delete option for default screen (ID 1)
    if (screen.id !== 1) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteScreen(screen)
      });
    }

    return items;
  }

  // Table column definitions
  const screenColumns = [
    {
      key: 'name',
      label: 'Screen',
      slot: 'name'
    },
    {
      key: 'created_at',
      label: 'Created',
      render: (screen) => new Date(screen.created_at).toLocaleDateString(),
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];
</script>

{#if !showFieldEditor}
  <PageHeader
    icon={Layout}
    title="Screens"
    subtitle="Define field layouts for issues and projects"
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        onclick={startCreate}
        keyboardHint="A"
      >
        Add Screen
      </Button>
    {/snippet}
  </PageHeader>


    <Modal isOpen={showCreateForm} onclose={cancelForm} maxWidth="max-w-lg">
      <!-- Modal header -->
      <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
        <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
          {editingScreen ? 'Edit Screen' : 'Create Screen'}
        </h3>
      </div>

      <!-- Modal content -->
      <div class="px-6 py-4">
        <form onsubmit={(e) => { e.preventDefault(); saveScreen(); }}>
          <div class="grid grid-cols-1 gap-6">
            <div>
              <Label for="screen-name" required class="mb-2">Screen Name</Label>
              <Input
                id="screen-name"
                bind:value={formData.name}
                placeholder="e.g., Standard Issue Form, Bug Report Form"
                required
              />
            </div>

            <div>
              <Label for="screen-description" class="mb-2">Description</Label>
              <Textarea
                id="screen-description"
                bind:value={formData.description}
                rows={3}
                placeholder="Optional description for this screen"
              />
            </div>
          </div>
        </form>
      </div>

      <DialogFooter
        onCancel={cancelForm}
        onConfirm={saveScreen}
        confirmLabel={editingScreen ? 'Update Screen' : 'Create Screen'}
        disabled={!formData.name.trim()}
      />
    </Modal>

    <DataTable
      columns={screenColumns}
      data={screens}
      keyField="id"
      emptyMessage="No screens found. Create your first screen to get started."
      emptyIcon={Circle}
      actionItems={buildScreenDropdownItems}
    >
      <div slot="name" let:item={screen}>
        <div class="font-semibold" style="color: var(--ds-text);">{screen.name}</div>
        {#if screen.description}
          <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{screen.description}</div>
        {/if}
      </div>
    </DataTable>
  {:else}
    <PageHeader
      icon={Settings}
      title="Configure Fields"
      subtitle="Screen: {editingScreenFields?.name}"
    >
      {#snippet actions()}
        <div class="flex gap-3">
          <Button
            variant="primary"
            onclick={saveScreenFields}
          >
            Save Fields
          </Button>
          <Button
            variant="default"
            onclick={cancelFieldEditor}
          >
            Cancel
          </Button>
        </div>
      {/snippet}
    </PageHeader>

    <!-- Drag and Drop Field Editor -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
      <!-- Available Fields -->
      <div class="rounded-xl p-6 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Available Fields</h3>
        <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">Drag fields to the screen configuration on the right</p>

        <Input
          placeholder="Search fields..."
          bind:value={fieldSearchQuery}
          class="mb-4"
        />

        <div class="space-y-1 min-h-96 max-h-[70vh] overflow-y-auto">
          {#each searchFilteredFields as field, index}
            {#if index === 0 || field.category !== searchFilteredFields[index - 1].category}
              <div class="text-xs font-semibold text-gray-500 uppercase tracking-wider mt-4 mb-2 first:mt-0">
                {field.category}
              </div>
            {/if}
            
            <div
              data-available-field={JSON.stringify(field)}
              class="group flex items-center gap-3 px-4 py-3 rounded border transition-all duration-200 cursor-grab hover:bg-blue-50 hover:border-blue-300 active:cursor-grabbing"
              style="border-color: var(--ds-border); background-color: var(--ds-background-input); user-select: none; -webkit-user-select: none;"
            >
              <!-- Drag Handle -->
              <div class="flex-shrink-0">
                <svg class="w-4 h-4 text-gray-400 group-hover:text-blue-500" fill="currentColor" viewBox="0 0 24 24">
                  <circle cx="9" cy="6" r="1.5"/>
                  <circle cx="15" cy="6" r="1.5"/>
                  <circle cx="9" cy="12" r="1.5"/>
                  <circle cx="15" cy="12" r="1.5"/>
                  <circle cx="9" cy="18" r="1.5"/>
                  <circle cx="15" cy="18" r="1.5"/>
                </svg>
              </div>
              
              <div class="flex-1">
                <div class="font-medium" style="color: var(--ds-text);">{field.name}</div>
                <div class="text-sm" style="color: var(--ds-text-subtle);">
                  {#if field.type === 'system'}
                    {SYSTEM_FIELDS.find(sf => sf.identifier === field.identifier)?.type || field.identifier}
                    • System
                  {:else}
                    {field.fieldType}
                    • Custom
                  {/if}
                </div>
              </div>
            </div>
          {/each}
          
          {#if searchFilteredFields.length === 0}
            <div class="text-center py-8">
              <p class="text-sm" style="color: var(--ds-text-subtle);">
                {#if fieldSearchQuery.trim()}
                  No fields match your search
                {:else}
                  All available fields have been added to the screen
                {/if}
              </p>
            </div>
          {/if}
        </div>
      </div>

      <!-- Screen Fields Configuration -->
      <div class="rounded-xl p-6 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Screen Fields ({screenFields.length})</h3>
        <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">Drag to reorder fields or drop available fields here</p>
        
        <div
          data-drop-zone
          class="min-h-96 max-h-[70vh] overflow-y-auto border-2 border-dashed border-gray-200 rounded p-4 space-y-2"
          class:border-blue-400={draggedField}
          class:bg-blue-50={draggedField}
        >
          {#if screenFields.length === 0}
            <div class="text-center py-12">
              <svg class="w-12 h-12 text-gray-300 mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"/>
              </svg>
              <p class="text-sm text-gray-500">Drop fields here to configure the screen</p>
            </div>
          {:else}
            {#each screenFields as field, index (field.field_identifier)}
              <div
                data-screen-field
                data-field-index={index}
                data-field-id={field.field_identifier}
                class="relative group flex items-center gap-3 px-4 py-3 rounded border bg-white hover:shadow-sm transition-all duration-200 h-16"
                style="border-color: var(--ds-border); user-select: none;"
              >
                <!-- Drop indicator -->
                {#if fieldDragState.get(field.field_identifier)?.closestEdge}
                  <DropIndicator edge={fieldDragState.get(field.field_identifier)?.closestEdge} gap={8} />
                {/if}
                <!-- Drag Handle -->
                <div 
                  class="cursor-grab active:cursor-grabbing flex-shrink-0 p-1 rounded hover:bg-gray-100 transition-colors"
                  style="touch-action: none;"
                >
                  <svg class="w-4 h-4 text-gray-400 group-hover:text-blue-500" fill="currentColor" viewBox="0 0 24 24">
                    <circle cx="9" cy="6" r="1.5"/>
                    <circle cx="15" cy="6" r="1.5"/>
                    <circle cx="9" cy="12" r="1.5"/>
                    <circle cx="15" cy="12" r="1.5"/>
                    <circle cx="9" cy="18" r="1.5"/>
                    <circle cx="15" cy="18" r="1.5"/>
                  </svg>
                </div>
                
                <div class="flex-1">
                  <div class="font-medium flex items-center gap-2" style="color: var(--ds-text);">
                    {getFieldDisplayName(field)}
                    <span class="text-xs px-1.5 py-0.5 rounded text-gray-500 bg-gray-100">
                      {field.field_type === 'system' ? 'system' : 'custom'}
                    </span>
                  </div>
                </div>
                
                <div class="flex items-center gap-3 flex-shrink-0">
                  <label class="flex items-center gap-1">
                    <input
                      type="checkbox"
                      bind:checked={field.is_required}
                      class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                    />
                    <span class="text-xs text-gray-600">Required</span>
                  </label>
                  
                  {#if field.field_type === 'system' && (field.field_identifier === 'title' || field.field_identifier === 'status')}
                    <div class="w-9 h-9 flex items-center justify-center flex-shrink-0">
                      <svg class="w-4 h-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/>
                      </svg>
                    </div>
                  {:else}
                    <button
                      onclick={() => removeField(index)}
                      class="text-red-500 hover:text-red-700 transition-colors p-1 rounded hover:bg-red-50 flex-shrink-0"
                      title="Remove field"
                    >
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                      </svg>
                    </button>
                  {/if}
                </div>
              </div>
            {/each}
          {/if}
        </div>
      </div>
    </div>

  {/if}