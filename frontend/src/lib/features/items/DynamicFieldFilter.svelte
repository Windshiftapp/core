<script>
  import { X, Calendar, Pencil } from '@lucide/svelte';
  import FieldSelector from '../../pickers/FieldSelector.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import IterationCombobox from '../../pickers/IterationCombobox.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Input from '../../components/Input.svelte';
  import { api } from '../../api.js';
	import { booleanOptions, operatorsByType, isMultiValueOperator, isNullOperator } from '../shared/filterOperators.js';

  let {
    filter = {
      field: null,
      operator: '=',
      value: '',
      values: [] // For IN operator
    },
    compact = false,
    testIdPrefix = null,
    fieldGroups = null,
    customFieldItems = null,
    optionLoader = null,
    specializedPickers = true,
    onchange = undefined,
    onremove = undefined,
    onexecute = undefined,
  } = $props();

  // Modal state for text input
  let showTextModal = $state(false);
  let tempTextValue = $state('');

  let operatorOptions = $derived(
    operatorsByType[filter.field?.type] || operatorsByType.text
  );
  let valueOptions = $state([]); // For enum/select fields
  let loadingOptions = $state(false);

  let lastLoadedFieldId = null;
  let valueLoadToken = 0;
  let loadedIterations = $state([]);

  // Map filter.values (iteration names) to iteration IDs for the picker
  let iterationMultiValues = $derived(
    filter.values
      .map(name => {
        const iter = loadedIterations.find(i => i.name === name);
        return iter?.id;
      })
      .filter(Boolean)
  );

  function handleIterationMultiSelect(selectedIds) {
    const names = selectedIds
      .map(id => loadedIterations.find(i => i.id === id)?.name)
      .filter(Boolean);
    onchange?.({
      ...filter,
      values: names
    });
  }

  // Load iterations when needed for multi-select mapping
  async function ensureIterationsLoaded() {
    if (loadedIterations.length > 0) return;
    try {
      const iters = await api.iterations.getAll();
      loadedIterations = iters || [];
    } catch (err) {
      console.error('Failed to load iterations:', err);
    }
  }

  $effect(() => {
    if (filter.field) {
      if (!operatorOptions.some((operator) => operator.value === filter.operator)) {
        onchange?.({ ...filter, operator: operatorOptions[0]?.value || '=' });
      }
      loadValueOptions(filter.field);
    }
  });

  $effect(() => {
    if (specializedPickers && filter.field?.id === 'iteration' && isMultiValueOperator(filter.operator)) {
      ensureIterationsLoaded();
    }
  });

  async function loadValueOptions(field) {
    if (!field || field.id === lastLoadedFieldId) return;
    lastLoadedFieldId = field.id;

    if (optionLoader) {
      const token = ++valueLoadToken;
      loadingOptions = true;
      try {
        const options = await optionLoader(field);
        if (token === valueLoadToken) valueOptions = options || [];
      } catch (error) {
        if (token !== valueLoadToken) return;
        console.error('Failed to load filter options:', error);
        valueOptions = [];
      } finally {
        if (token === valueLoadToken) loadingOptions = false;
      }
      return;
    }

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
          // Custom field with predefined options (handle both new and legacy format)
          if (field.options.items && Array.isArray(field.options.items)) {
            // New ID-based format: {next_id, items: [{id, label}]}
            valueOptions = field.options.items.map(opt => ({ value: opt.id, label: opt.label }));
          } else if (Array.isArray(field.options)) {
            // Legacy string array format
            valueOptions = field.options.map(opt => ({ value: opt, label: opt }));
          }
        }
      } catch (error) {
        console.error('Failed to load value options:', error);
        valueOptions = [];
      } finally {
        loadingOptions = false;
      }
    } else if (field.type === 'user') {
      loadingOptions = true;
      try {
        const groupList = await api.groups.getAll();
        valueOptions = (groupList || []).map(g => ({
          value: g.name,
          label: g.name
        }));
      } catch (error) {
        console.error('Failed to load group options:', error);
        valueOptions = [];
      } finally {
        loadingOptions = false;
      }
    } else {
      valueOptions = [];
    }
  }

  function handleFieldSelect(field) {
    const ops = operatorsByType[field.type] || operatorsByType.text;
    operatorOptions = ops;
    const validOps = ops.map(op => op.value);
    const newOperator = validOps.includes(filter.operator) ? filter.operator : (ops[0]?.value || '=');
    onchange?.({
      ...filter,
      field: field,
      operator: newOperator,
      value: '',
      values: []
    });
  }

  function handleFieldClear() {
    onchange?.({
      ...filter,
      field: null,
      value: '',
      values: []
    });
  }

  function handleOperatorChange(event) {
    const newOperator = event.target.value;

    // Reset value/values based on new operator
	if (isMultiValueOperator(newOperator)) {
      onchange?.({
        ...filter,
        operator: newOperator,
        values: [],
        value: ''
      });
	} else {
      onchange?.({
        ...filter,
        operator: newOperator,
        value: '',
        values: []
      });
    }
  }

  function handleValueChange(event) {
    onchange?.({
      ...filter,
      value: event.target.value
    });
  }

  function handleValueKeydown(event) {
    if (event.key === 'Enter') {
      event.preventDefault();
      onexecute?.();
    }
  }

  function handleMultiValueToggle(value) {
    const newValues = filter.values.includes(value)
      ? filter.values.filter(v => v !== value)
      : [...filter.values, value];

    onchange?.({
      ...filter,
      values: newValues
    });
  }

  function handleRemove() {
    onremove?.();
  }

  function handleMilestoneSelect(result) {
    onchange?.({
      ...filter,
      value: result.value,  // milestone ID
      displayValue: result.milestone?.name  // for display
    });
  }

  function handleIterationSelect(result) {
    onchange?.({
      ...filter,
      value: result.value,  // iteration ID
      displayValue: result.iteration?.name  // for display
    });
  }

  function openTextModal() {
    tempTextValue = filter.value || '';
    showTextModal = true;
  }

  function closeTextModal() {
    showTextModal = false;
  }

  function applyTextValue() {
    onchange?.({
      ...filter,
      value: tempTextValue
    });
    onexecute?.();
    showTextModal = false;
  }

  function clearTextValue() {
    tempTextValue = '';
    onchange?.({
      ...filter,
      value: ''
    });
    onexecute?.();
    showTextModal = false;
  }
