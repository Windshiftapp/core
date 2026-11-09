<script>
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import { t } from '../stores/i18n.svelte.js';
  import Select from '../components/Select.svelte';
  import EditableField from './EditableField.svelte';
  import InlineEditorActions from './InlineEditorActions.svelte';
  import InlineEditorError from './InlineEditorError.svelte';

  let {
    value = null, options = [], placeholder = '', disabled = false,
    required = false, className = '', displayClass = 'hover-bg cursor-pointer',
    allowClear = false, onsave = null
  } = $props();

  const effectivePlaceholder = $derived(placeholder || t('common.select') + '...');

  // Build options for the Select component, including placeholder if needed
  const selectOptions = $derived.by(() => {
    const opts = [];
    if (allowClear || !required) {
      opts.push({ value: null, label: effectivePlaceholder });
    }
    opts.push(...options);
    return opts;
  });

  const currentOption = $derived(options.find((opt) => opt.value === value));
  const displayText = $derived(currentOption?.label || effectivePlaceholder);
  const displayColor = $derived(currentOption?.color);

  function handleCommit(draft) {
    if (required && (draft === null || draft === undefined || draft === '')) {
      throw new Error(t('validation.required'));
    }
    if (draft === value) return;
    return onsave?.({ value: draft });
  }
</script>

<EditableField {value} editable={!disabled} oncommit={handleCommit}>
  {#snippet display()}
    <span
      class="inline-flex items-center gap-2 px-2 py-1 text-sm rounded transition-colors {displayClass} {className}"
      style={!currentOption ? 'color: var(--ds-text-subtle);' : ''}
    >
      <span class="flex items-center gap-2">
        {#if currentOption && displayColor}
          <span class="w-2 h-2 rounded-full" style="background-color: {displayColor};"></span>
        {/if}
        {displayText}
      </span>
      <ChevronDown class="w-3 h-3" style="color: var(--ds-text-subtle);" />
    </span>
  {/snippet}
  {#snippet editor({ draft, setDraft, error, saving, commit, cancel, markAction })}
    <div class="inline-flex items-center gap-1 w-full">
      <div class="flex-1 relative">
        <Select
          value={draft}
          options={selectOptions}
          {disabled}
          size="small"
          onchange={(val) => {
            setDraft(val);
            commit();
          }}
          class={className}
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
