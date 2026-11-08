<script>
  import Calendar from '@lucide/svelte/icons/calendar';
  import Text from '../../components/Text.svelte';
  import DatePicker from '../../pickers/DatePicker.svelte';
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

  let testId = $derived(`sidebar-date-${fieldKey.replaceAll('_', '-')}`);
</script>

<div class="mb-1">
  {#if editing}
    <div class="w-full py-1.5" use:clickOutside onclickOutside={onCancel}>
      <DatePicker
        inline
        autoOpen
        value={value ? value.split('T')[0] : ''}
        onchange={(next) => onSave(next || null)}
        dataTestid={`${testId}-input`}
      />
    </div>
  {:else}
    <button
      data-testid={testId}
      onclick={() => editable && onStartEdit()}
      class="hover-bg w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
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
