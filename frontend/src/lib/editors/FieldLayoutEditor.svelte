<script>
  import { onMount, onDestroy } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import { Settings } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DropIndicator from '../layout/DropIndicator.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = $bindable(false),
    title = '',
    subtitle = '',
    availableFields = [],            // Fields that can be selected: { identifier, name, type?, fieldType?, category?, description? }
    selectedFields = $bindable([]),  // Currently selected fields: { field_identifier, field_name, display_order, is_required?, ... }
    showRequiredToggle = true,
    protectedFieldIds = [],          // Field identifiers that cannot be removed
    showTypeLabels = true,           // Show system/custom badges
    onSave = () => {},
    onCancel = () => {}
  } = $props();

  const effectiveTitle = $derived(title || t('fields.configureFields'));

  // Search state
  let fieldSearchQuery = $state('');

  // Drag and drop state
  let draggedField = $state(null);
  let fieldDragState = $state(new Map());
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
    document.querySelectorAll('[data-available-field-editor]').forEach(element => {
      const fieldData = JSON.parse(element.dataset.availableFieldEditor);

      const cleanup = draggable({
        element,
        getInitialData: () => ({ field: fieldData, type: 'available-field' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => { element.style.opacity = ''; }
      });

      setupCleanups.push(cleanup);
    });

    // Setup selected fields as both draggable and drop targets with edge detection
    document.querySelectorAll('[data-selected-field-editor]').forEach(element => {
      const fieldIndex = parseInt(element.dataset.fieldIndex);
      const fieldId = element.dataset.fieldId;

      fieldDragState.set(fieldId, { closestEdge: null });

      // Make draggable
      const dragHandle = element.querySelector('.cursor-grab');
      const draggableCleanup = draggable({
        element,
        dragHandle: dragHandle || element,
        getInitialData: () => ({ fieldIndex, fieldId, type: 'selected-field' }),
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
          if (data.type === 'selected-field' && data.fieldIndex === fieldIndex) return false;
          return data.type === 'available-field' || data.type === 'selected-field';
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
          } else if (data.type === 'selected-field') {
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
    const dropZone = document.querySelector('[data-drop-zone-editor]');
    if (dropZone) {
      const cleanup = dropTargetForElements({
        element: dropZone,
        canDrop: ({ source }) => source.data.type === 'available-field',
        onDragEnter: () => { draggedField = true; },
        onDragLeave: () => { draggedField = null; },
        onDrop: ({ source }) => {
          if (source.data.type === 'available-field') {
            addFieldToEnd(source.data.field);
          }
          draggedField = null;
        }
      });
      setupCleanups.push(cleanup);
    }
  }

  // Re-setup drag and drop when modal is shown or fields change
  $effect(() => {
    if (isOpen && selectedFields && typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => setupDragAndDrop(), 50);
    }
  });

  function addFieldToEnd(fieldData) {
    // Check if field already exists
    const identifier = fieldData.identifier || fieldData.id?.toString();
    if (selectedFields.some(f => f.field_identifier === identifier)) {
      return;
    }

    const newField = {
      field_identifier: identifier,
      field_type: fieldData.type || 'custom',
      field_name: fieldData.name,
      display_order: selectedFields.length,
      is_required: false
    };

    selectedFields = [...selectedFields, newField];
  }

  function addFieldAtPosition(fieldData, targetIndex, closestEdge) {
    // Check if field already exists
    const identifier = fieldData.identifier || fieldData.id?.toString();
    if (selectedFields.some(f => f.field_identifier === identifier)) {
      return;
    }

    const insertIndex = closestEdge === 'bottom' ? targetIndex + 1 : targetIndex;

    const newField = {
      field_identifier: identifier,
      field_type: fieldData.type || 'custom',
      field_name: fieldData.name,
      display_order: insertIndex,
      is_required: false
    };

    const newFields = [...selectedFields];
    newFields.splice(insertIndex, 0, newField);
    selectedFields = newFields.map((f, i) => ({ ...f, display_order: i }));
  }

  function reorderFieldWithEdge(fromIndex, toIndex, closestEdge) {
    if (fromIndex === toIndex) return;

    const insertIndex = closestEdge === 'bottom' ? toIndex + 1 : toIndex;
    const adjustedInsertIndex = fromIndex < insertIndex ? insertIndex - 1 : insertIndex;

    const newFields = [...selectedFields];
    const [movedField] = newFields.splice(fromIndex, 1);
    newFields.splice(adjustedInsertIndex, 0, movedField);

    selectedFields = newFields.map((f, i) => ({ ...f, display_order: i }));
  }

  function removeField(index) {
    const field = selectedFields[index];

    // Prevent removing protected fields
    if (protectedFieldIds.includes(field.field_identifier)) {
      return;
    }

    selectedFields = selectedFields.filter((_, i) => i !== index);
    selectedFields = selectedFields.map((field, i) => ({ ...field, display_order: i }));
  }

  // Filter available fields to exclude already selected ones
  let availableFieldsFiltered = $derived.by(() =>
    availableFields.filter(field => {
      const identifier = field.identifier || field.id?.toString();
      return !selectedFields.some(sf => sf.field_identifier === identifier);
    })
  );

  // Apply search filter
  let searchFilteredFields = $derived.by(() =>
    availableFieldsFiltered.filter(field => {
      if (!fieldSearchQuery.trim()) return true;
      const query = fieldSearchQuery.toLowerCase();
      const identifier = field.identifier || field.id?.toString() || '';
      return field.name.toLowerCase().includes(query) ||
             identifier.toLowerCase().includes(query);
    })
  );

  // Group fields by category
  let groupedFields = $derived.by(() => {
    const groups = {};
    for (const field of searchFilteredFields) {
      const category = field.category || 'Fields';
      if (!groups[category]) {
        groups[category] = [];
      }
      groups[category].push(field);
    }
    return groups;
  });

  function handleClose() {
    fieldSearchQuery = '';
    cleanupDragAndDrop();
    onCancel();
  }

  function handleSave() {
    fieldSearchQuery = '';
    cleanupDragAndDrop();
    onSave();
  }

  function getFieldDisplayName(field) {
    return field.field_name || field.field_identifier;
  }

  function isProtected(field) {
    return protectedFieldIds.includes(field.field_identifier);
  }
</script>

<Modal {isOpen} onclose={handleClose} maxWidth="max-w-4xl">
  <ModalHeader title={effectiveTitle} {subtitle} icon={Settings} onClose={handleClose} />

  <div class="p-6">
    <!-- Two-panel layout -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- Available Fields (Left Panel) -->
      <div class="rounded-xl p-4 border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
        <h4 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">{t('editors.availableFields')}</h4>
        <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">{t('editors.dragFieldsToAdd')}</p>

        <Input
          placeholder={t('fields.searchFields')}
          bind:value={fieldSearchQuery}
          class="mb-3"
        />

        <div class="space-y-1 min-h-48 max-h-[50vh] overflow-y-auto" style="overscroll-behavior: contain;">
          {#each Object.entries(groupedFields) as [category, fields]}
            <div class="text-xs font-semibold uppercase tracking-wider mt-3 mb-2 first:mt-0" style="color: var(--ds-text-subtlest);">
              {category}
            </div>

            {#each fields as field}
              {@const identifier = field.identifier || field.id?.toString()}
              <div
                data-available-field-editor={JSON.stringify({ ...field, identifier })}
                class="group flex items-center gap-3 px-3 py-2 rounded border transition-all duration-200 cursor-grab hover:border-blue-300 active:cursor-grabbing"
                style="border-color: var(--ds-border); background-color: var(--ds-background-input); user-select: none; -webkit-user-select: none;"
                onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-hovered)'}
                onmouseleave={(e) => e.currentTarget.style.background = 'var(--ds-background-input)'}
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

                <div class="flex-1 min-w-0">
                  <div class="font-medium text-sm truncate" style="color: var(--ds-text);">{field.name}</div>
                  {#if field.fieldType || field.description}
                    <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
                      {field.fieldType || ''}{field.fieldType && field.description ? ' - ' : ''}{field.description || ''}
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          {/each}

          {#if searchFilteredFields.length === 0}
            <div class="text-center py-6">
              <p class="text-sm" style="color: var(--ds-text-subtle);">
                {#if fieldSearchQuery.trim()}
                  {t('editors.noFieldsMatchSearch')}
                {:else if availableFields.length === 0}
                  {t('editors.noFieldsAvailable')}
                {:else}
                  {t('editors.allFieldsAdded')}
                {/if}
              </p>
            </div>
          {/if}
        </div>
      </div>

      <!-- Selected Fields (Right Panel) -->
      <div class="rounded-xl p-4 border" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
        <h4 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">{t('editors.selectedFields')} ({selectedFields.length})</h4>
        <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">{t('editors.dragToReorderOrDrop')}</p>

        <div
          data-drop-zone-editor
          class="min-h-48 max-h-[50vh] overflow-y-auto border-2 border-dashed rounded p-3 space-y-2"
          style="border-color: var(--ds-border); overscroll-behavior: contain;"
          class:border-blue-400={draggedField}
          class:bg-blue-50={draggedField}
        >
          {#if selectedFields.length === 0}
            <div class="text-center py-8">
              <svg class="w-10 h-10 text-gray-300 mx-auto mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"/>
              </svg>
              <p class="text-sm text-gray-500">{t('editors.dropFieldsHere')}</p>
            </div>
          {:else}
            {#each selectedFields as field, index (field.field_identifier)}
              <div
                data-selected-field-editor
                data-field-index={index}
                data-field-id={field.field_identifier}
                class="relative group flex items-center gap-3 px-3 py-2 rounded border bg-white hover:shadow-sm transition-all duration-200"
                style="border-color: var(--ds-border); user-select: none;"
              >
                <!-- Drop indicator -->
                {#if fieldDragState.get(field.field_identifier)?.closestEdge}
                  <DropIndicator edge={fieldDragState.get(field.field_identifier)?.closestEdge} gap={8} />
                {/if}

                <!-- Drag Handle -->
                <div
                  class="cursor-grab active:cursor-grabbing flex-shrink-0 p-1 rounded hover-bg transition-colors"
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

                <div class="flex-1 min-w-0">
                  <div class="font-medium text-sm flex items-center gap-2" style="color: var(--ds-text);">
                    <span class="truncate">{getFieldDisplayName(field)}</span>
                    {#if showTypeLabels && field.field_type}
                      <span class="text-xs px-1.5 py-0.5 rounded text-gray-500 bg-gray-100 flex-shrink-0">
                        {field.field_type}
                      </span>
                    {/if}
                  </div>
                </div>

                <div class="flex items-center gap-2 flex-shrink-0">
                  {#if showRequiredToggle}
                    <label class="flex items-center gap-1">
                      <input
                        type="checkbox"
                        bind:checked={field.is_required}
                        class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                      />
                      <span class="text-xs text-gray-600">{t('common.required')}</span>
                    </label>
                  {/if}

                  {#if isProtected(field)}
                    <div class="w-8 h-8 flex items-center justify-center flex-shrink-0">
                      <svg class="w-4 h-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/>
                      </svg>
                    </div>
                  {:else}
                    <button
                      onclick={() => removeField(index)}
                      class="text-red-500 hover:text-red-700 transition-colors p-1 rounded hover:bg-red-50 flex-shrink-0"
                      title={t('aria.removeField')}
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

    <!-- Footer -->
    <div class="flex justify-end gap-3 mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
      <Button variant="outline" onclick={handleClose}>{t('common.cancel')}</Button>
      <Button onclick={handleSave}>{t('common.save')}</Button>
    </div>
  </div>
</Modal>
