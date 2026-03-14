<script>
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import { api } from '../api.js';
  import { Plus, Trash2, GripVertical, Pencil, Type, AlignLeft, ListChecks, ToggleLeft, AlertTriangle, Search } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import PortalModal from './PortalModal.svelte';
  import DropIndicator from '../layout/DropIndicator.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import Checkbox from '../components/Checkbox.svelte';

  let {
    isOpen = $bindable(false),
    requestTypeId = null,
    requestTypeName = '',
    isDarkMode = false,
    onsaved = undefined,
    onclose = undefined
  } = $props();

  // Field data
  let fields = $state([]);
  let availableFields = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let saving = $state(false);

  // Step management - steps are explicitly tracked, not derived from fields
  let steps = $state([1]);
  let currentStep = $state(1);

  // Search/filter
  let fieldSearchQuery = $state('');

  // Drag state
  let draggedField = $state(false);
  let fieldDragState = $state(new Map());
  let setupCleanups = $state([]);
  let setupTimeout;

  // Virtual field creation
  let addingVirtualField = $state(false);
  let virtualFieldName = $state('');
  let virtualFieldType = $state('text');
  let virtualFieldRequired = $state(false);
  let virtualFieldOptions = $state([{ value: '', label: '' }]);

  // Field editing
  let editingField = $state(null);
  let editDisplayName = $state('');
  let editDescription = $state('');

  // Helper function to capitalize field labels
  function capitalizeLabel(name) {
    if (!name) return '';
    return name.charAt(0).toUpperCase() + name.slice(1);
  }

  // Track the previous open state to only load when actually opening
  let wasOpen = $state(false);

  // Computed: fields for current step
  let currentStepFields = $derived(
    fields
      .filter(f => (f.step_number || 1) === currentStep)
      .sort((a, b) => a.display_order - b.display_order)
  );

  // Computed: filtered available fields (exclude already configured and apply search)
  let filteredAvailableFields = $derived(
    availableFields
      .filter(f => !fields.some(cf => cf.field_identifier === f.identifier))
      .filter(f => {
        if (!fieldSearchQuery.trim()) return true;
        const query = fieldSearchQuery.toLowerCase();
        return f.name.toLowerCase().includes(query) || f.identifier.toLowerCase().includes(query);
      })
  );

  // Load fields when modal opens
  $effect(() => {
    if (isOpen && !wasOpen && requestTypeId) {
      wasOpen = true;
      loadFields();
    } else if (!isOpen && wasOpen) {
      wasOpen = false;
      clearForm();
    }
  });

  // Re-setup drag and drop when fields or step changes
  $effect(() => {
    if (!loading && fields && typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => setupDragAndDrop(), 50);
    }
  });

  async function loadFields() {
    try {
      loading = true;
      error = null;
      fields = await api.requestTypes.getFields(requestTypeId);

      // Initialize steps from loaded fields
      const loadedSteps = [...new Set(fields.map(f => f.step_number || 1))].sort((a, b) => a - b);
      steps = loadedSteps.length > 0 ? loadedSteps : [1];
      currentStep = 1;

      // Load available fields for this request type's item type
      await loadAvailableFields();
    } catch (err) {
      console.error('Failed to load request type fields:', err);
      error = err.message || t('requestTypeFields.failedToLoadFields');
    } finally {
      loading = false;
    }
  }

  async function loadAvailableFields() {
    try {
      availableFields = await api.requestTypes.getAvailableFields(requestTypeId);
    } catch (err) {
      console.error('Failed to load available fields:', err);
      // Fall back to default fields
      availableFields = [
        { identifier: 'title', name: 'Title', type: 'default' },
        { identifier: 'description', name: 'Description', type: 'default' }
      ];
    }
  }

  function clearForm() {
    addingVirtualField = false;
    virtualFieldName = '';
    virtualFieldType = 'text';
    virtualFieldRequired = false;
    virtualFieldOptions = [{ value: '', label: '' }];
    editingField = null;
    error = null;
    fieldSearchQuery = '';
    cleanupDragAndDrop();
  }

  // === Drag and Drop Setup ===

  function cleanupDragAndDrop() {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupCleanups.forEach(fn => fn());
    setupCleanups = [];
    fieldDragState = new Map();
    draggedField = false;
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

    // Setup configured fields as both draggable and drop targets with edge detection
    document.querySelectorAll('[data-configured-field]').forEach(element => {
      const fieldIndex = parseInt(element.dataset.fieldIndex);
      const fieldId = element.dataset.fieldId;

      fieldDragState.set(fieldId, { closestEdge: null });

      // Make draggable
      const dragHandle = element.querySelector('.cursor-grab');
      const draggableCleanup = draggable({
        element,
        dragHandle: dragHandle || element,
        getInitialData: () => ({ fieldIndex, fieldId, type: 'configured-field' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => {
          element.style.opacity = '';
          clearDragState();
        }
      });

      // Make drop target with edge detection
      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'configured-field' && data.fieldIndex === fieldIndex) return false;
          return data.type === 'available-field' || data.type === 'configured-field';
        },
        getData: ({ input, element }) => {
          return attachClosestEdge({}, { input, element, allowedEdges: ['top', 'bottom'] });
        },
        onDragEnter: ({ self }) => {
          const closestEdge = extractClosestEdge(self.data);
          setDragState(fieldId, { closestEdge });
        },
        onDragLeave: () => {
          setDragState(fieldId, { closestEdge: null });
        },
        onDrop: ({ self, source }) => {
          const closestEdge = extractClosestEdge(self.data);
          const data = source.data;

          if (data.type === 'available-field') {
            addFieldAtPosition(data.field, fieldIndex, closestEdge);
          } else if (data.type === 'configured-field') {
            reorderFieldWithEdge(data.fieldIndex, fieldIndex, closestEdge);
          }

          setDragState(fieldId, { closestEdge: null });
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
        onDragLeave: () => { draggedField = false; },
        onDrop: ({ source }) => {
          if (source.data.type === 'available-field') {
            addFieldToStep(source.data.field);
          }
          draggedField = false;
        }
      });
      setupCleanups.push(cleanup);
    }
  }

  function setDragState(fieldId, state) {
    fieldDragState.set(fieldId, state);
    fieldDragState = new Map(fieldDragState);
  }

  function clearDragState() {
    fieldDragState.forEach((_, id) => {
      fieldDragState.set(id, { closestEdge: null });
    });
    fieldDragState = new Map(fieldDragState);
  }

  // === Field Management ===

  function addFieldToStep(fieldData) {
    // Check if field already exists in any step
    if (fields.some(f => f.field_identifier === fieldData.identifier)) {
      return;
    }

    const newField = {
      field_identifier: fieldData.identifier,
      field_type: fieldData.type,
      is_required: false,
      display_order: currentStepFields.length,
      field_name: fieldData.name,
      step_number: currentStep
    };

    fields = [...fields, newField];
    saveFields();
  }

  function addFieldAtPosition(fieldData, targetIndex, closestEdge) {
    // Check if field already exists
    if (fields.some(f => f.field_identifier === fieldData.identifier)) {
      return;
    }

    // Find the actual field at targetIndex in current step
    const targetField = currentStepFields[targetIndex];
    if (!targetField) {
      addFieldToStep(fieldData);
      return;
    }

    const insertOrder = closestEdge === 'bottom' ? targetField.display_order + 1 : targetField.display_order;

    const newField = {
      field_identifier: fieldData.identifier,
      field_type: fieldData.type,
      is_required: false,
      display_order: insertOrder,
      field_name: fieldData.name,
      step_number: currentStep
    };

    // Increment display_order for fields at or after insert position
    fields = fields.map(f => {
      if ((f.step_number || 1) === currentStep && f.display_order >= insertOrder) {
        return { ...f, display_order: f.display_order + 1 };
      }
      return f;
    });

    fields = [...fields, newField];
    recalculateDisplayOrder();
    saveFields();
  }

  function reorderFieldWithEdge(fromIndex, toIndex, closestEdge) {
    if (fromIndex === toIndex) return;

    const sortedFields = currentStepFields;
    const movedField = sortedFields[fromIndex];
    const targetField = sortedFields[toIndex];

    if (!movedField || !targetField) return;

    // Calculate new display order
    let newOrder;
    if (closestEdge === 'bottom') {
      newOrder = targetField.display_order + 0.5;
    } else {
      newOrder = targetField.display_order - 0.5;
    }

    // Update the moved field's order
    movedField.display_order = newOrder;

    // Re-sort and reassign sequential orders
    recalculateDisplayOrder();
    saveFields();
  }

  function recalculateDisplayOrder() {
    // Group by step and recalculate display_order within each step
    const byStep = {};
    fields.forEach(f => {
      const step = f.step_number || 1;
      if (!byStep[step]) byStep[step] = [];
      byStep[step].push(f);
    });

    Object.values(byStep).forEach(stepFields => {
      stepFields.sort((a, b) => a.display_order - b.display_order);
      stepFields.forEach((f, i) => f.display_order = i);
    });

    fields = [...fields];
  }

  function removeField(field) {
    fields = fields.filter(f => f !== field);
    recalculateDisplayOrder();
    saveFields();
  }

  function toggleRequired(field) {
    field.is_required = !field.is_required;
    fields = [...fields];
    saveFields();
  }

  // === Virtual Field Management ===

  function startAddingVirtualField() {
    addingVirtualField = true;
    virtualFieldName = '';
    virtualFieldType = 'text';
    virtualFieldRequired = false;
    virtualFieldOptions = [{ value: '', label: '' }];
  }

  function cancelAddingVirtualField() {
    addingVirtualField = false;
    virtualFieldName = '';
    virtualFieldType = 'text';
    virtualFieldRequired = false;
    virtualFieldOptions = [{ value: '', label: '' }];
  }

  function addVirtualField() {
    if (!virtualFieldName.trim()) {
      error = t('requestTypeFields.pleaseEnterFieldName');
      return;
    }

    const fieldIdentifier = `vf_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;

    // Prepare options for select type
    let optionsJson = null;
    if (virtualFieldType === 'select') {
      const validOptions = virtualFieldOptions.filter(o => o.value.trim() && o.label.trim());
      if (validOptions.length === 0) {
        error = t('requestTypeFields.addAtLeastOneOption');
        return;
      }
      optionsJson = JSON.stringify(validOptions);
    }

    fields = [...fields, {
      field_identifier: fieldIdentifier,
      field_type: 'virtual',
      is_required: virtualFieldRequired,
      display_order: currentStepFields.length,
      field_name: virtualFieldName.trim(),
      display_name: virtualFieldName.trim(),
      step_number: currentStep,
      virtual_field_type: virtualFieldType,
      virtual_field_options: optionsJson
    }];

    cancelAddingVirtualField();
    saveFields();
  }

  function addVirtualFieldOption() {
    virtualFieldOptions = [...virtualFieldOptions, { value: '', label: '' }];
  }

  function removeVirtualFieldOption(index) {
    virtualFieldOptions = virtualFieldOptions.filter((_, i) => i !== index);
  }

  // === Field Editing ===

  function startEditingField(field) {
    editingField = field;
    editDisplayName = field.display_name || '';
    editDescription = field.description || '';
  }

  function saveFieldEdit() {
    if (editingField) {
      editingField.display_name = editDisplayName.trim() || null;
      editingField.description = editDescription.trim() || null;
      fields = [...fields];
      editingField = null;
      saveFields();
    }
  }

  function cancelFieldEdit() {
    editingField = null;
    editDisplayName = '';
    editDescription = '';
  }

  // === Step Management ===

  function addStep() {
    const maxStep = Math.max(...steps, 0);
    steps = [...steps, maxStep + 1];
    currentStep = maxStep + 1;
    // Save fields to persist the new step structure
    saveFields();
  }

  function removeStep(stepNumber) {
    if (steps.length <= 1) return;

    // Remove fields from this step
    fields = fields.filter(f => (f.step_number || 1) !== stepNumber);

    // Renumber remaining steps
    const stepsToKeep = steps.filter(s => s !== stepNumber).sort((a, b) => a - b);
    const renumberMap = {};
    stepsToKeep.forEach((s, i) => renumberMap[s] = i + 1);

    fields = fields.map(f => ({
      ...f,
      step_number: renumberMap[f.step_number || 1] || 1
    }));

    // Update steps list
    steps = stepsToKeep.length > 0 ? stepsToKeep.map((_, i) => i + 1) : [1];
    currentStep = Math.min(currentStep, Math.max(...steps));

    saveFields();
  }

  function stepHasFields(step) {
    return fields.some(f => (f.step_number || 1) === step);
  }

  // === Save ===

  async function saveFields() {
    try {
      saving = true;
      error = null;

      // Ensure empty steps are preserved by including step metadata
      // The backend will handle this based on field data
      const fieldsToSave = fields.map(f => ({
        field_identifier: f.field_identifier,
        field_type: f.field_type,
        display_order: f.display_order,
        is_required: f.is_required,
        display_name: f.display_name || null,
        description: f.description || null,
        step_number: f.step_number || 1,
        virtual_field_type: f.virtual_field_type || null,
        virtual_field_options: f.virtual_field_options || null
      }));

      await api.requestTypes.updateFields(requestTypeId, fieldsToSave);
      onsaved?.();
    } catch (err) {
      console.error('Failed to save fields:', err);
      error = err.message || t('requestTypeFields.failedToSaveFields');
    } finally {
      saving = false;
    }
  }

  function handleClose() {
    isOpen = false;
    onclose?.();
  }

  function getFieldTypeLabel(field) {
    if (field.field_type === 'virtual') {
      const typeLabels = {
        text: t('requestTypeFields.text'),
        textarea: t('requestTypeFields.multiLine'),
        select: t('requestTypeFields.select'),
        checkbox: t('requestTypeFields.checkbox')
      };
      return `${t('requestTypeFields.virtualField')} - ${typeLabels[field.virtual_field_type] || field.virtual_field_type}`;
    }
    return field.field_type === 'default' ? t('requestTypeFields.defaultField') : t('requestTypeFields.customField');
  }

  function getAvailableFieldTypeLabel(field) {
    if (field.type === 'default') {
      return t('requestTypeFields.system');
    }
    if (field.field_type) {
      return field.field_type;
    }
    return t('requestTypeFields.custom');
  }
</script>

{#if isOpen}
  <PortalModal
    isOpen={isOpen}
    isDarkMode={isDarkMode}
    maxWidth="max-w-5xl"
    title={`${t('requestTypeFields.configureFields')}: ${requestTypeName}`}
    onClose={handleClose}
    bodyClass="px-6 py-4"
  >
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else}
      {#if error}
        <div
          class="mb-4 p-3 rounded border"
          style="background-color: var(--ds-status-error-subtle); border-color: var(--ds-status-error);"
        >
          <p class="text-sm" style="color: var(--ds-status-error);">
            {error}
          </p>
        </div>
      {/if}

      <!-- Step Tabs -->
      <div class="flex items-center gap-2 mb-4 pb-3 border-b" style="border-color: var(--ds-border-subtle);">
        {#each steps as step}
          {@const hasFields = stepHasFields(step)}
          <button
            onclick={() => currentStep = step}
            class="px-4 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-1.5"
            style="
              background-color: {currentStep === step
                ? 'var(--ds-interactive)'
                : hasFields
                  ? 'var(--ds-surface-raised)'
                  : 'var(--ds-status-warning-subtle)'};
              color: {currentStep === step ? 'white' : 'var(--ds-text-subtle)'};
              border: {!hasFields && currentStep !== step ? '1px solid var(--ds-status-warning)' : '1px solid transparent'};
            "
          >
            {t('requestTypeFields.step')} {step}
            {#if !hasFields && currentStep !== step}
              <AlertTriangle class="w-3 h-3" style="color: var(--ds-status-warning);" />
            {/if}
          </button>
        {/each}
        <button
          onclick={addStep}
          class="px-3 py-2 rounded-lg text-sm transition-all flex items-center gap-1"
          style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
          title={t('requestTypeFields.addNewStep')}
        >
          <Plus class="w-4 h-4" />
        </button>
        {#if steps.length > 1}
          <button
            onclick={() => removeStep(currentStep)}
            class="px-3 py-2 rounded-lg text-sm transition-all"
            style="color: var(--ds-status-error);"
            title={t('requestTypeFields.removeCurrentStep')}
          >
            <Trash2 class="w-4 h-4" />
          </button>
        {/if}
      </div>

      <!-- Empty Step Warning -->
      {#if !stepHasFields(currentStep) && !addingVirtualField}
        <div
          class="mb-4 p-3 rounded border flex items-center gap-2"
          style="background-color: var(--ds-status-warning-subtle); border-color: var(--ds-status-warning);"
        >
          <AlertTriangle class="w-4 h-4 flex-shrink-0" style="color: var(--ds-status-warning);" />
          <p class="text-sm" style="color: var(--ds-text);">
            {t('requestTypeFields.stepHasNoFields')}
          </p>
        </div>
      {/if}

      <!-- Dual Panel Layout -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Available Fields Panel -->
        <div class="rounded-xl p-4 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h3 class="text-base font-semibold mb-3" style="color: var(--ds-text);">
            {t('requestTypeFields.availableFields')}
          </h3>
          <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">
            {t('requestTypeFields.dragFieldsHint')}
          </p>

          <!-- Search -->
          <div class="relative mb-3">
            <Search class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2" style="color: var(--ds-text-subtle);" />
            <input
              type="text"
              bind:value={fieldSearchQuery}
              placeholder={t('requestTypeFields.searchFields')}
              class="w-full pl-9 pr-3 py-2 rounded border text-sm focus:outline-none focus:ring-2"
              style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
            />
          </div>

          <div class="space-y-1 max-h-80 overflow-y-auto">
            {#each filteredAvailableFields as field (field.identifier)}
              <div
                data-available-field={JSON.stringify(field)}
                class="group flex items-center gap-3 px-3 py-2.5 rounded border transition-all duration-200 cursor-grab hover:border-blue-300 active:cursor-grabbing"
                style="border-color: var(--ds-border); background-color: var(--ds-background-input); user-select: none;"
              >
                <!-- Drag Handle -->
                <div class="flex-shrink-0">
                  <svg class="w-4 h-4" style="color: var(--ds-text-subtle);" fill="currentColor" viewBox="0 0 24 24">
                    <circle cx="9" cy="6" r="1.5"/>
                    <circle cx="15" cy="6" r="1.5"/>
                    <circle cx="9" cy="12" r="1.5"/>
                    <circle cx="15" cy="12" r="1.5"/>
                    <circle cx="9" cy="18" r="1.5"/>
                    <circle cx="15" cy="18" r="1.5"/>
                  </svg>
                </div>

                <div class="flex-1 min-w-0">
                  <div class="font-medium text-sm" style="color: var(--ds-text);">
                    {field.name}
                  </div>
                </div>

                <span
                  class="text-xs px-1.5 py-0.5 rounded flex-shrink-0"
                  style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);"
                >
                  {getAvailableFieldTypeLabel(field)}
                </span>
              </div>
            {:else}
              <div class="text-center py-6">
                <p class="text-sm" style="color: var(--ds-text-subtle);">
                  {#if fieldSearchQuery.trim()}
                    {t('requestTypeFields.noFieldsMatch')}
                  {:else}
                    {t('requestTypeFields.allFieldsAdded')}
                  {/if}
                </p>
              </div>
            {/each}
          </div>
        </div>

        <!-- Configured Fields Panel -->
        <div class="rounded-xl p-4 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h3 class="text-base font-semibold mb-3" style="color: var(--ds-text);">
            {t('requestTypeFields.step')} {currentStep} {t('requestTypeFields.fields')} ({currentStepFields.length})
          </h3>
          <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">
            {t('requestTypeFields.dragToReorder')}
          </p>

          <div
            data-drop-zone
            class="min-h-64 max-h-80 overflow-y-auto border-2 border-dashed rounded p-3 space-y-2"
            class:border-blue-400={draggedField}
            style="border-color: {draggedField ? 'var(--ds-interactive)' : 'var(--ds-border)'}; background-color: {draggedField ? 'var(--ds-interactive-subtle)' : 'transparent'};"
          >
            {#if currentStepFields.length === 0 && !addingVirtualField}
              <div class="text-center py-8">
                <svg class="w-10 h-10 mx-auto mb-3" style="color: var(--ds-text-subtle);" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"/>
                </svg>
                <p class="text-sm" style="color: var(--ds-text-subtle);">
                  {t('requestTypeFields.dropFieldsHere')}
                </p>
              </div>
            {:else}
              {#each currentStepFields as field, index (field.field_identifier)}
                <div
                  data-configured-field
                  data-field-index={index}
                  data-field-id={field.field_identifier}
                  class="relative group flex items-center gap-3 px-3 py-2.5 rounded border transition-all duration-200"
                  style="border-color: var(--ds-border); background-color: var(--ds-background); user-select: none;"
                >
                  <!-- Drop indicator -->
                  {#if fieldDragState.get(field.field_identifier)?.closestEdge}
                    <DropIndicator edge={fieldDragState.get(field.field_identifier)?.closestEdge} gap={8} />
                  {/if}

                  <!-- Drag Handle -->
                  <div
                    class="cursor-grab active:cursor-grabbing flex-shrink-0 p-1 rounded transition-colors"
                    style="touch-action: none;"
                  >
                    <svg class="w-4 h-4" style="color: var(--ds-text-subtle);" fill="currentColor" viewBox="0 0 24 24">
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
                      {capitalizeLabel(field.display_name || field.field_name || field.field_identifier)}
                      <span
                        class="text-xs px-1.5 py-0.5 rounded"
                        style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);"
                      >
                        {field.field_type === 'virtual' ? t('requestTypeFields.virtual') : field.field_type === 'default' ? t('requestTypeFields.system') : t('requestTypeFields.custom')}
                      </span>
                    </div>
                    {#if field.display_name && field.display_name !== field.field_name && field.field_type !== 'virtual'}
                      <div class="text-xs" style="color: var(--ds-text-subtle);">
                        {field.field_name || field.field_identifier}
                      </div>
                    {/if}
                  </div>

                  <div class="flex items-center gap-2 flex-shrink-0">
                    <!-- Required Checkbox -->
                    <Checkbox
                      checked={field.is_required}
                      onchange={() => toggleRequired(field)}
                      label={t('requestTypeFields.required')}
                      size="small"
                    />

                    <!-- Edit Button -->
                    <button
                      onclick={() => startEditingField(field)}
                      class="p-1.5 rounded transition-all opacity-0 group-hover:opacity-100"
                      style="color: var(--ds-text-subtle);"
                      title={t('layout.editDisplaySettings')}
                    >
                      <Pencil class="w-3.5 h-3.5" />
                    </button>

                    <!-- Remove Button -->
                    <button
                      onclick={() => removeField(field)}
                      class="p-1.5 rounded transition-all opacity-0 group-hover:opacity-100"
                      style="color: var(--ds-status-error);"
                      title={t('requestTypeFields.removeField')}
                    >
                      <Trash2 class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              {/each}
            {/if}
          </div>
        </div>
      </div>

      <!-- Add Virtual Field Section -->
      {#if addingVirtualField}
        <div
          class="mt-4 p-4 rounded border space-y-3"
          style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
        >
          <div class="text-sm font-medium" style="color: var(--ds-text);">
            {t('requestTypeFields.addVirtualField')}
          </div>

          <div>
            <label for="virtual-field-name" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
              {t('requestTypeFields.fieldName')}
            </label>
            <input
              id="virtual-field-name"
              type="text"
              bind:value={virtualFieldName}
              placeholder={t('requestTypeFields.fieldNamePlaceholder')}
              class="w-full px-3 py-2 rounded border focus:outline-none focus:ring-2 text-sm"
              style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
            />
          </div>

          <div>
            <span id="virtual-field-type-label" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
              {t('requestTypeFields.fieldType')}
            </span>
            <div class="grid grid-cols-4 gap-2" role="group" aria-labelledby="virtual-field-type-label">
              {#each [
                { value: 'text', label: t('requestTypeFields.text'), icon: Type },
                { value: 'textarea', label: t('requestTypeFields.multiLine'), icon: AlignLeft },
                { value: 'select', label: t('requestTypeFields.select'), icon: ListChecks },
                { value: 'checkbox', label: t('requestTypeFields.checkbox'), icon: ToggleLeft }
              ] as type}
                <button
                  onclick={() => virtualFieldType = type.value}
                  class="flex flex-col items-center gap-1 p-3 rounded border transition-all"
                  style="background-color: {virtualFieldType === type.value ? 'var(--ds-interactive-subtle)' : 'transparent'}; border-color: {virtualFieldType === type.value ? 'var(--ds-interactive)' : 'var(--ds-border)'}; color: {virtualFieldType === type.value ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'};"
                >
                  <type.icon class="w-5 h-5" />
                  <span class="text-xs">{type.label}</span>
                </button>
              {/each}
            </div>
          </div>

          {#if virtualFieldType === 'select'}
            <div>
              <span id="virtual-field-options-label" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                {t('requestTypeFields.options')}
              </span>
              <div class="space-y-2" role="group" aria-labelledby="virtual-field-options-label">
                {#each virtualFieldOptions as option, i}
                  <div class="flex gap-2">
                    <input
                      type="text"
                      bind:value={option.value}
                      placeholder={t('requestTypeFields.value')}
                      class="flex-1 px-3 py-2 rounded border focus:outline-none focus:ring-2 text-sm"
                      style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
                    />
                    <input
                      type="text"
                      bind:value={option.label}
                      placeholder={t('requestTypeFields.label')}
                      class="flex-1 px-3 py-2 rounded border focus:outline-none focus:ring-2 text-sm"
                      style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
                    />
                    <button
                      onclick={() => removeVirtualFieldOption(i)}
                      class="p-2 rounded"
                      style="color: var(--ds-status-error);"
                      disabled={virtualFieldOptions.length === 1}
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>
                {/each}
                <button
                  onclick={addVirtualFieldOption}
                  class="text-sm flex items-center gap-1"
                  style="color: var(--ds-interactive);"
                >
                  <Plus class="w-4 h-4" /> {t('requestTypeFields.addOption')}
                </button>
              </div>
            </div>
          {/if}

          <Checkbox
            bind:checked={virtualFieldRequired}
            label={t('requestTypeFields.requiredField')}
            size="small"
          />

          <div class="flex gap-2">
            <Button
              onclick={addVirtualField}
              variant="primary"
              size="medium"
              class="flex-1"
            >
              {t('requestTypeFields.addVirtualField')}
            </Button>
            <Button
              onclick={cancelAddingVirtualField}
              variant="default"
              size="medium"
            >
              {t('common.cancel')}
            </Button>
          </div>
        </div>
      {:else}
        <!-- Add Virtual Field Button -->
        <div class="mt-4">
          <button
            onclick={startAddingVirtualField}
            class="flex items-center gap-2 px-4 py-2 rounded border-2 border-dashed transition-all text-sm"
            style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
          >
            <Type class="w-4 h-4" />
            <span class="font-medium">{t('requestTypeFields.addVirtualField')}</span>
          </button>
        </div>
      {/if}
    {/if}

    <!-- Footer -->
    <div
      class="px-6 py-4 border-t flex items-center justify-between -mx-6 -mb-4 mt-6"
      style="background-color: var(--ds-surface-sunken); border-color: var(--ds-border);"
    >
      <div class="text-sm" style="color: var(--ds-text-subtle);">
        {#if saving}
          <div class="flex items-center gap-2">
            <Spinner size="sm" />
            <span>{t('requestTypeFields.saving')}</span>
          </div>
        {:else}
          {t('requestTypeFields.changesSavedAuto')}
        {/if}
      </div>
      <Button
        onclick={handleClose}
        variant="primary"
        size="medium"
      >
        {t('common.done')}
      </Button>
    </div>
  </PortalModal>
{/if}

<!-- Field Edit Modal -->
{#if editingField}
  <PortalModal
    isOpen={true}
    isDarkMode={isDarkMode}
    maxWidth="max-w-md"
    title={t('requestTypeFields.editFieldDisplay')}
    onClose={cancelFieldEdit}
    bodyClass="px-6 py-4"
  >
    <div class="space-y-4">
      <div>
        <label for="edit-field-display-name" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
          {t('requestTypeFields.displayName')}
        </label>
        <input
          id="edit-field-display-name"
          type="text"
          bind:value={editDisplayName}
          placeholder={editingField.field_name || editingField.field_identifier}
          class="w-full px-3 py-2 rounded border focus:outline-none focus:ring-2"
          style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
        />
        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
          {t('requestTypeFields.overrideLabel')}
        </p>
      </div>

      <div>
        <label for="edit-field-description" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
          {t('requestTypeFields.descriptionHelpText')}
        </label>
        <textarea
          id="edit-field-description"
          bind:value={editDescription}
          placeholder={t('requestTypeFields.helpTextPlaceholder')}
          rows="3"
          class="w-full px-3 py-2 rounded border focus:outline-none focus:ring-2"
          style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
        ></textarea>
      </div>

      <div class="flex gap-2 pt-2">
        <Button
          onclick={saveFieldEdit}
          variant="primary"
          size="medium"
          class="flex-1"
        >
          {t('common.save')}
        </Button>
        <Button
          onclick={cancelFieldEdit}
          variant="default"
          size="medium"
        >
          {t('common.cancel')}
        </Button>
      </div>
    </div>
  </PortalModal>
{/if}
