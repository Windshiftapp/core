<script>
  import Calendar from '@lucide/svelte/icons/calendar';
  import { t } from '../stores/i18n.svelte.js';
  import { dateOnlyKey, formatDateOnly } from '../utils/dateFormatter.js';
  import EditableField from './EditableField.svelte';
  import Input from '../components/Input.svelte';
  import InlineEditorActions from './InlineEditorActions.svelte';
  import InlineEditorError from './InlineEditorError.svelte';

  let {
    value = '', placeholder = '', disabled = false, required = false,
    className = '', editingClass = 'border-ds-interactive ring-1 ring-ds-border-focused',
    displayClass = 'hover-bg cursor-text', enableSingleClick = false,
    enableDoubleClick = false, onsave = null, onclick: onclickProp = null
  } = $props();

  const effectivePlaceholder = $derived(placeholder || t('editors.selectDate'));
  const displayValue = $derived(value ? (formatDateOnly(value) || value) : '');

  function toDraft(stored) {
    return stored ? dateOnlyKey(stored) : '';
  }

  function handleCommit(draft) {
    const next = String(draft ?? '');
    if (required && !next) throw new Error(t('validation.required'));
    if (next === toDraft(value)) return;
    return onsave?.({ value: next });
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
      style={!value ? 'color: var(--ds-text-subtle);' : ''}
      data-testid="inline-date-display"
    >
      <Calendar class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
      <span data-testid="inline-date-value">{displayValue || effectivePlaceholder}</span>
    </span>
  {/snippet}
  {#snippet editor({ draft, setDraft, error, saving, commit, cancel, markAction })}
    <div class="inline-flex items-center gap-1 w-full">
      <div class="flex-1 relative">
        <Input
          type="date"
          value={draft ?? ''}
          class="w-full px-2 py-1 text-sm border rounded {editingClass} {className} {error ? 'border-ds-border-danger' : ''}"
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
  .hover-bg:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
