<script>
  import { tick } from 'svelte';
  import Clock from '@lucide/svelte/icons/clock';
  import { t } from '../stores/i18n.svelte.js';
  import { durationToString, parseDuration } from '../utils/timeUtils.js';
  import BaseInlineEditor from './BaseInlineEditor.svelte';
  import Input from '../components/Input.svelte';

  let {
    value = null, placeholder = '', disabled = false, required = false,
    className = '', editingClass = 'editing-input',
    displayClass = 'display-text cursor-text', enableSingleClick = false,
    enableDoubleClick = false, onsave = null, onclick: onclickProp = null
  } = $props();

  const displayValue = $derived(
    value != null && value > 0 ? durationToString(value, { withDays: true }) : ''
  );

  let baseEditor;
  let editValue = $state('');
  let inputElement = $state(null);

  function handleStartEdit() {
    editValue = value != null && value > 0 ? durationToString(value, { withDays: true }) : '';
    tick().then(() => {
      inputElement?.focus();
      inputElement?.select();
    });
  }

  // An empty input clears the estimate; a non-positive or unparseable string is
  // rejected so a typo never silently becomes zero.
  function handleSave() {
    const raw = String(editValue ?? '').trim();
    if (raw === '') {
      if (required && value == null) {
        baseEditor.setError(t('validation.required'));
        return;
      }
      if ((value ?? null) === null) {
        baseEditor.cancelEditing();
        return;
      }
      baseEditor.setSaving(true);
      onsave?.({ value: null });
      return;
    }
    const minutes = parseDuration(raw);
    if (!Number.isFinite(minutes) || minutes <= 0) {
      baseEditor.setError(t('validation.pattern'));
      return;
    }
    const rounded = Math.round(minutes);
    if (rounded === (value ?? null)) {
      baseEditor.cancelEditing();
      return;
    }
    baseEditor.setSaving(true);
    onsave?.({ value: rounded });
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
      {placeholder}
      dataTestid="inline-duration-input"
      class="w-full px-2 py-1 text-sm border rounded {editingClass} {className} {error ? 'border-ds-border-danger' : ''}"
      disabled={saving}
      {onkeydown}
      {onblur}
      style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border-focused);"
    />
  {/snippet}
  {#snippet displayContent()}
    <span
      class="flex items-center gap-2 text-left w-full px-2 py-1 text-sm rounded whitespace-nowrap transition-colors {displayClass} {className}"
      style={!displayValue ? 'color: var(--ds-text-subtle);' : ''}
      data-testid="inline-duration-display"
    >
      <Clock class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
      <span data-testid="inline-duration-value">{displayValue || placeholder}</span>
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
</style>