</script>

<div
  data-testid={testIdPrefix || undefined}
  class={compact ? "flex flex-col gap-2" : "flex items-start gap-2 p-2.5 rounded border"}
  style={compact ? "" : "background-color: var(--ds-surface-raised); border-color: var(--ds-border);"}
>
  <!-- Header row: Field Selector + Remove button (compact) -->
  <div class={compact ? "flex items-start gap-2 w-full" : "flex-1 min-w-0"} style={compact ? "" : "max-width: 250px;"}>
    <div data-testid={testIdPrefix ? `${testIdPrefix}-field` : undefined} class={compact ? "flex-1" : ""}>
      <FieldSelector
        selectedField={filter.field}
        placeholder="Choose field..."
        {fieldGroups}
        {customFieldItems}
        onSelect={handleFieldSelect}
        onClear={handleFieldClear}
      />
    </div>
  </div>

  {#if filter.field}
    <!-- Operator + Value row -->
    <div class={compact ? "flex gap-2 w-full" : "contents"}>
      <!-- Operator Selector -->
      <div data-testid={testIdPrefix ? `${testIdPrefix}-operator` : undefined} class={compact ? "flex-shrink-0" : ""} style={compact ? "width: 90px;" : "min-width: 150px;"}>
        <BasePicker
          value={filter.operator}
          items={operatorOptions}
          placeholder={compact ? "=" : "Select operator"}
          getValue={(item) => item.value}
          getLabel={(item) => compact ? item.value : item.label}
          onSelect={(item) => {
            if (item) {
              const newOperator = item.value;
			  if (isMultiValueOperator(newOperator)) {
                onchange?.({ ...filter, operator: newOperator, values: [], value: '' });
			  } else if (isNullOperator(newOperator)) {
				onchange?.({ ...filter, operator: newOperator, value: '', values: [] });
              } else {
                onchange?.({ ...filter, operator: newOperator, values: [] });
              }
            }
          }}
        />
      </div>

      <!-- Value Input -->
      <div data-testid={testIdPrefix ? `${testIdPrefix}-value` : undefined} class={compact ? "flex-1 min-w-0" : "flex-1"} style={compact ? "" : "min-width: 200px;"}>
	  {#if isNullOperator(filter.operator)}
		<div class="px-3 py-2 text-sm" style="color: var(--ds-text-subtle);">No value required</div>
	  {:else if isMultiValueOperator(filter.operator)}
        <!-- Multi-value selector for IN/NOT IN -->
        {#if specializedPickers && filter.field.id === 'iteration'}
          <IterationCombobox
            multiSelect={true}
            values={iterationMultiValues}
            placeholder="Search iterations..."
            onSelect={handleIterationMultiSelect}
          />
        {:else if loadingOptions}
          <div class="px-3 py-2 text-sm" style="color: var(--ds-text-subtle);">Loading options...</div>
        {:else if valueOptions.length > 0}
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
          <div
            role="button"
            tabindex="0"
            onclick={openTextModal}
            onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openTextModal(); } }}
            class="w-full flex items-center gap-2 px-3 py-2 text-sm border rounded transition-colors text-left cursor-pointer"
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
              <span style="color: var(--ds-text-subtle);">{filter.field?.type === 'user' ? 'Enter group names or usernames...' : 'Enter comma-separated values...'}</span>
              <Pencil class="w-3 h-3 flex-shrink-0 ml-auto" style="color: var(--ds-text-subtle);" />
            {/if}
          </div>
        {/if}
      {:else if specializedPickers && filter.field.id === 'milestone'}
        <!-- Milestone picker -->
        <MilestoneCombobox
          value={filter.value}
          placeholder="Select milestone..."
          onSelect={handleMilestoneSelect}
        />
      {:else if specializedPickers && filter.field.id === 'iteration'}
        <!-- Iteration picker -->
        <IterationCombobox
          value={filter.value}
          placeholder="Select iteration..."
          onSelect={handleIterationSelect}
        />
      {:else if specializedPickers && filter.field.type === 'user'}
        <!-- User picker -->
        <UserPicker
          value={filter.value}
          placeholder="Select user..."
          showUnassigned={true}
          unassignedLabel="Unassigned"
          onSelect={(user) => {
            onchange?.({ ...filter, value: user ? user.id : '' });
          }}
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
              onchange?.({ ...filter, value: item ? item.value : '' });
            }}
          />
        {:else}
          <!-- Fallback text input via modal for enum/select with no options -->
          <div
            role="button"
            tabindex="0"
            onclick={openTextModal}
            onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openTextModal(); } }}
            class="w-full flex items-center gap-2 px-3 py-2 text-sm border rounded transition-colors text-left cursor-pointer"
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
          </div>
        {/if}
      {:else if filter.field.type === 'date'}
        <!-- Date input -->
        <div class="relative">
          <Input
            type="date"
            value={filter.value}
            oninput={handleValueChange}
            onkeydown={handleValueKeydown}
            class="pr-10"
            size="small"
          />
          <Calendar class="w-4 h-4 absolute right-3 top-1/2 transform -translate-y-1/2 pointer-events-none" style="color: var(--ds-text-subtle);" />
        </div>
      {:else if filter.field.type === 'number'}
        <!-- Number input -->
        <Input
          type="number"
          placeholder="Enter number..."
          value={filter.value}
          oninput={handleValueChange}
          onkeydown={handleValueKeydown}
          size="small"
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
            onchange?.({ ...filter, value: item ? item.value : '' });
          }}
        />
      {:else}
        <!-- Text input via modal -->
        <div
          role="button"
          tabindex="0"
          onclick={openTextModal}
          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openTextModal(); } }}
          class="w-full flex items-center gap-2 px-3 py-2 text-sm border rounded transition-colors text-left cursor-pointer"
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
        </div>
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
    <Input
      dataTestid={testIdPrefix ? `${testIdPrefix}-value-input` : undefined}
      type="text"
      bind:value={tempTextValue}
      placeholder="Enter value..."
      size="small"
    />
    <div class="flex justify-end gap-2 mt-4">
      <Button variant="ghost" size="sm" onclick={clearTextValue}>Clear</Button>
      <Button variant="ghost" size="sm" onclick={closeTextModal}>Cancel</Button>
      <Button dataTestid={testIdPrefix ? `${testIdPrefix}-apply-value` : undefined} variant="primary" size="sm" onclick={applyTextValue}>Apply</Button>
    </div>
  </div>
</Modal>

<style>
  .filter-option-hover:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
