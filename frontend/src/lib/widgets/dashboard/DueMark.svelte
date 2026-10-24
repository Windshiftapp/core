<script>
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import Clock from '@lucide/svelte/icons/clock';
  import Badge from '../../components/Badge.svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import {
    formatDueCompact,
    formatDueTooltip,
    getDueSeverity,
  } from '../../utils/dateFormatter.js';

  let {
    dueDate = null,
    iconOnly = false,
  } = $props();

  const severity = $derived(getDueSeverity(dueDate));
  const variant = $derived(
    severity === 'overdue' ? 'danger' : severity === 'soon' ? 'warning' : 'neutral'
  );
  const icon = $derived(severity === 'overdue' ? AlertTriangle : Clock);
  const tooltip = $derived(dueDate ? formatDueTooltip(dueDate) : '');
  const compact = $derived(dueDate ? formatDueCompact(dueDate) : '');
</script>

{#if dueDate && severity}
  <Tooltip content={tooltip} placement="top">
    <Badge
      {variant}
      size="xs"
      {icon}
      class="due-mark tabular-nums"
      data-testid="due-mark"
      data-severity={severity}
      aria-label={tooltip}
    >
      {#if !iconOnly}
        <span class="due-mark-text">{compact}</span>
      {/if}
    </Badge>
  </Tooltip>
{/if}

<style>
  /* When the widget body is very narrow, drop the day count and keep only the
     icon. The widget body (WidgetWrapper .widget-content) defines the container. */
  @container widget (max-width: 300px) {
    .due-mark-text {
      display: none;
    }
  }
</style>
