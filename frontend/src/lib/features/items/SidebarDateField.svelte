<script>
  import Calendar from '@lucide/svelte/icons/calendar';
  import Text from '../../components/Text.svelte';
  import DatePicker from '../../pickers/DatePicker.svelte';
  import { clickOutside } from '../../actions/clickOutside.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateOnly } from '../../utils/dateFormatter.js';
  import EditableField from '../../editors/EditableField.svelte';

  let {
    fieldKey,
    label,
    value = null,
    editable = false,
    onSave = () => {},
    onStartEdit = () => {},
    onCancel = () => {},
  } = $props();

  let testId = $derived(`sidebar-date-${fieldKey.replaceAll('_', '-')}`);
</script>

<div class="mb-1">
  <EditableField
    {value}
    {editable}
    onstartedit={onStartEdit}
    oncommit={(draft) => onSave(draft || null)}
    displayTestid={testId}
  >
    {#snippet display()}
      <div class="hover-bg w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group">
        <Text variant="subtle" size="sm">{label}</Text>
        <div class="flex items-center gap-2">
          {#if value}
            <Calendar size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
            <span style="color: var(--ds-text);">{formatDateOnly(value)}</span>
          {:else}
            <Text variant="subtle" size="sm">{t('common.none')}</Text>
          {/if}
        </div>
      </div>
    {/snippet}
    {#snippet editor({ draft, setDraft, commit, cancel })}
      <div
        class="w-full py-1.5"
        use:clickOutside
        onclickOutside={() => {
          onCancel?.();
          cancel();
        }}
      >
        <DatePicker
          inline
          autoOpen
          value={draft ? String(draft).split('T')[0] : ''}
          onchange={(next) => {
            setDraft(next || null);
            commit();
          }}
          dataTestid={`${testId}-input`}
        />
      </div>
    {/snippet}
  </EditableField>
</div>
