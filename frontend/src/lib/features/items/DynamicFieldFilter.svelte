<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { X, Calendar, Pencil } from 'lucide-svelte';
  import FieldSelector from '../../pickers/FieldSelector.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import { api } from '../../api.js';

  // Boolean options for select
  const booleanOptions = [
    { value: 'true', label: 'True' },
    { value: 'false', label: 'False' }
  ];

  const dispatch = createEventDispatcher();

  export let filter = {
    field: null,
    operator: '=',
    value: '',
    values: [] // For IN operator
  };

  // Compact mode for narrow containers (like sidebar)
  export let compact = false;

  // Modal state for text input
  let showTextModal = false;
  let tempTextValue = '';

  let operatorOptions = [];
  let valueOptions = []; // For enum/select fields
  let loadingOptions = false;

  // Operator options based on field type
  const operatorsByType = {
    text: [
      { value: '=', label: 'equals' },
      { value: '!=', label: 'does not equal' },
      { value: '~', label: 'contains' }
    ],
    number: [
      { value: '=', label: 'equals' },
      { value: '!=', label: 'does not equal' },
      { value: '<', label: 'less than' },
      { value: '<=', label: 'less than or equal' },
      { value: '>', label: 'greater than' },
      { value: '>=', label: 'greater than or equal' }
    ],
    date: [
      { value: '=', label: 'on' },
      { value: '!=', label: 'not on' },
      { value: '<', label: 'before' },
      { value: '<=', label: 'on or before' },
      { value: '>', label: 'after' },
      { value: '>=', label: 'on or after' }
    ],
    enum: [
      { value: '=', label: 'is' },
      { value: '!=', label: 'is not' },
      { value: 'IN', label: 'is one of' },
      { value: 'NOT IN', label: 'is not one of' }
    ],
    select: [
      { value: '=', label: 'is' },
      { value: '!=', label: 'is not' },
      { value: 'IN', label: 'is one of' },
      { value: 'NOT IN', label: 'is not one of' }
    ],
    boolean: [
      { value: '=', label: 'is' }
    ],
    user: [
      { value: '=', label: 'is' },
      { value: '!=', label: 'is not' }
    ],
    reference: [
      { value: '=', label: 'is' },
      { value: '!=', label: 'is not' },
      { value: 'IN', label: 'is one of' },
      { value: 'NOT IN', label: 'is not one of' }
    ],
    identifier: [
      { value: '=', label: 'equals' },
      { value: '!=', label: 'does not equal' },
      { value: 'IN', label: 'is one of' },
      { value: 'NOT IN', label: 'is not one of' }
    ]
  };

  $: if (filter.field) {
    updateOperatorOptions(filter.field.type);
    loadValueOptions(filter.field);
  }

  function updateOperatorOptions(fieldType) {
    operatorOptions = operatorsByType[fieldType] || operatorsByType.text;

    // Reset operator if current one is not valid for the new field type
    const validOperators = operatorOptions.map(op => op.value);
    if (!validOperators.includes(filter.operator)) {
      const newOperator = operatorOptions[0]?.value || '=';
      dispatch('change', {
        ...filter,
        operator: newOperator
      });
    }
  }

  async function loadValueOptions(field) {
    if (!field) return;

    // Load options for enum/select fields
    if (field.type === 'enum' || field.type === 'select') {
      loadingOptions = true;
      try {
        if (field.id === 'status') {
          // Load status options
          const statuses = await api.statuses.getAll();
          valueOptions = (statuses || []).map(s => ({ value: s.name, label: s.name }));
        } else if (field.id === 'priority') {
          // Static priority options
          valueOptions = [
            { value: 'low', label: 'Low' },
            { value: 'medium', label: 'Medium' },
            { value: 'high', label: 'High' },
            { value: 'critical', label: 'Critical' }
          ];
        } else if (field.id === 'itemType') {
          // Load item type options from API
          const itemTypes = await api.itemTypes.getAll();
          valueOptions = (itemTypes || []).map(t => ({
            value: t.name,
            label: t.name
          }));
        } else if (field.id === 'milestone') {
          // Load milestone options from API
          const milestones = await api.milestones.getAll();
          valueOptions = (milestones || []).map(m => ({
            value: m.name,
            label: m.name
          }));
        } else if (field.id === 'project') {
          // Load project options from API
          const projects = await api.projects.getAll();
          valueOptions = (projects || []).map(p => ({
            value: p.name,
            label: p.name
          }));
        } else if (field.id === 'workspace') {
          // Load workspace options from API
          const workspaces = await api.workspaces.getAll();
          valueOptions = (workspaces || []).map(w => ({
            value: w.name,
            label: w.name
          }));
        } else if (field.options) {
          // Custom field with predefined options
          valueOptions = field.options.map(opt => ({ value: opt, label: opt }));
        }
      } catch (error) {
        console.error('Failed to load value options:', error);
        valueOptions = [];
      } finally {
        loadingOptions = false;
      }
    }
  }

  function handleFieldSelect(event) {
    dispatch('change', {
      ...filter,
      field: event.detail,
      value: '',
      values: []
    });
  }

  function handleFieldClear() {
    dispatch('change', {
      ...filter,
      field: null,
      value: '',
      values: []
    });
  }

  function handleOperatorChange(event) {
    const newOperator = event.target.value;

    // Reset value/values based on new operator
    if (newOperator === 'IN' || newOperator === 'NOT IN') {
      dispatch('change', {
        ...filter,
        operator: newOperator,
        values: [],
        value: ''
      });
    } else {
      dispatch('change', {
        ...filter,
        operator: newOperator,
        value: '',
        values: []
      });
    }
  }

  function handleValueChange(event) {
    dispatch('change', {
      ...filter,
      value: event.target.value
    });
  }

  function handleValueKeydown(event) {
    if (event.key === 'Enter') {
      event.preventDefault();
      dispatch('execute');
    }
  }

  function handleMultiValueToggle(value) {
    const newValues = filter.values.includes(value)
      ? filter.values.filter(v => v !== value)
      : [...filter.values, value];

    dispatch('change', {
      ...filter,
      values: newValues
    });
  }

  function handleRemove() {
    dispatch('remove');
  }

  function handleMilestoneSelect(event) {
    dispatch('change', {
      ...filter,
      value: event.detail.value,  // milestone ID
      displayValue: event.detail.milestone?.name  // for display
    });
  }

  function isMultiValueOperator(operator) {
    return operator === 'IN' || operator === 'NOT IN';
  }

  function openTextModal() {
    tempTextValue = filter.value || '';
    showTextModal = true;
  }

  function closeTextModal() {
    showTextModal = false;
  }

  function applyTextValue() {
    dispatch('change', {
      ...filter,
      value: tempTextValue
    });
    dispatch('execute');
    showTextModal = false;
  }

  function clearTextValue() {
    tempTextValue = '';
    dispatch('change', {
      ...filter,
      value: ''
    });
    dispatch('execute');
    showTextModal = false;
  }
