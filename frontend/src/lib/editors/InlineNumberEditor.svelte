<script>
  import { t } from '../stores/i18n.svelte.js';
  import EditableField from './EditableField.svelte';
  import Input from '../components/Input.svelte';
  import InlineEditorActions from './InlineEditorActions.svelte';
  import InlineEditorError from './InlineEditorError.svelte';

  let {
    value = null, placeholder = '', disabled = false, required = false,
    min = 0, step = 'any', className = '', editingClass = 'editing-input',
    displayClass = 'display-text cursor-text', enableSingleClick = false,
    enableDoubleClick = false, onsave = null, onclick: onclickProp = null
  } = $props();

  // An empty input clears the value; non-numeric text is rejected rather than
  // silently coerced to NaN.
  function parseValue(raw) {
    const trimmed = String(raw ?? '').trim();
    if (trimmed === '') return null;
    const parsed = Number(trimmed);
    return Number.isFinite(parsed) ? parsed : Number.NaN;
  }

  function toDraft(stored) {
    return stored == null ? '' : String(stored);
  }

  function handleCommit(draft) {
    const parsed = parseValue(draft);
    if (parsed !== null && Number.isNaN(parsed)) throw new Error(t('validation.pattern'));
    if (parsed !== null && parsed < min) throw new Error(t('validation.minValue', { min }));
    if (required && parsed === null) throw new Error(t('validation.required'));
    if (parsed === (value ?? null)) return;
    return onsave?.({ value: parsed });
  }
</script>

<EditableField
  {value}
  editable={!disabled}
  oncommit={handleCommit}
  {toDraft}
  {enableSingleClick}
  {enableDoubleClick}
  onclick={onclickProp}
>
  {#snippet display()}
    <span
      class="block text-left w-full px-2 py-1 text-sm rounded tabular-nums transition-colors {displayClass} {className}"
      class:placeholder-text={value == null}
      style="color: var(--ds-text);"
      data-testid="inline-number-display"
    >
      <span data-testid="inline-number-value">{value == null ? placeholder : value}</span>
    </span>
  {/snippet}
  {#snippet editor({ draft, setDraft, error, saving, commit, cancel, markAction })}
    <div class="inline-flex items-center gap-1 w-full">
      <div class="flex-1 relative">
        <Input
          type="number"
          {min}
          {step}
          dataTestid="inline-number-input"
          value={draft ?? ''}
          class="w-full px-2 py-1 text-sm border rounded tabular-nums {editingClass} {className} {error ? 'border-ds-border-danger' : ''}"
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
