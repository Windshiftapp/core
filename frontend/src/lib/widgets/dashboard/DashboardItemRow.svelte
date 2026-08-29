<script>
  import DueMark from './DueMark.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { systemPriorityName, systemStatusName } from '../../utils/systemLabels.js';

  let {
    title,
    itemKey,
    statusName = null,
    statusBuiltinKey = null,
    statusColor = null,
    priorityName = null,
    priorityBuiltinKey = null,
    priorityColor = null,
    dueDate = null,
    timestamp = null,
    density = 'comfortable',
    onclick,
  } = $props();

  const hasPriority = $derived(!!(priorityName && priorityColor));
  const hasStatus = $derived(!!statusName);
  const localizedPriorityName = $derived(systemPriorityName({ name: priorityName, builtin_key: priorityBuiltinKey }));
  const localizedStatusName = $derived(systemStatusName({ name: statusName, builtin_key: statusBuiltinKey }));
  const rowPadding = $derived(density === 'compact' ? 'p-1.5' : 'p-2');
</script>

<button
  data-testid="dashboard-item-row"
  data-item-key={itemKey}
  data-density={density}
  class="dashboard-item-row w-full flex items-center justify-between gap-3 {rowPadding} rounded border text-left transition-colors"
  style="border-color: var(--ds-border); background-color: var(--ds-surface);"
  onmouseenter={(e) => (e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)')}
  onmouseleave={(e) => (e.currentTarget.style.backgroundColor = 'var(--ds-surface)')}
  {onclick}
>
  <div class="flex items-center gap-1.5 min-w-0 flex-1">
    {#if hasPriority}
      <span
        class="inline-block w-2 h-2 rounded-full flex-shrink-0"
        style={`background-color: ${priorityColor};`}
        title={localizedPriorityName}
        aria-label={t('dashboard.widgetContent.priorityAria', { name: localizedPriorityName })}
      ></span>
    {/if}
    <span class="text-sm truncate" style="color: var(--ds-text);">{title}</span>
  </div>

  <div class="dashboard-item-meta flex items-center gap-2 flex-shrink-0 text-[0.7rem]" style="color: var(--ds-text-subtle);">
    <span class="dashboard-item-key font-mono" data-testid="dashboard-item-key">{itemKey}</span>
    {#if hasStatus}
      <span
        class="dashboard-item-status inline-flex items-center rounded px-1.5 py-[1px] font-medium"
        style={statusColor
          ? `background-color: ${statusColor}1f; color: ${statusColor};`
          : 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);'}
      >
        {localizedStatusName}
      </span>
    {/if}
    {#if timestamp}
      <span class="dashboard-item-timestamp">{timestamp}</span>
    {/if}
    <DueMark {dueDate} />
  </div>
</button>

<style>
  /* Container-query drop order: the widget body (WidgetWrapper .widget-content)
     establishes the `widget` container. Less valuable columns drop first so the
     title is the last thing to truncate. */
  @container widget (max-width: 400px) {
    .dashboard-item-status {
      display: none;
    }
  }

  @container widget (max-width: 300px) {
    .dashboard-item-key {
      display: none;
    }
  }
</style>
