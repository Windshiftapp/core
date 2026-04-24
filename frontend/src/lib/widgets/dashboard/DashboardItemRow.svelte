<script>
  import { formatDueDate, getDueBadgeClass } from '../../utils/dateFormatter.js';

  let {
    title,
    itemKey,
    statusName = null,
    statusColor = null,
    priorityName = null,
    priorityColor = null,
    dueDate = null,
    timestamp = null,
    onclick,
  } = $props();

  const hasPriority = $derived(!!(priorityName && priorityColor));
  const hasStatus = $derived(!!statusName);
</script>

<button
  class="w-full flex items-center justify-between gap-3 p-2 rounded border text-left transition-colors"
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
        title={priorityName}
        aria-label={`Priority: ${priorityName}`}
      ></span>
    {/if}
    <span class="text-sm truncate" style="color: var(--ds-text);">{title}</span>
  </div>

  <div class="flex items-center gap-2 flex-shrink-0 text-[0.7rem]" style="color: var(--ds-text-subtle);">
    <span class="font-mono">{itemKey}</span>
    {#if hasStatus}
      <span
        class="inline-flex items-center rounded px-1.5 py-[1px] font-medium"
        style={statusColor
          ? `background-color: ${statusColor}1f; color: ${statusColor};`
          : 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);'}
      >
        {statusName}
      </span>
    {/if}
    {#if timestamp}
      <span>{timestamp}</span>
    {/if}
    {#if dueDate}
      <span
        class={`inline-flex items-center rounded-full px-2 py-0.5 text-[0.65rem] font-semibold ${getDueBadgeClass(dueDate)}`}
      >
        {formatDueDate(dueDate)}
      </span>
    {/if}
  </div>
</button>
