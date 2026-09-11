<script>
  import { X, Calendar, Pencil } from '@lucide/svelte';
  import FieldSelector from '../../pickers/FieldSelector.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import { api } from '../../api.js';
  import { findQlCompletionField } from '../../utils/qlCompletion.js';
  import { parseFieldOptions } from '../../utils/optionUtils.js';
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
  let hasValuePicker = $state(false);
  let serverValueHelp = $state(null);
  let valueLoadToken = 0;
  let loadedFieldKey = null;

  const selectedPickerValue = $derived(matchOptionValue(filter.value));
  const selectedPickerValues = $derived(
    (filter.values || []).map((value) => matchOptionValue(value))
  );

  function matchOptionValue(value) {
    return valueOptions.find((option) => String(option.value) === String(value))?.value ?? value;
  }

  function fieldCanHaveValueHelp(field) {
    return Boolean(field?.completion?.value_help || field?.completion?.values?.length) ||
      ['enum', 'select', 'multiselect', 'user', 'reference', 'milestone', 'iteration'].includes(field?.type);
  }

  $effect(() => {
    const field = filter.field;
    if (!field) {
      loadedFieldKey = null;
      return;
    }
    if (!operatorOptions.some((operator) => operator.value === filter.operator)) {
      onchange?.({ ...filter, operator: operatorOptions[0]?.value || '=' });
    }
    const fieldKey = `${field.id ?? ''}:${field.customFieldId ?? ''}`;
    if (fieldKey !== loadedFieldKey) {
      loadedFieldKey = fieldKey;
      loadValueOptions(field);
    }
  });

  async function loadValueOptions(field, query = '') {
    if (!field) return;

    const token = ++valueLoadToken;
    hasValuePicker = false;
    serverValueHelp = null;
    valueOptions = [];

    if (!optionLoader && !fieldCanHaveValueHelp(field)) {
      loadingOptions = false;
      return;
    }
    loadingOptions = true;

    if (optionLoader) {
      try {
        const options = await optionLoader(field);
        if (token !== valueLoadToken) return;
        valueOptions = options || [];
        hasValuePicker = valueOptions.length > 0;
      } catch (error) {
        if (token !== valueLoadToken) return;
        console.error('Failed to load filter options:', error);
        valueOptions = [];
      } finally {
        if (token === valueLoadToken) loadingOptions = false;
      }
      return;
    }

    try {
      let completion = findQlCompletionField(null, field);
      if (!completion) {
        const catalog = await api.queryLanguage.getCatalog();
        completion = findQlCompletionField(catalog, field);
      }
      if (token !== valueLoadToken) return;

      if (completion?.value_help) {
        serverValueHelp = completion.value_help;
        hasValuePicker = true;
        const rows = await api.queryLanguage.getValues(completion.value_help, query);
        if (token !== valueLoadToken) return;
        valueOptions = (rows || []).map((row) => ({ value: row.value, label: row.label }));
      } else if (completion?.values?.length) {
        hasValuePicker = true;
        valueOptions = completion.values.map((row) => ({ value: row.value, label: row.label }));
      } else if (field.options) {
        valueOptions = parseFieldOptions(field.options).items.map((option) => ({
          value: option.id,
          label: option.label,
        }));
        hasValuePicker = valueOptions.length > 0;
      }
    } catch (error) {
      if (token !== valueLoadToken) return;
      console.error('Failed to load filter options:', error);
      valueOptions = [];
      hasValuePicker = false;
      serverValueHelp = null;
    } finally {
      if (token === valueLoadToken) loadingOptions = false;
    }
  }

  function searchValueOptions(query) {
    if (serverValueHelp) void loadValueOptions(filter.field, query);
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

  function handleRemove() {
    onremove?.();
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
        {#if (loadingOptions && fieldCanHaveValueHelp(filter.field)) || hasValuePicker}
          <BasePicker
            id={testIdPrefix ? `${testIdPrefix}-value-search` : undefined}
            inputTestid={testIdPrefix ? `${testIdPrefix}-value-search` : undefined}
            value={selectedPickerValues}
            items={valueOptions}
            loading={loadingOptions}
            multiple={true}
            placeholder="Search values..."
            ariaLabel="Search values"
            serverSearch={Boolean(serverValueHelp)}
            onSearchChange={searchValueOptions}
            getValue={(item) => item.value}
            getLabel={(item) => item.label}
            optionTestid={(option) => testIdPrefix ? `${testIdPrefix}-value-option-${option.value}` : undefined}
            onChange={(values) => onchange?.({ ...filter, values, value: '' })}
          />
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
      {:else if (loadingOptions && fieldCanHaveValueHelp(filter.field)) || hasValuePicker}
        <BasePicker
          id={testIdPrefix ? `${testIdPrefix}-value-search` : undefined}
          inputTestid={testIdPrefix ? `${testIdPrefix}-value-search` : undefined}
          value={selectedPickerValue}
          items={valueOptions}
          loading={loadingOptions}
          placeholder="Search values..."
          ariaLabel="Search values"
          showUnassigned={true}
          unassignedLabel="Select value..."
          serverSearch={Boolean(serverValueHelp)}
          onSearchChange={searchValueOptions}
          getValue={(item) => item.value}
          getLabel={(item) => item.label}
          optionTestid={(option) => testIdPrefix ? `${testIdPrefix}-value-option-${option.value}` : undefined}
          onSelect={(item) => onchange?.({ ...filter, value: item ? item.value : '' })}
        />
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
