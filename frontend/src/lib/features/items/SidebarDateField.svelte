<script>
  import { Calendar } from '@lucide/svelte';
  import Text from '../../components/Text.svelte';
  import { clickOutside } from '../../actions/clickOutside.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateOnly } from '../../utils/dateFormatter.js';

  let {
    fieldKey,
    label,
    value = null,
    editing = false,
    editable = false,
    onSave = () => {},
    onStartEdit = () => {},
    onCancel = () => {},
  } = $props();

  function focusAndShowPicker(node) {
    node.focus();
    setTimeout(() => {
      try {
        node.showPicker();
      } catch {
        // The browser may not support programmatic picker opening.
      }
    }, 0);
  }

  let testId = $derived(`sidebar-date-${fieldKey.replaceAll('_', '-')}`);
</script>

<div class="mb-3">
  {#if editing}
    <div class="w-full py-1.5" use:clickOutside onclickOutside={onCancel}>
      <input
        data-testid={`${testId}-input`}
        type="date"
        value={value ? value.split('T')[0] : ''}
        onchange={(event) => onSave(event.currentTarget.value || null)}
        class="w-full px-2 py-1 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
        style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
        use:focusAndShowPicker
      />
    </div>
  {:else}
    <button
      data-testid={testId}
      onclick={() => editable && onStartEdit()}
      class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
      onmouseenter={(event) =>
        (event.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)')}
      onmouseleave={(event) => (event.currentTarget.style.backgroundColor = '')}
      disabled={!editable}
    >
      <Text variant="subtle" size="sm">{label}</Text>
      <div class="flex items-center gap-2">
        {#if value}
          <Calendar size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
          <span style="color: var(--ds-text);">{formatDateOnly(value)}</span>
        {:else}
          <Text variant="subtle" size="sm">{t('common.none')}</Text>
        {/if}
      </div>
    </button>
  {/if}
</div>