</script>

<div
  class={compact ? "flex flex-col gap-2" : "flex items-start gap-2 p-2.5 rounded border"}
  style={compact ? "" : "background-color: var(--ds-surface-raised); border-color: var(--ds-border);"}
>
  <!-- Header row: Field Selector + Remove button (compact) -->
  <div class={compact ? "flex items-start gap-2 w-full" : "flex-1 min-w-0"} style={compact ? "" : "max-width: 250px;"}>
    <div class={compact ? "flex-1" : ""}>
      <FieldSelector
        selectedField={filter.field}
        placeholder="Choose field..."
        on:select={handleFieldSelect}
        on:clear={handleFieldClear}
      />
    </div>
  </div>

  {#if filter.field}
    <!-- Operator + Value row -->
    <div class={compact ? "flex gap-2 w-full" : "contents"}>
      <!-- Operator Selector -->
      <div class={compact ? "flex-shrink-0" : ""} style={compact ? "width: 90px;" : "min-width: 150px;"}>
        <BasePicker
          value={filter.operator}
          items={operatorOptions}
          placeholder={compact ? "=" : "Select operator"}
          getValue={(item) => item.value}
          getLabel={(item) => compact ? item.value : item.label}
          onSelect={(item) => {
            if (item) {
              const newOperator = item.value;
              if (newOperator === 'IN' || newOperator === 'NOT IN') {
                dispatch('change', { ...filter, operator: newOperator, values: [], value: '' });
              } else {
                dispatch('change', { ...filter, operator: newOperator, values: [] });
              }
            }
          }}
        />
      </div>

      <!-- Value Input -->
      <div class={compact ? "flex-1 min-w-0" : "flex-1"} style={compact ? "" : "min-width: 200px;"}>
      {#if isMultiValueOperator(filter.operator)}
        <!-- Multi-value selector for IN/NOT IN -->
        {#if valueOptions.length > 0}
          <div class="border rounded p-2 max-h-32 overflow-y-auto" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
            {#each valueOptions as option}
              <div class="py-1 px-2 rounded filter-option-hover">
                <Checkbox
                  checked={filter.values.includes(option.value)}
                  onchange={() => handleMultiValueToggle(option.value)}
                  label={option.label}
                  size="small"
                />
              </div>
            {/each}
          </div>
        {:else}
          <!-- Multi-value text input via modal -->
          <button
            type="button"
            onclick={openTextModal}
            class="w-full flex items-center gap-2 px-3 py-2 text-sm border rounded transition-colors text-left"
            style="background-color: var(--ds-surface); border-color: var(--ds-border);"
          >
            {#if filter.value}
              <span class="truncate flex-1" style="color: var(--ds-text);">{filter.value}</span>
              <button
                type="button"
                onclick={(e) => { e.stopPropagation(); clearTextValue(); }}
                class="p-0.5 rounded transition-colors flex-shrink-0"
                style="color: var(--ds-text-subtle);"
                title="Clear value"
              >
                <X class="w-3 h-3" />
              </button>
            {:else}
              <span style="color: var(--ds-text-subtle);">Enter comma-separated values...</span>
              <Pencil class="w-3 h-3 flex-shrink-0 ml-auto" style="color: var(--ds-text-subtle);" />
            {/if}
          </button>
        {/if}
      {:else if filter.field.id === 'milestone'}
        <!-- Milestone picker -->
        <MilestoneCombobox
          value={filter.value}
          placeholder="Select milestone..."
          on:select={handleMilestoneSelect}
        />
      {:else if filter.field.type === 'enum' || filter.field.type === 'select'}
        <!-- Dropdown for enum/select fields -->
        {#if loadingOptions}
          <div class="px-3 py-2 text-sm" style="color: var(--ds-text-subtle);">Loading options...</div>
        {:else if valueOptions.length > 0}
          <BasePicker
            value={filter.value}
            items={valueOptions}
            placeholder="Select value..."
            showUnassigned={true}
            unassignedLabel="Select value..."
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
            onSelect={(item) => {
              dispatch('change', { ...filter, value: item ? item.value : '' });
            }}
          />
        {:else}
          <!-- Fallback text input via modal for enum/select with no options -->
          <button
            type="button"
            onclick={openTextModal}
            class="w-full flex items-center gap-2 px-3 py-2 text-sm border rounded transition-colors text-left"
            style="background-color: var(--ds-surface); border-color: var(--ds-border);"
          >
            {#if filter.value}
              <span class="truncate flex-1" style="color: var(--ds-text);">{filter.value}</span>
              <button
                type="button"
                onclick={(e) => { e.stopPropagation(); clearTextValue(); }}
                class="p-0.5 rounded transition-colors flex-shrink-0"
                style="color: var(--ds-text-subtle);"
                title="Clear value"
              >
                <X class="w-3 h-3" />
              </button>
            {:else}
              <span style="color: var(--ds-text-subtle);">Enter value...</span>
              <Pencil class="w-3 h-3 flex-shrink-0 ml-auto" style="color: var(--ds-text-subtle);" />
            {/if}
          </button>
        {/if}
      {:else if filter.field.type === 'date'}
        <!-- Date input -->
        <div class="relative">
          <input
            type="date"
            value={filter.value}
            oninput={handleValueChange}
            onkeydown={handleValueKeydown}
            class="w-full px-3 py-2 pr-10 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
          <Calendar class="w-4 h-4 absolute right-3 top-1/2 transform -translate-y-1/2 pointer-events-none" style="color: var(--ds-text-subtle);" />
        </div>
      {:else if filter.field.type === 'number'}
        <!-- Number input -->
        <input
          type="number"
          placeholder="Enter number..."
          value={filter.value}
          oninput={handleValueChange}
          onkeydown={handleValueKeydown}
          class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
          style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
        />
      {:else if filter.field.type === 'boolean'}
        <!-- Boolean select -->
        <BasePicker
          value={filter.value}
          items={booleanOptions}
          placeholder="Select value..."
          showUnassigned={true}
          unassignedLabel="Select value..."
          getValue={(item) => item.value}
          getLabel={(item) => item.label}
          onSelect={(item) => {
            dispatch('change', { ...filter, value: item ? item.value : '' });
          }}
        />
      {:else}
        <!-- Text input via modal -->
        <button
          type="button"
          onclick={openTextModal}
          class="w-full flex items-center gap-2 px-3 py-2 text-sm border rounded transition-colors text-left"
          style="background-color: var(--ds-surface); border-color: var(--ds-border);"
        >
          {#if filter.value}
            <span class="truncate flex-1" style="color: var(--ds-text);">{filter.value}</span>
            <button
              type="button"
              onclick={(e) => { e.stopPropagation(); clearTextValue(); }}
              class="p-0.5 rounded transition-colors flex-shrink-0"
              style="color: var(--ds-text-subtle);"
              title="Clear value"
            >
              <X class="w-3 h-3" />
            </button>
          {:else}
            <span style="color: var(--ds-text-subtle);">Enter value...</span>
            <Pencil class="w-3 h-3 flex-shrink-0 ml-auto" style="color: var(--ds-text-subtle);" />
          {/if}
        </button>
      {/if}
      </div>
    </div>
  {/if}

  <!-- Remove Button (only show in non-compact mode, as it's in header for compact) -->
  {#if !compact}
    <button
      type="button"
      onclick={handleRemove}
      class="p-2 rounded transition-colors"
      style="color: var(--ds-text-subtle);"
      title="Remove filter"
    >
      <X class="w-5 h-5" />
    </button>
  {/if}
</div>

<!-- Text Input Modal -->
<Modal bind:isOpen={showTextModal} maxWidth="max-w-md" onclose={closeTextModal} onSubmit={applyTextValue}>
  <div class="p-4">
    <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      {filter.field?.label || 'Enter Value'}
    </h3>
    <input
      type="text"
      bind:value={tempTextValue}
      placeholder="Enter value..."
      class="w-full px-3 py-2 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
      style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
    />
    <div class="flex justify-end gap-2 mt-4">
      <Button variant="ghost" size="sm" onclick={clearTextValue}>Clear</Button>
      <Button variant="ghost" size="sm" onclick={closeTextModal}>Cancel</Button>
      <Button variant="primary" size="sm" onclick={applyTextValue}>Apply</Button>
    </div>
  </div>
</Modal>

<style>
  .filter-option-hover:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
