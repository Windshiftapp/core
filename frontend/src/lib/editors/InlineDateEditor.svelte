<script>
  import { tick } from 'svelte';
  import { Calendar } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { dateOnlyKey, formatDateOnly } from '../utils/dateFormatter.js';
  import BaseInlineEditor from './BaseInlineEditor.svelte';
  import Input from '../components/Input.svelte';

  let {
    value = '', placeholder = '', disabled = false, required = false,
    className = '', editingClass = 'border-ds-interactive ring-1 ring-ds-border-focused',
    displayClass = 'hover-bg cursor-text', enableSingleClick = false,
    enableDoubleClick = false, onsave = null, onclick: onclickProp = null
  } = $props();

  const effectivePlaceholder = $derived(placeholder || t('editors.selectDate'));
  const displayValue = $derived(value ? (formatDateOnly(value) || value) : '');

  let baseEditor;
  let editValue = $state('');
  let inputElement = $state(null);

  function formatInputDate(dateStr) {
    if (!dateStr) return '';
    return dateOnlyKey(dateStr);
  }

  function handleStartEdit() {
    editValue = formatInputDate(value);
    tick().then(() => inputElement?.focus());
  }

  function handleSave() {
    if (required && !editValue) {
      baseEditor.setError(t('validation.required'));
      return;
    }
    if (editValue === formatInputDate(value)) {
      baseEditor.cancelEditing();
      return;
    }
    baseEditor.setSaving(true);
    onsave?.({ value: editValue });
  }

  export function confirmSave(newValue) {
    value = newValue;
    editValue = '';
    baseEditor.confirmSave();
  }

  export function rejectSave(errorMessage) {
    baseEditor.rejectSave(errorMessage);
  }
</script>

<BaseInlineEditor
  bind:this={baseEditor}
  {disabled}
  {enableSingleClick}
  {enableDoubleClick}
  onclick={onclickProp}
  onsave={handleSave}
  onstartedit={handleStartEdit}
>
  {#snippet editingInput({ saving, error, onkeydown, onblur })}
    <Input
      bind:inputRef={inputElement}
      bind:value={editValue}
      type="date"
      class="w-full px-2 py-1 text-sm border rounded {editingClass} {className} {error ? 'border-ds-border-danger' : ''}"
      disabled={saving}
      {onkeydown}
      {onblur}
    />
  {/snippet}
  {#snippet displayContent()}
    <span
      class="block text-left w-full px-2 py-1 text-sm rounded transition-colors flex items-center gap-2 {displayClass} {className}"
      style={!value ? 'color: var(--ds-text-subtle);' : ''}
    >
      <Calendar class="w-4 h-4" style="color: var(--ds-text-subtle);" />
      {displayValue || effectivePlaceholder}
    </span>
  {/snippet}
</BaseInlineEditor>

<style>
  .hover-bg:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
