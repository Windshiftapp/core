<script>
  import { items } from '../api/items.js';
  import { updateCustomFieldValue } from '../utils/customFieldValueUpdates.js';
  import InlineTextEditor from './InlineTextEditor.svelte';
  import InlineSelectEditor from './InlineSelectEditor.svelte';
  import InlineDateEditor from './InlineDateEditor.svelte';
  import InlineNumberEditor from './InlineNumberEditor.svelte';
  import InlineDurationEditor from './InlineDurationEditor.svelte';

  let {
    item, field, fieldType = 'text', options = [], placeholder = '',
    required = false, disabled = false, className = '',
    maxLines = null,
    enableSingleClick = false, enableDoubleClick = false,
    onitemUpdated = null, onupdateError = null, onclick: onclickProp = null
  } = $props();

  // Get current field value
  const fieldValue = $derived(getFieldValue(item, field));

  function getFieldValue(item, field) {
    if (!item) return null;
    if (field === 'title') return item.title || '';
    if (field === 'description') return item.description || '';
    if (field.startsWith('custom_field_')) {
      const fieldId = field.replace('custom_field_', '');
      return item.custom_field_values?.[fieldId] || null;
    }
    return item[field] ?? null;
  }

  // Returns the promise so the editor's commit can await it and surface a
  // failure inline instead of via the imperative confirm/reject dance.
  // Keep simple field/custom-value updates here; field-specific orchestration
  // belongs to itemDetailStore and ListCellRenderer.
  async function handleSave(detail) {
    const { value } = detail;
    try {
      let updatedItem;
      if (field.startsWith('custom_field_')) {
        const fieldId = field.replace('custom_field_', '');
        // The server merges custom field patches per field, so a single-field
        // payload leaves the item's other fields untouched.
        updatedItem = await updateCustomFieldValue(item.id, fieldId, value);
      } else {
        updatedItem = await items.update(item.id, { [field]: value });
      }
      const merged = { ...item, ...updatedItem };
      onitemUpdated?.({ item: merged, field, value });
      return merged;
    } catch (error) {
      onupdateError?.({ error: error?.message || 'Failed to save changes', field, value });
      throw error;
    }
  }

  function handleClick() {
    if (enableSingleClick) {
      onclickProp?.();
    }
  }
</script>

{#if fieldType === 'select'}
  <InlineSelectEditor
    value={fieldValue}
    {options}
    {placeholder}
    {required}
    {disabled}
    {className}
    onsave={handleSave}
  />
{:else if fieldType === 'date'}
  <InlineDateEditor
    value={fieldValue}
    {placeholder}
    {required}
    {disabled}
    {className}
    {enableSingleClick}
    {enableDoubleClick}
    onsave={handleSave}
    onclick={handleClick}
  />
{:else if fieldType === 'number'}
  <InlineNumberEditor
    value={fieldValue}
    {placeholder}
    {required}
    {disabled}
    {className}
    {enableSingleClick}
    {enableDoubleClick}
    onsave={handleSave}
    onclick={handleClick}
  />
{:else if fieldType === 'duration'}
  <InlineDurationEditor
    value={fieldValue}
    {placeholder}
    {required}
    {disabled}
    {className}
    {enableSingleClick}
    {enableDoubleClick}
    onsave={handleSave}
    onclick={handleClick}
  />
{:else}
  <InlineTextEditor
    value={fieldValue}
    {placeholder}
    {required}
    {disabled}
    {className}
    {maxLines}
    {enableSingleClick}
    {enableDoubleClick}
    onsave={handleSave}
    onclick={handleClick}
  />
{/if}
