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
    enableSingleClick = false, enableDoubleClick = false,
    onitemUpdated = null, onupdateError = null, onclick: onclickProp = null
  } = $props();

  let editorComponent = $state(null);
  let saving = false;

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

  // Keep simple field/custom-value updates here; field-specific orchestration
  // belongs to itemDetailStore and ListCellRenderer.
  async function handleSave(detail) {
    const { value } = detail;
    if (saving) return;

    try {
      saving = true;
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
      editorComponent?.confirmSave?.(value);
      onitemUpdated?.({ item: merged, field, value });
    } catch (error) {
      const errorMessage = error?.message || 'Failed to save changes';
      editorComponent?.rejectSave?.(errorMessage);
      onupdateError?.({ error: errorMessage, field, value });
    } finally {
      saving = false;
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
    bind:this={editorComponent}
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
    bind:this={editorComponent}
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
    bind:this={editorComponent}
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
    bind:this={editorComponent}
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
    bind:this={editorComponent}
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
{/if}
