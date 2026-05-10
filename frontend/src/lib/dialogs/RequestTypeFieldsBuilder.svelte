<script>
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import { createPopover, melt } from '@melt-ui/svelte';
  import { fly } from 'svelte/transition';
  import { tick } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Trash2, Pencil, Type, AlignLeft, ListChecks, ToggleLeft, AlertTriangle, Search, X } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import PortalModal from './PortalModal.svelte';
  import DropIndicator from '../layout/DropIndicator.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import Checkbox from '../components/Checkbox.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';

  let {
    requestTypeId = null,
    requestTypeName = '',
    resourceId = null,
    resourceName = '',
    channelId = null,
    apiHandlers = null,
    isDarkMode = false,
    onsaved = undefined,
    onclose = undefined
  } = $props();

  // Title template lives on the request_type row (not on its fields), but
  // surfacing it here is the only place it makes sense — admins reach for
  // it the moment they remove the title field. Asset reports go through
  // apiHandlers and don't have a title-template column, so we only load /
  // edit it when this builder is wired to a real request type.
  const isRequestTypeMode = $derived(!apiHandlers && requestTypeId !== null);
  let requestTypeRow = $state(null);
  let titleTemplate = $state('');
  let titleTemplateSaving = $state(false);
  let titleTemplateError = $state(null);
  let titleTemplateSaveTimer;

  // Resolve the effective resource id/name and API handlers. Defaults preserve
  // the legacy request-type behavior while allowing reuse for asset reports.
  // The default updateFields closes over channelId since the route requires it.
  const handlers = $derived(apiHandlers || {
    getFields: (id) => api.requestTypes.getFields(id),
    getAvailableFields: (id) => api.requestTypes.getAvailableFields(id),
    updateFields: (id, fields) => api.requestTypes.updateFields(channelId, id, fields)
  });
  const activeResourceId = $derived(resourceId ?? requestTypeId);
  const activeResourceName = $derived(resourceName || requestTypeName);

  // Field data
  let fields = $state([]);
  let availableFields = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let saving = $state(false);

  // Step management
  let steps = $state([1]);
  let currentStep = $state(1);

  // Drag state (for reordering configured fields only)
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

  // Add-field popover
  let addFieldQuery = $state('');
  let addFieldSearchEl = $state(null);
  const {
    elements: { trigger: addFieldTrigger, content: addFieldContent },
    states: { open: addFieldOpen }
  } = createPopover({
    forceVisible: true,
    positioning: { placement: 'bottom-start', gutter: 6 },
    portal: 'body'
  });

  $effect(() => {
    if ($addFieldOpen) {
      tick().then(() => addFieldSearchEl?.focus({ preventScroll: true }));
    } else {
      addFieldQuery = '';
    }
  });

  function capitalizeLabel(name) {
    if (!name) return '';
    return name.charAt(0).toUpperCase() + name.slice(1);
  }

  // Computed: fields for current step
  let currentStepFields = $derived(
    fields
      .filter(f => (f.step_number || 1) === currentStep)
      .sort((a, b) => a.display_order - b.display_order)
  );

  // True when the title field is anywhere in the form. When false, the
  // server falls back to the title_template — and the inline warning below
  // surfaces that requirement.
  let titleFieldInForm = $derived(
    fields.some(f => f.field_identifier === 'title')
  );

  // Computed: filtered available fields (exclude already configured, optional search filter)
  let filteredAvailableFields = $derived(
    availableFields
      .filter(f => !fields.some(cf => cf.field_identifier === f.identifier))
      .filter(f => {
        const q = addFieldQuery.trim().toLowerCase();
        if (!q) return true;
        return f.name.toLowerCase().includes(q) || f.identifier.toLowerCase().includes(q);
      })
  );

  // Mount: load on first render
  $effect(() => {
    if (activeResourceId) {
      loadFields();
    }
    return () => {
      if (titleTemplateSaveTimer) clearTimeout(titleTemplateSaveTimer);
      cleanupDragAndDrop();
    };
  });

  // Re-setup drag-and-drop (configured fields reorder) when fields/step change
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
      fields = await handlers.getFields(activeResourceId);

      // Title is enforced as required server-side; keep the visible state
      // honest for legacy rows that were saved with is_required=false.
      fields.forEach(f => {
        if (f.field_identifier === 'title' && !f.is_required) {
          f.is_required = true;
        }
      });

      const loadedSteps = [...new Set(fields.map(f => f.step_number || 1))].sort((a, b) => a - b);
      steps = loadedSteps.length > 0 ? loadedSteps : [1];
      currentStep = 1;

      await loadAvailableFields();

      if (isRequestTypeMode) {
        await loadRequestType();
      }
    } catch (err) {
      console.error('Failed to load request type fields:', err);
      error = err.message || t('requestTypeFields.failedToLoadFields');
    } finally {
      loading = false;
    }
  }

  async function loadRequestType() {
    try {
      requestTypeRow = await api.requestTypes.get(requestTypeId);
      titleTemplate = requestTypeRow?.title_template || '';
    } catch (err) {
      console.error('Failed to load request type:', err);
    }
  }

  function scheduleTitleTemplateSave() {
    if (titleTemplateSaveTimer) clearTimeout(titleTemplateSaveTimer);
    titleTemplateSaveTimer = setTimeout(saveTitleTemplate, 600);
  }

  async function saveTitleTemplate() {
    if (!isRequestTypeMode || !requestTypeRow) return;
    try {
      titleTemplateSaving = true;
      titleTemplateError = null;
      // PUT /channels/.../request-types/:id requires the full editable body.
      await api.requestTypes.update(channelId, requestTypeId, {
        name: requestTypeRow.name,
        description: requestTypeRow.description || '',
        icon: requestTypeRow.icon,
        color: requestTypeRow.color,
        item_type_id: requestTypeRow.item_type_id,
        workspace_id: requestTypeRow.workspace_id || null,
        title_template: titleTemplate.trim(),
        is_active: requestTypeRow.is_active !== false
      });
      requestTypeRow.title_template = titleTemplate.trim();
      onsaved?.();
    } catch (err) {
      console.error('Failed to save title template:', err);
      titleTemplateError = err.message || 'Failed to save title template';
    } finally {
      titleTemplateSaving = false;
    }
  }

  async function loadAvailableFields() {
    try {
      availableFields = await handlers.getAvailableFields(activeResourceId);
    } catch (err) {
      console.error('Failed to load available fields:', err);
      availableFields = [
        { identifier: 'title', name: 'Title', type: 'default' },
        { identifier: 'description', name: 'Description', type: 'default' }
      ];
    }
  }

  // === Drag and Drop (configured fields reorder only) ===

  function cleanupDragAndDrop() {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupCleanups.forEach(fn => fn());
    setupCleanups = [];
    fieldDragState = new Map();
  }

  function setupDragAndDrop() {
    cleanupDragAndDrop();

    document.querySelectorAll('[data-configured-field]').forEach((element) => {
      const fieldIndex = parseInt(element.dataset.fieldIndex);
      const fieldId = element.dataset.fieldId;

      fieldDragState.set(fieldId, { closestEdge: null });

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

      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'configured-field' && data.fieldIndex === fieldIndex) return false;
          return data.type === 'configured-field';
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
          if (source.data.type === 'configured-field') {
            reorderFieldWithEdge(source.data.fieldIndex, fieldIndex, closestEdge);
          }
          setDragState(fieldId, { closestEdge: null });
        }
      });

      setupCleanups.push(() => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });
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
    if (fields.some(f => f.field_identifier === fieldData.identifier)) {
      return;
    }
    const newField = {
      field_identifier: fieldData.identifier,
      field_type: fieldData.type,
      // Title is enforced as required server-side whenever the field is on
      // the form (items.title is NOT NULL). Mirror that here so the FE
      // toggle reflects reality.
      is_required: fieldData.identifier === 'title' ? true : false,
      display_order: currentStepFields.length,
      field_name: fieldData.name,
      step_number: currentStep
    };
    fields = [...fields, newField];
    saveFields();
  }

  function pickField(field) {
    addFieldToStep(field);
    $addFieldOpen = false;
  }

  function reorderFieldWithEdge(fromIndex, toIndex, closestEdge) {
    if (fromIndex === toIndex) return;
    const sortedFields = currentStepFields;
    const movedField = sortedFields[fromIndex];
    const targetField = sortedFields[toIndex];
    if (!movedField || !targetField) return;

    let newOrder;
    if (closestEdge === 'bottom') {
      newOrder = targetField.display_order + 0.5;
    } else {
      newOrder = targetField.display_order - 0.5;
    }
    movedField.display_order = newOrder;
    recalculateDisplayOrder();
    saveFields();
  }

  function recalculateDisplayOrder() {
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
    if (field.field_identifier === 'title') {
      field.is_required = true;
      fields = [...fields];
      return;
    }
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
    saveFields();
  }

  function removeStep(stepNumber) {
    if (steps.length <= 1) return;
    fields = fields.filter(f => (f.step_number || 1) !== stepNumber);
    const stepsToKeep = steps.filter(s => s !== stepNumber).sort((a, b) => a - b);
    const renumberMap = {};
    stepsToKeep.forEach((s, i) => renumberMap[s] = i + 1);
    fields = fields.map(f => ({
      ...f,
      step_number: renumberMap[f.step_number || 1] || 1
    }));
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
      const fieldsToSave = fields.map(f => ({
        field_identifier: f.field_identifier,
        field_type: f.field_type,
        display_order: f.display_order,
        is_required: f.field_identifier === 'title' ? true : f.is_required,
        display_name: f.display_name || null,
        description: f.description || null,
        step_number: f.step_number || 1,
        virtual_field_type: f.virtual_field_type || null,
        virtual_field_options: f.virtual_field_options || null
      }));
      await handlers.updateFields(activeResourceId, fieldsToSave);
      onsaved?.();
    } catch (err) {
      console.error('Failed to save fields:', err);
      error = err.message || t('requestTypeFields.failedToSaveFields');
    } finally {
      saving = false;
    }
  }

  function handleClose() {
    onclose?.();
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

<div class="flex flex-col h-full">
  <!-- Header — matches the customize panel's header sizing (px-6 py-4 + text-lg) so they line up across the seam. -->
  <div
    class="flex items-center justify-between px-6 py-4 border-b flex-shrink-0"
    style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
  >
    <h2 class="text-lg font-semibold truncate" style="color: var(--ds-text);">
      {activeResourceName}
    </h2>
    <button
      onclick={handleClose}
      class="p-1.5 rounded transition-all"
      style="color: var(--ds-text-subtle);"
      aria-label={t('common.close')}
    >
      <X class="w-5 h-5" />
    </button>
  </div>

  <!-- Body -->
  <div class="flex-1 overflow-y-auto px-4 py-4">
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
          <p class="text-sm" style="color: var(--ds-status-error);">{error}</p>
        </div>
      {/if}

      <!-- Step Tabs -->
      <div class="flex items-center gap-2 mb-4 pb-3 border-b flex-wrap" style="border-color: var(--ds-border-subtle);">
        {#each steps as step}
          {@const hasFields = stepHasFields(step)}
          <button
            onclick={() => currentStep = step}
            class="px-3 py-1.5 rounded-lg text-sm font-medium transition-all flex items-center gap-1.5"
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
          class="px-2 py-1.5 rounded-lg text-sm transition-all flex items-center gap-1"
          style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
          title={t('requestTypeFields.addNewStep')}
        >
          <Plus class="w-4 h-4" />
        </button>
        {#if steps.length > 1}
          <button
            onclick={() => removeStep(currentStep)}
            class="px-2 py-1.5 rounded-lg text-sm transition-all"
            style="color: var(--ds-status-error);"
            title={t('requestTypeFields.removeCurrentStep')}
          >
            <Trash2 class="w-4 h-4" />
          </button>
        {/if}
      </div>

      <!-- Title Template (request-type mode, only when title is removed) -->
      {#if isRequestTypeMode && !titleFieldInForm}
        {@const templateMissing = !titleTemplate.trim()}
        <div
          class="mb-4 p-3 rounded border"
          style="
            background-color: {templateMissing ? 'var(--ds-status-warning-subtle)' : 'var(--ds-surface-raised)'};
            border-color: {templateMissing ? 'var(--ds-status-warning)' : 'var(--ds-border)'};
          "
        >
          <div class="flex items-start gap-2 mb-2">
            {#if templateMissing}
              <AlertTriangle class="w-4 h-4 flex-shrink-0 mt-0.5" style="color: var(--ds-status-warning);" />
            {/if}
            <div class="flex-1">
              <label for="rtfb-title-template" class="block text-sm font-medium" style="color: var(--ds-text);">
                Title template (required — title field is hidden)
              </label>
              <p class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">
                Supports <code>{'{{type.name}}'}</code>, <code>{'{{requester.name}}'}</code>, <code>{'{{description}}'}</code>, and <code>{'{{custom.<field_name>}}'}</code>.
              </p>
            </div>
          </div>
          <input
            id="rtfb-title-template"
            type="text"
            bind:value={titleTemplate}
            oninput={scheduleTitleTemplateSave}
            onblur={saveTitleTemplate}
            placeholder={'Request: {{type.name}} from {{requester.name}}'}
            class="w-full px-3 py-2 rounded border text-sm focus:outline-none focus:ring-2"
            style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
          />
          {#if titleTemplateError}
            <p class="text-xs mt-1" style="color: var(--ds-status-error);">{titleTemplateError}</p>
          {:else if titleTemplateSaving}
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">Saving…</p>
          {/if}
        </div>
      {/if}

      <!-- Empty Step Warning -->
      {#if !stepHasFields(currentStep) && !addingVirtualField}
        <div
          class="mb-3 p-3 rounded border flex items-center gap-2"
          style="background-color: var(--ds-status-warning-subtle); border-color: var(--ds-status-warning);"
        >
          <AlertTriangle class="w-4 h-4 flex-shrink-0" style="color: var(--ds-status-warning);" />
          <p class="text-sm" style="color: var(--ds-text);">
            {t('requestTypeFields.stepHasNoFields')}
          </p>
        </div>
      {/if}

      <!-- Add-field action row -->
      <div class="flex items-center gap-2 mb-3">
        <button
          use:melt={$addFieldTrigger}
          class="flex items-center gap-1.5 px-3 py-1.5 rounded border text-sm transition-all"
          style="background-color: var(--ds-interactive); color: white; border-color: var(--ds-interactive);"
        >
          <Plus class="w-4 h-4" /> {t('requestTypeFields.addField', 'Add field')}
        </button>
        {#if !addingVirtualField}
          <button
            onclick={startAddingVirtualField}
            class="flex items-center gap-1.5 px-3 py-1.5 rounded border-2 border-dashed text-sm transition-all"
            style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
          >
            <Type class="w-4 h-4" />
            {t('requestTypeFields.addVirtualField')}
          </button>
        {/if}
      </div>

      <!-- Configured Fields list (full width) -->
      <div class="space-y-2">
        {#each currentStepFields as field, index (field.field_identifier)}
          <div
            data-configured-field
            data-field-index={index}
            data-field-id={field.field_identifier}
            class="relative group flex items-center gap-2 px-3 py-2 rounded border transition-all"
            style="border-color: var(--ds-border); background-color: var(--ds-background); user-select: none;"
          >
            {#if fieldDragState.get(field.field_identifier)?.closestEdge}
              <DropIndicator edge={fieldDragState.get(field.field_identifier)?.closestEdge} gap={8} />
            {/if}

            <div
              class="cursor-grab active:cursor-grabbing flex-shrink-0 p-1 rounded"
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
              <div class="font-medium text-sm flex items-center gap-2 truncate" style="color: var(--ds-text);">
                {capitalizeLabel(field.display_name || field.field_name || field.field_identifier)}
                <span
                  class="text-xs px-1.5 py-0.5 rounded flex-shrink-0"
                  style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);"
                >
                  {field.field_type === 'virtual' ? t('requestTypeFields.virtual') : field.field_type === 'default' ? t('requestTypeFields.system') : t('requestTypeFields.custom')}
                </span>
              </div>
              {#if field.display_name && field.display_name !== field.field_name && field.field_type !== 'virtual'}
                <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
                  {field.field_name || field.field_identifier}
                </div>
              {/if}
            </div>

            <div class="flex items-center gap-1 flex-shrink-0">
              <Checkbox
                checked={field.is_required}
                onchange={() => toggleRequired(field)}
                label={t('requestTypeFields.required')}
                size="small"
                disabled={field.field_identifier === 'title'}
              />
              <button
                onclick={() => startEditingField(field)}
                class="p-1.5 rounded transition-all opacity-0 group-hover:opacity-100"
                style="color: var(--ds-text-subtle);"
                title={t('layout.editDisplaySettings')}
              >
                <Pencil class="w-3.5 h-3.5" />
              </button>
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
      </div>

      <!-- Add Virtual Field config -->
      {#if addingVirtualField}
        <div
          class="mt-4 p-3 rounded border space-y-3"
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
            <div class="grid grid-cols-2 gap-2" role="group" aria-labelledby="virtual-field-type-label">
              {#each [
                { value: 'text', label: t('requestTypeFields.text'), icon: Type },
                { value: 'textarea', label: t('requestTypeFields.multiLine'), icon: AlignLeft },
                { value: 'select', label: t('requestTypeFields.select'), icon: ListChecks },
                { value: 'checkbox', label: t('requestTypeFields.checkbox'), icon: ToggleLeft }
              ] as type}
                <button
                  onclick={() => virtualFieldType = type.value}
                  class="flex flex-col items-center gap-1 p-2 rounded border transition-all"
                  style="background-color: {virtualFieldType === type.value ? 'var(--ds-interactive-subtle)' : 'transparent'}; border-color: {virtualFieldType === type.value ? 'var(--ds-interactive)' : 'var(--ds-border)'}; color: {virtualFieldType === type.value ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'};"
                >
                  <type.icon class="w-4 h-4" />
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
                      class="flex-1 px-2 py-1.5 rounded border focus:outline-none focus:ring-2 text-sm"
                      style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
                    />
                    <input
                      type="text"
                      bind:value={option.label}
                      placeholder={t('requestTypeFields.label')}
                      class="flex-1 px-2 py-1.5 rounded border focus:outline-none focus:ring-2 text-sm"
                      style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
                    />
                    <button
                      onclick={() => removeVirtualFieldOption(i)}
                      class="p-1.5 rounded"
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
            <Button onclick={addVirtualField} variant="primary" size="medium" class="flex-1">
              {t('requestTypeFields.addVirtualField')}
            </Button>
            <Button onclick={cancelAddingVirtualField} variant="default" size="medium">
              {t('common.cancel')}
            </Button>
          </div>
        </div>
      {/if}
    {/if}
  </div>

  <!-- Footer -->
  <div
    class="px-4 py-2 border-t flex items-center justify-between flex-shrink-0"
    style="background-color: var(--ds-surface-sunken); border-color: var(--ds-border);"
  >
    <div class="text-xs" style="color: var(--ds-text-subtle);">
      {#if saving}
        <div class="flex items-center gap-2">
          <Spinner size="sm" />
          <span>{t('requestTypeFields.saving')}</span>
        </div>
      {:else}
        {t('requestTypeFields.changesSavedAuto')}
      {/if}
    </div>
  </div>
</div>

<!-- Add-field popover (portaled to body) -->
{#if $addFieldOpen}
  <div
    use:melt={$addFieldContent}
    class="z-[70] w-72 rounded border shadow-lg flex flex-col"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
    transition:fly={{ duration: 150, y: -5 }}
  >
    <div class="p-2 border-b" style="border-color: var(--ds-border);">
      <div class="relative">
        <Search class="w-4 h-4 absolute left-2 top-1/2 -translate-y-1/2" style="color: var(--ds-text-subtle);" />
        <input
          bind:this={addFieldSearchEl}
          type="text"
          bind:value={addFieldQuery}
          placeholder={t('requestTypeFields.searchFields')}
          class="w-full pl-8 pr-2 py-1.5 rounded border text-sm focus:outline-none focus:ring-2"
          style="background-color: var(--ds-background-input); color: var(--ds-text); border-color: var(--ds-border); --tw-ring-color: var(--ds-interactive);"
        />
      </div>
    </div>
    <div class="max-h-72 overflow-y-auto py-1">
      {#each filteredAvailableFields as field (field.identifier)}
        <button
          onclick={() => pickField(field)}
          class="w-full flex items-center justify-between gap-2 px-3 py-2 text-left transition-colors"
          style="color: var(--ds-text);"
          onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
        >
          <span class="text-sm font-medium truncate">{field.name}</span>
          <span
            class="text-xs px-1.5 py-0.5 rounded flex-shrink-0"
            style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);"
          >
            {getAvailableFieldTypeLabel(field)}
          </span>
        </button>
      {:else}
        <div class="px-3 py-4 text-center text-sm" style="color: var(--ds-text-subtle);">
          {addFieldQuery.trim() ? t('requestTypeFields.noFieldsMatch') : t('requestTypeFields.allFieldsAdded')}
        </div>
      {/each}
    </div>
  </div>
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
        <DescriptionText>
          {t('requestTypeFields.overrideLabel')}
        </DescriptionText>
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
        <Button onclick={saveFieldEdit} variant="primary" size="medium" class="flex-1">
          {t('common.save')}
        </Button>
        <Button onclick={cancelFieldEdit} variant="default" size="medium">
          {t('common.cancel')}
        </Button>
      </div>
    </div>
  </PortalModal>
{/if}
