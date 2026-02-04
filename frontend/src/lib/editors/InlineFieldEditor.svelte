<script>
  import { itemUpdateService } from '../services/itemUpdateService.js';
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

  // Get current field value
  const fieldValue = $derived(getFieldValue(item, field));

  function getFieldValue(item, field) {
    if (!item) return null;

    switch (field) {
      case 'title':
        return item.title || '';
      case 'status':
        return item.status || null;
      case 'priority':
        return item.priority || null;
      case 'assignee':
        return item.assignee_id || null;
      case 'milestone':
        return item.milestone_id || null;
      case 'description':
        return item.description || '';
      default:
        if (field.startsWith('custom_field_')) {
          const fieldId = field.replace('custom_field_', '');
          return item.custom_field_values?.[fieldId] || null;
        }
        return item[field] || null;
    }
  }

  async function handleSave(detail) {
    const { value } = detail;

    try {
      // Use ItemUpdateService to update the field
      const updatedItem = await itemUpdateService.updateField(
        item,
        field,
        value,
        (updatedItem, field, value) => {
          // Success callback
          if (editorComponent?.confirmSave) {
            editorComponent.confirmSave(value);
          }

          // Call update callback to parent
          onitemUpdated?.({
            item: updatedItem,
            field,
            value
          });
        },
        (error, field, value) => {
          // Error callback
          const errorMessage = error.message || 'Failed to save changes';

          if (editorComponent?.rejectSave) {
            editorComponent.rejectSave(errorMessage);
          }

          // Call error callback
          onupdateError?.({
            error: errorMessage,
            field,
            value
          });
        }
      );

    } catch (error) {
      console.error('Update failed:', error);

      if (editorComponent?.rejectSave) {
        editorComponent.rejectSave(error.message || 'Failed to save changes');
      }
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
