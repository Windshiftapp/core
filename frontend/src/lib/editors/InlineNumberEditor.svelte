<script>
  import { tick } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import BaseInlineEditor from './BaseInlineEditor.svelte';
  import Input from '../components/Input.svelte';

  let {
    value = null, placeholder = '', disabled = false, required = false,
    min = 0, step = 'any', className = '', editingClass = 'editing-input',
    displayClass = 'display-text cursor-text', enableSingleClick = false,
    enableDoubleClick = false, onsave = null, onclick: onclickProp = null
  } = $props();

  let baseEditor;
  let editValue = $state('');
  let inputElement = $state(null);

  function handleStartEdit() {
    editValue = value == null ? '' : String(value);
    tick().then(() => {
      inputElement?.focus();
      inputElement?.select();
    });
  }

  // An empty input clears the value; non-numeric text is rejected rather than
  // silently coerced to NaN.
  function parseValue(raw) {
    const trimmed = String(raw ?? '').trim();
    if (trimmed === '') return null;
    const parsed = Number(trimmed);
    return Number.isFinite(parsed) ? parsed : Number.NaN;
  }

  function handleSave() {
    const parsed = parseValue(editValue);
    if (parsed !== null && Number.isNaN(parsed)) {
      baseEditor.setError(t('validation.pattern'));
      return;
    }
    if (parsed !== null && parsed < min) {
      baseEditor.setError(t('validation.minValue', { min }));
      return;
    }
    if (required && parsed === null) {
      baseEditor.setError(t('validation.required'));
      return;
    }
    if (parsed === (value ?? null)) {
      baseEditor.cancelEditing();
      return;
    }
    baseEditor.setSaving(true);
    onsave?.({ value: parsed });
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
      type="number"
      {min}
      {step}
      dataTestid="inline-number-input"
      class="w-full px-2 py-1 text-sm border rounded tabular-nums {editingClass} {className} {error ? 'border-ds-border-danger' : ''}"
      disabled={saving}
      {onkeydown}
      {onblur}
      style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border-focused);"
    />
  {/snippet}
  {#snippet displayContent()}
    <span
      class="block text-left w-full px-2 py-1 text-sm rounded tabular-nums transition-colors {displayClass} {className}"
      class:placeholder-text={value == null}
      style="color: var(--ds-text);"
      data-testid="inline-number-display"
    >
      <span data-testid="inline-number-value">{value == null ? placeholder : value}</span>
    </span>
  {/snippet}
</BaseInlineEditor>

<style>
  .display-text:hover {
    background-color: var(--ds-background-neutral-hovered);
  }

  .editing-input {
    border-color: var(--ds-border-focused);
    box-shadow: 0 0 0 1px var(--ds-border-focused);
  }

  .placeholder-text {
    color: var(--ds-text-subtlest) !important;
  }
</style>
