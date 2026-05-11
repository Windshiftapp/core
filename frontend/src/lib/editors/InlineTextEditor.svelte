<script>
  import { tick } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import BaseInlineEditor from './BaseInlineEditor.svelte';

  let {
    value = '', placeholder = '', disabled = false, required = false,
    maxLength = 255, className = '', editingClass = 'editing-input',
    displayClass = 'display-text cursor-text', enableSingleClick = false,
    enableDoubleClick = false, onsave = null, onclick: onclickProp = null
  } = $props();

  const effectivePlaceholder = $derived(placeholder || t('editors.enterText'));

  let baseEditor;
  let editValue = $state('');
  let inputElement = $state(null);

  function handleStartEdit() {
    editValue = value || '';
    tick().then(() => {
      inputElement?.focus();
      inputElement?.select();
    });
  }

  function handleSave() {
    const trimmedValue = editValue.trim();
    if (required && !trimmedValue) {
      baseEditor.setError(t('validation.required'));
      return;
    }
    if (trimmedValue.length > maxLength) {
      baseEditor.setError(t('validation.maxLength', { max: maxLength }));
      return;
    }
    if (trimmedValue === (value || '')) {
      baseEditor.cancelEditing();
      return;
    }
    baseEditor.setSaving(true);
    onsave?.({ value: trimmedValue });
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
    <input
      bind:this={inputElement}
      bind:value={editValue}
      placeholder={effectivePlaceholder}
      maxlength={maxLength}
      class="w-full px-2 py-1 text-sm border rounded {editingClass} {className}"
      class:border-red-500={error}
      disabled={saving}
      {onkeydown}
      {onblur}
      style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border-focused);"
    />
  {/snippet}
  {#snippet displayContent()}
    <span
      class="block text-left w-full px-2 py-1 text-sm rounded transition-colors {displayClass} {className}"
      class:placeholder-text={!value}
      style="color: var(--ds-text);"
    >
      {value || effectivePlaceholder}
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
