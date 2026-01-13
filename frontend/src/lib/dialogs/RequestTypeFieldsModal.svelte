<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Trash2, GripVertical, Check, ChevronUp, ChevronDown, Pencil, Type, AlignLeft, ListChecks, ToggleLeft } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Spinner from '../components/Spinner.svelte';
  import PortalModal from './PortalModal.svelte';

  const dispatch = createEventDispatcher();

  export let isOpen = false;
  export let requestTypeId = null;
  export let requestTypeName = '';
  export let isDarkMode = false;

  let fields = [];
  let loading = false;
  let error = null;
  let availableFields = [];
  let saving = false;

  // Step management
  let steps = [1];
  let currentStep = 1;

  // Field being added
  let addingField = false;
  let newFieldIdentifier = '';
  let newFieldType = 'default';
  let newIsRequired = false;

  // Virtual field creation
  let addingVirtualField = false;
  let virtualFieldName = '';
  let virtualFieldType = 'text';
  let virtualFieldRequired = false;
  let virtualFieldOptions = [{ value: '', label: '' }];

  // Field editing
  let editingField = null;
  let editDisplayName = '';
  let editDescription = '';

  // Track the previous open state to only load when actually opening
  let wasOpen = false;

  // Computed: fields for current step
  $: currentStepFields = fields.filter(f => (f.step_number || 1) === currentStep);

  // Update steps list when fields change
  $: {
    const stepNumbers = [...new Set(fields.map(f => f.step_number || 1))].sort((a, b) => a - b);
    if (stepNumbers.length === 0) {
      steps = [1];
    } else {
      steps = stepNumbers;
    }
  }

  // Load fields when modal opens
  $: {
    if (isOpen && !wasOpen && requestTypeId) {
      wasOpen = true;
      loadFields();
    } else if (!isOpen && wasOpen) {
      wasOpen = false;
      clearForm();
    }
  }

  async function loadFields() {
    try {
      loading = true;
      error = null;
      fields = await api.requestTypes.getFields(requestTypeId);

      // Load available custom fields (for dropdown)
      const allCustomFields = await api.customFields.getAll();
      availableFields = [
        { id: 'title', name: 'Title', type: 'default' },
        { id: 'description', name: 'Description', type: 'default' },
        ...allCustomFields.map(f => ({ id: f.id.toString(), name: f.name, type: 'custom' }))
      ];
    } catch (err) {
      console.error('Failed to load request type fields:', err);
      error = err.message || 'Failed to load fields';
    } finally {
      loading = false;
    }
  }

  function clearForm() {
    addingField = false;
    addingVirtualField = false;
    newFieldIdentifier = '';
    newFieldType = 'default';
    newIsRequired = false;
    virtualFieldName = '';
    virtualFieldType = 'text';
    virtualFieldRequired = false;
    virtualFieldOptions = [{ value: '', label: '' }];
    editingField = null;
    error = null;
  }

  function startAddingField() {
    addingField = true;
    addingVirtualField = false;
  }

  function startAddingVirtualField() {
    addingVirtualField = true;
    addingField = false;
    virtualFieldName = '';
    virtualFieldType = 'text';
    virtualFieldRequired = false;
    virtualFieldOptions = [{ value: '', label: '' }];
  }

  function cancelAddingField() {
    addingField = false;
    addingVirtualField = false;
    newFieldIdentifier = '';
    newFieldType = 'default';
    newIsRequired = false;
    virtualFieldName = '';
    virtualFieldType = 'text';
    virtualFieldRequired = false;
    virtualFieldOptions = [{ value: '', label: '' }];
  }

  function addField() {
    if (!newFieldIdentifier) {
      error = 'Please select a field';
      return;
    }

    if (fields.some(f => f.field_identifier === newFieldIdentifier && (f.step_number || 1) === currentStep)) {
      error = 'This field is already added to this step';
      return;
    }

    const field = availableFields.find(f => f.id === newFieldIdentifier);
    const fieldName = field ? field.name : newFieldIdentifier;

    fields = [...fields, {
      field_identifier: newFieldIdentifier,
      field_type: newFieldType,
      is_required: newIsRequired,
      display_order: currentStepFields.length,
      field_name: fieldName,
      step_number: currentStep
    }];

    cancelAddingField();
    saveFields();
  }

  function addVirtualField() {
    if (!virtualFieldName.trim()) {
      error = 'Please enter a field name';
      return;
    }

    const fieldIdentifier = `vf_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;

    // Prepare options for select type
    let optionsJson = null;
    if (virtualFieldType === 'select') {
      const validOptions = virtualFieldOptions.filter(o => o.value.trim() && o.label.trim());
      if (validOptions.length === 0) {
        error = 'Please add at least one option for select field';
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

    cancelAddingField();
    saveFields();
  }

  function addVirtualFieldOption() {
    virtualFieldOptions = [...virtualFieldOptions, { value: '', label: '' }];
  }

  function removeVirtualFieldOption(index) {
    virtualFieldOptions = virtualFieldOptions.filter((_, i) => i !== index);
  }

  function removeField(fieldToRemove) {
    fields = fields.filter(f => f !== fieldToRemove);
    recalculateDisplayOrder();
    saveFields();
  }

  function moveFieldUp(field) {
    const stepFields = fields.filter(f => (f.step_number || 1) === (field.step_number || 1));
    const fieldIndex = stepFields.findIndex(f => f === field);
    if (fieldIndex <= 0) return;

    // Swap display_order with previous field
    const prevField = stepFields[fieldIndex - 1];
    const tempOrder = field.display_order;
    field.display_order = prevField.display_order;
    prevField.display_order = tempOrder;

    fields = [...fields];
    saveFields();
  }

  function moveFieldDown(field) {
    const stepFields = fields.filter(f => (f.step_number || 1) === (field.step_number || 1));
    const fieldIndex = stepFields.findIndex(f => f === field);
    if (fieldIndex >= stepFields.length - 1) return;

    // Swap display_order with next field
    const nextField = stepFields[fieldIndex + 1];
    const tempOrder = field.display_order;
    field.display_order = nextField.display_order;
    nextField.display_order = tempOrder;

    fields = [...fields];
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

  function toggleRequired(field) {
    field.is_required = !field.is_required;
    fields = [...fields];
    saveFields();
  }

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

  function addStep() {
    const maxStep = Math.max(...steps, 0);
    steps = [...steps, maxStep + 1];
    currentStep = maxStep + 1;
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

  async function saveFields() {
    try {
      saving = true;
      error = null;

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
      dispatch('saved');
    } catch (err) {
      console.error('Failed to save fields:', err);
      error = err.message || 'Failed to save fields';
    } finally {
      saving = false;
    }
  }

  function handleClose() {
    isOpen = false;
    dispatch('close');
  }

  function getFieldTypeLabel(field) {
    if (field.field_type === 'virtual') {
      const typeLabels = { text: 'Text', textarea: 'Multi-line', select: 'Select', checkbox: 'Checkbox' };
      return `Virtual - ${typeLabels[field.virtual_field_type] || field.virtual_field_type}`;
    }
    return field.field_type === 'default' ? 'Default Field' : 'Custom Field';
  }
</script>

{#if isOpen}
  <PortalModal
    isOpen={isOpen}
    isDarkMode={isDarkMode}
    maxWidth="max-w-3xl"
    title={`Configure Fields: ${requestTypeName}`}
    onClose={handleClose}
    bodyClass="px-6 py-4 max-h-[70vh] overflow-y-auto"
  >
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else}
      {#if error}
        <div
          class="mb-4 p-3 rounded border"
          style="background-color: {isDarkMode ? 'rgba(239, 68, 68, 0.1)' : '#fef2f2'}; border-color: {isDarkMode ? 'rgba(239, 68, 68, 0.3)' : '#fecaca'};"
        >
          <p class="text-sm" style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};">
            {error}
          </p>
        </div>
      {/if}

      <!-- Step Tabs -->
      <div class="flex items-center gap-2 mb-4 pb-3 border-b" style="border-color: {isDarkMode ? '#475569' : '#e5e7eb'};">
        {#each steps as step}
          <button
            onclick={() => currentStep = step}
            class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
            style="background-color: {currentStep === step ? (isDarkMode ? '#3b82f6' : '#3b82f6') : (isDarkMode ? '#334155' : '#f3f4f6')}; color: {currentStep === step ? '#ffffff' : (isDarkMode ? '#94a3b8' : '#6b7280')};"
          >
            Step {step}
          </button>
        {/each}
        <button
          onclick={addStep}
          class="px-3 py-2 rounded-lg text-sm transition-all flex items-center gap-1"
          style="background-color: {isDarkMode ? '#334155' : '#f3f4f6'}; color: {isDarkMode ? '#94a3b8' : '#6b7280'};"
          title="Add new step"
        >
          <Plus class="w-4 h-4" />
        </button>
        {#if steps.length > 1}
          <button
            onclick={() => removeStep(currentStep)}
            class="px-3 py-2 rounded-lg text-sm transition-all"
            style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};"
            title="Remove current step"
          >
            <Trash2 class="w-4 h-4" />
          </button>
        {/if}
      </div>

      <!-- Fields List for Current Step -->
      <div class="space-y-2 mb-4">
        {#each currentStepFields.sort((a, b) => a.display_order - b.display_order) as field, index}
          <div
            class="flex items-start gap-3 p-3 rounded border"
            style="background-color: {isDarkMode ? '#334155' : '#f9fafb'}; border-color: {isDarkMode ? '#475569' : '#e5e7eb'};"
          >
            <div class="flex flex-col gap-1 pt-1">
              <button
                onclick={() => moveFieldUp(field)}
                disabled={index === 0}
                class="p-1 rounded transition-all disabled:opacity-30"
                style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};"
              >
                <ChevronUp class="w-4 h-4" />
              </button>
              <button
                onclick={() => moveFieldDown(field)}
                disabled={index === currentStepFields.length - 1}
                class="p-1 rounded transition-all disabled:opacity-30"
                style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};"
              >
                <ChevronDown class="w-4 h-4" />
              </button>
            </div>

            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="font-medium text-sm" style="color: {isDarkMode ? '#e2e8f0' : '#111827'};">
                  {field.display_name || field.field_name || field.field_identifier}
                </span>
                {#if field.display_name && field.display_name !== field.field_name}
                  <span class="text-xs" style="color: {isDarkMode ? '#64748b' : '#9ca3af'};">
                    ({field.field_name || field.field_identifier})
                  </span>
                {/if}
              </div>
              <div class="text-xs mt-0.5" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                {getFieldTypeLabel(field)}
              </div>
              {#if field.description}
                <div class="text-xs mt-1 italic" style="color: {isDarkMode ? '#64748b' : '#9ca3af'};">
                  {field.description}
                </div>
              {/if}
            </div>

            <div class="flex items-center gap-2">
              <button
                onclick={() => startEditingField(field)}
                class="p-2 rounded transition-all"
                style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};"
                title="Edit display settings"
              >
                <Pencil class="w-4 h-4" />
              </button>

              <button
                onclick={() => toggleRequired(field)}
                class="flex items-center gap-1.5 px-2.5 py-1.5 rounded border transition-all text-xs"
                style="background-color: {field.is_required ? (isDarkMode ? '#1e40af' : '#dbeafe') : 'transparent'}; border-color: {isDarkMode ? '#475569' : '#e5e7eb'}; color: {field.is_required ? (isDarkMode ? '#60a5fa' : '#2563eb') : (isDarkMode ? '#94a3b8' : '#6b7280')};"
              >
                {#if field.is_required}
                  <Check class="w-3 h-3" />
                {/if}
                <span>Required</span>
              </button>

              <button
                onclick={() => removeField(field)}
                class="p-2 rounded transition-all"
                style="color: {isDarkMode ? '#fca5a5' : '#dc2626'}; background-color: {isDarkMode ? 'rgba(220, 38, 38, 0.1)' : 'transparent'};"
                title="Remove field"
              >
                <Trash2 class="w-4 h-4" />
              </button>
            </div>
          </div>
        {/each}

        {#if currentStepFields.length === 0 && !addingField && !addingVirtualField}
          <div class="text-center py-8">
            <p class="text-sm" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
              No fields in Step {currentStep}. Add fields below.
            </p>
          </div>
        {/if}
      </div>

      <!-- Add Field Form -->
      {#if addingField}
        <div
          class="p-4 rounded border space-y-3"
          style="background-color: {isDarkMode ? '#334155' : '#f9fafb'}; border-color: {isDarkMode ? '#475569' : '#e5e7eb'};"
        >
          <div class="text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
            Add Existing Field
          </div>
          <div>
            <label class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
              Field
            </label>
            <BasePicker
              bind:value={newFieldIdentifier}
              items={availableFields}
              placeholder="Select a field..."
              showUnassigned={true}
              unassignedLabel="Select a field..."
              getValue={(field) => field.id}
              getLabel={(field) => `${field.name} (${field.type})`}
              onSelect={(field) => {
                if (field) newFieldType = field.type;
              }}
            />
          </div>

          <div class="flex items-center gap-2">
            <input
              type="checkbox"
              bind:checked={newIsRequired}
              id="newFieldRequired"
              class="h-4 w-4 rounded border-gray-300 focus:ring-2 focus:ring-blue-500"
            />
            <label for="newFieldRequired" class="text-sm" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
              Required field
            </label>
          </div>

          <div class="flex gap-2">
            <Button
              onclick={addField}
              variant="primary"
              size="medium"
              class="flex-1"
            >
              Add Field
            </Button>
            <Button
              onclick={cancelAddingField}
              variant="default"
              size="medium"
            >
              Cancel
            </Button>
          </div>
        </div>

      <!-- Add Virtual Field Form -->
      {:else if addingVirtualField}
        <div
          class="p-4 rounded border space-y-3"
          style="background-color: {isDarkMode ? '#334155' : '#f9fafb'}; border-color: {isDarkMode ? '#475569' : '#e5e7eb'};"
        >
          <div class="text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
            Add Virtual Field
          </div>

          <div>
            <label class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
              Field Name
            </label>
            <input
              type="text"
              bind:value={virtualFieldName}
              placeholder="e.g., Urgency Level"
              class="w-full px-3 py-2 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
            />
          </div>

          <div>
            <label class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
              Field Type
            </label>
            <div class="grid grid-cols-4 gap-2">
              {#each [
                { value: 'text', label: 'Text', icon: Type },
                { value: 'textarea', label: 'Multi-line', icon: AlignLeft },
                { value: 'select', label: 'Select', icon: ListChecks },
                { value: 'checkbox', label: 'Checkbox', icon: ToggleLeft }
              ] as type}
                <button
                  onclick={() => virtualFieldType = type.value}
                  class="flex flex-col items-center gap-1 p-3 rounded border transition-all"
                  style="background-color: {virtualFieldType === type.value ? (isDarkMode ? '#1e40af' : '#dbeafe') : 'transparent'}; border-color: {virtualFieldType === type.value ? (isDarkMode ? '#3b82f6' : '#3b82f6') : (isDarkMode ? '#475569' : '#e5e7eb')}; color: {virtualFieldType === type.value ? (isDarkMode ? '#60a5fa' : '#2563eb') : (isDarkMode ? '#94a3b8' : '#6b7280')};"
                >
                  <svelte:component this={type.icon} class="w-5 h-5" />
                  <span class="text-xs">{type.label}</span>
                </button>
              {/each}
            </div>
          </div>

          {#if virtualFieldType === 'select'}
            <div>
              <label class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
                Options
              </label>
              <div class="space-y-2">
                {#each virtualFieldOptions as option, i}
                  <div class="flex gap-2">
                    <input
                      type="text"
                      bind:value={option.value}
                      placeholder="Value"
                      class="flex-1 px-3 py-2 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                      style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
                    />
                    <input
                      type="text"
                      bind:value={option.label}
                      placeholder="Label"
                      class="flex-1 px-3 py-2 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                      style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
                    />
                    <button
                      onclick={() => removeVirtualFieldOption(i)}
                      class="p-2 rounded"
                      style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};"
                      disabled={virtualFieldOptions.length === 1}
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>
                {/each}
                <button
                  onclick={addVirtualFieldOption}
                  class="text-sm flex items-center gap-1"
                  style="color: {isDarkMode ? '#60a5fa' : '#2563eb'};"
                >
                  <Plus class="w-4 h-4" /> Add option
                </button>
              </div>
            </div>
          {/if}

          <div class="flex items-center gap-2">
            <input
              type="checkbox"
              bind:checked={virtualFieldRequired}
              id="virtualFieldRequired"
              class="h-4 w-4 rounded border-gray-300 focus:ring-2 focus:ring-blue-500"
            />
            <label for="virtualFieldRequired" class="text-sm" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
              Required field
            </label>
          </div>

          <div class="flex gap-2">
            <Button
              onclick={addVirtualField}
              variant="primary"
              size="medium"
              class="flex-1"
            >
              Add Virtual Field
            </Button>
            <Button
              onclick={cancelAddingField}
              variant="default"
              size="medium"
            >
              Cancel
            </Button>
          </div>
        </div>

      <!-- Add Field Buttons -->
      {:else}
        <div class="flex gap-2">
          <button
            onclick={startAddingField}
            class="flex-1 flex items-center justify-center gap-2 px-4 py-3 rounded border-2 border-dashed transition-all"
            style="border-color: {isDarkMode ? '#475569' : '#d1d5db'}; color: {isDarkMode ? '#94a3b8' : '#6b7280'};"
          >
            <Plus class="w-5 h-5" />
            <span class="font-medium">Add Field</span>
          </button>
          <button
            onclick={startAddingVirtualField}
            class="flex-1 flex items-center justify-center gap-2 px-4 py-3 rounded border-2 border-dashed transition-all"
            style="border-color: {isDarkMode ? '#475569' : '#d1d5db'}; color: {isDarkMode ? '#94a3b8' : '#6b7280'};"
          >
            <Type class="w-5 h-5" />
            <span class="font-medium">Add Virtual Field</span>
          </button>
        </div>
      {/if}
    {/if}

    <div
      class="px-6 py-4 border-t flex items-center justify-between -mx-6 -mb-4 mt-6"
      style="background-color: {isDarkMode ? '#334155' : '#f9fafb'}; border-color: {isDarkMode ? '#475569' : '#e5e7eb'};"
    >
      <div class="text-sm" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
        {#if saving}
          <div class="flex items-center gap-2">
            <Spinner size="sm" />
            <span>Saving...</span>
          </div>
        {:else}
          Changes are saved automatically
        {/if}
      </div>
      <Button
        onclick={handleClose}
        variant="primary"
        size="medium"
      >
        Done
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
    title="Edit Field Display"
    onClose={cancelFieldEdit}
    bodyClass="px-6 py-4"
  >
    <div class="space-y-4">
      <div>
        <label class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
          Display Name
        </label>
        <input
          type="text"
          bind:value={editDisplayName}
          placeholder={editingField.field_name || editingField.field_identifier}
          class="w-full px-3 py-2 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
          style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
        />
        <p class="text-xs mt-1" style="color: {isDarkMode ? '#64748b' : '#9ca3af'};">
          Override the label shown in the portal form
        </p>
      </div>

      <div>
        <label class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
          Description / Help Text
        </label>
        <textarea
          bind:value={editDescription}
          placeholder="Enter help text to show below the field..."
          rows="3"
          class="w-full px-3 py-2 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
          style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
        ></textarea>
      </div>

      <div class="flex gap-2 pt-2">
        <Button
          onclick={saveFieldEdit}
          variant="primary"
          size="medium"
          class="flex-1"
        >
          Save
        </Button>
        <Button
          onclick={cancelFieldEdit}
          variant="default"
          size="medium"
        >
          Cancel
        </Button>
      </div>
    </div>
  </PortalModal>
{/if}
