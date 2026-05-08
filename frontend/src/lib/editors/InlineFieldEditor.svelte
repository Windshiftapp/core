<script>
  import { api } from '../api.js';
  import InlineTextEditor from './InlineTextEditor.svelte';
  import InlineSelectEditor from './InlineSelectEditor.svelte';
  import InlineDateEditor from './InlineDateEditor.svelte';

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

  // InlineFieldEditor only ever wraps simple string/date/select edits whose
  // payload is `{ [field]: value }` (or a custom_field_values merge). Field-
  // specific orchestration (status transitions, optimistic updates, edit
  // lifecycle, joined display names) lives in itemDetailStore.saveField for
  // the sidebar and in ListCellRenderer.handleItemUpdate for inline pickers.
  // Keep this component a thin pass-through and don't reintroduce a
  // field-mapping switch here.
  async function handleSave(detail) {
    const { value } = detail;
    if (saving) return;

    try {
      saving = true;
      let updateData;
      if (field.startsWith('custom_field_')) {
        const fieldId = field.replace('custom_field_', '');
        updateData = {
          custom_field_values: { ...(item.custom_field_values || {}), [fieldId]: value }
        };
      } else {
        updateData = { [field]: value };
      }
      const updatedItem = await api.items.update(item.id, updateData);
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
