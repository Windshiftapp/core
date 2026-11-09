<script>
  import { t } from '../stores/i18n.svelte.js';
  import EditableField from './EditableField.svelte';
  import Input from '../components/Input.svelte';
  import InlineEditorActions from './InlineEditorActions.svelte';
  import InlineEditorError from './InlineEditorError.svelte';

  let {
    value = '', placeholder = '', disabled = false, required = false,
    maxLength = 255, className = '', editingClass = 'editing-input',
    displayClass = 'display-text cursor-text', maxLines = null,
    enableSingleClick = false, enableDoubleClick = false,
    onsave = null, onclick: onclickProp = null
  } = $props();

  const effectivePlaceholder = $derived(placeholder || t('editors.enterText'));

  function handleCommit(draft) {
    const trimmed = String(draft ?? '').trim();
    if (required && !trimmed) throw new Error(t('validation.required'));
    if (trimmed.length > maxLength) throw new Error(t('validation.maxLength', { max: maxLength }));
    if (trimmed === (value || '')) return;
    return onsave?.({ value: trimmed });
  }
</script>

<EditableField
  {value}
  editable={!disabled}
  oncommit={handleCommit}
  {enableSingleClick}
  {enableDoubleClick}
  onclick={onclickProp}
>
  {#snippet display()}
    <span
      class="block text-left w-full px-2 py-1 text-sm rounded transition-colors {displayClass} {className}"
      class:placeholder-text={!value}
      style="color: var(--ds-text);{maxLines ? ` display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: ${maxLines}; line-clamp: ${maxLines}; overflow: hidden;` : ''}"
    >
      {value || effectivePlaceholder}
    </span>
  {/snippet}
  {#snippet editor({ draft, setDraft, error, saving, commit, cancel, markAction })}
    <div class="inline-flex items-center gap-1 w-full">
      <div class="flex-1 relative">
        <Input
          value={draft ?? ''}
          placeholder={effectivePlaceholder}
          maxlength={maxLength}
          class="w-full px-2 py-1 text-sm border rounded {editingClass} {className} {error ? 'border-ds-border-danger' : ''}"
          style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border-focused);"
          oninput={(e) => setDraft(e.currentTarget.value)}
          autofocus
        />
        <InlineEditorError {error} />
      </div>
      <div class="flex items-center gap-1">
        <InlineEditorActions
          {saving}
          onsave={() => markAction(commit)}
          oncancel={() => markAction(cancel)}
        />
      </div>
    </div>
  {/snippet}
</EditableField>

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
