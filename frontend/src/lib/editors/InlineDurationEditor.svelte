<script>
  import Clock from '@lucide/svelte/icons/clock';
  import { t } from '../stores/i18n.svelte.js';
  import { durationToString, parseDuration } from '../utils/timeUtils.js';
  import EditableField from './EditableField.svelte';
  import Input from '../components/Input.svelte';
  import InlineEditorActions from './InlineEditorActions.svelte';
  import InlineEditorError from './InlineEditorError.svelte';

  let {
    value = null, placeholder = '', disabled = false, required = false,
    className = '', editingClass = 'editing-input',
    displayClass = 'display-text cursor-text', enableSingleClick = false,
    enableDoubleClick = false, onsave = null, onclick: onclickProp = null
  } = $props();

  const displayValue = $derived(
    value != null && value > 0 ? durationToString(value, { withDays: true }) : ''
  );

  function toDraft(stored) {
    return stored != null && stored > 0 ? durationToString(stored, { withDays: true }) : '';
  }

  // An empty input clears the estimate; a non-positive or unparseable string is
  // rejected so a typo never silently becomes zero.
  function handleCommit(draft) {
    const raw = String(draft ?? '').trim();
    if (raw === '') {
      if (required && value == null) throw new Error(t('validation.required'));
      if ((value ?? null) === null) return;
      return onsave?.({ value: null });
    }
    const minutes = parseDuration(raw);
    if (!Number.isFinite(minutes) || minutes <= 0) throw new Error(t('validation.pattern'));
    const rounded = Math.round(minutes);
    if (rounded === (value ?? null)) return;
    return onsave?.({ value: rounded });
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
      class="flex items-center gap-2 text-left w-full px-2 py-1 text-sm rounded whitespace-nowrap transition-colors {displayClass} {className}"
      style={!displayValue ? 'color: var(--ds-text-subtle);' : ''}
      data-testid="inline-duration-display"
    >
      <Clock class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
      <span data-testid="inline-duration-value">{displayValue || placeholder}</span>
    </span>
  {/snippet}
  {#snippet editor({ draft, setDraft, error, saving, commit, cancel, markAction })}
    <div class="inline-flex items-center gap-1 w-full">
      <div class="flex-1 relative">
        <Input
          {placeholder}
          dataTestid="inline-duration-input"
          value={draft ?? ''}
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
</style>
