<script>
  import DueMark from './DueMark.svelte';
  import Link from '../../components/Link.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { objectDisplayName } from '../../utils/systemLabels.js';

  let {
    title,
    itemKey = null,
    href = null,
    unread = false,
    statusName = null,
    statusBuiltinKey = null,
    statusColor = null,
    priorityName = null,
    priorityBuiltinKey = null,
    priorityColor = null,
    dueDate = null,
    timestamp = null,
    density = 'comfortable',
    dataTestid = 'dashboard-item-row',
    onclick,
  } = $props();

  const hasPriority = $derived(!!(priorityName && priorityColor));
  const hasStatus = $derived(!!statusName);
  const localizedPriorityName = $derived(objectDisplayName({ name: priorityName, builtin_key: priorityBuiltinKey }, 'priority'));
  const localizedStatusName = $derived(objectDisplayName({ name: statusName, builtin_key: statusBuiltinKey }, 'status'));
  const rowPadding = $derived(density === 'compact' ? 'p-1.5' : 'p-2');
  const rowClass = $derived(
    `dashboard-item-row w-full flex items-center justify-between gap-3 ${rowPadding} rounded border text-left transition-colors`
  );

  // Link only invokes onClick for an unmodified left click, so this is the
  // one place that must suppress the anchor's own navigation.
  function handleActivate(event) {
    event?.preventDefault?.();
    onclick?.(event);
  }
</script>

{#snippet rowContent()}
  <div class="flex items-center gap-1.5 min-w-0 flex-1">
    {#if hasPriority}
      <span
        class="inline-block w-2 h-2 rounded-full flex-shrink-0"
        style={`background-color: ${priorityColor};`}
        title={localizedPriorityName}
        aria-label={t('dashboard.states.priorityLabel', { priority: localizedPriorityName })}
      ></span>
    {/if}
    <span
      class="text-sm truncate"
      style={unread ? 'color: var(--ds-text); font-weight: 500;' : 'color: var(--ds-text);'}
      >{title}</span
    >
  </div>

  <div class="dashboard-item-meta flex items-center gap-2 flex-shrink-0 text-[0.7rem]" style="color: var(--ds-text-subtle);">
    {#if itemKey}
      <span class="dashboard-item-key font-mono" data-testid="dashboard-item-key">{itemKey}</span>
    {/if}
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
{/snippet}

{#if href}
  <!-- A real href keeps cmd/ctrl/shift/middle-click opening a new tab; Link
       only diverts an unmodified left click to the onclick handler. -->
  <Link
    {href}
    onClick={handleActivate}
    data-testid={dataTestid}
    data-item-key={itemKey ?? undefined}
    data-density={density}
    class={rowClass}
    style="border-color: var(--ds-border);"
  >
    {@render rowContent()}
  </Link>
{:else}
  <button
    data-testid={dataTestid}
    data-item-key={itemKey ?? undefined}
    data-density={density}
    class={rowClass}
    style="border-color: var(--ds-border);"
    onclick={handleActivate}
  >
    {@render rowContent()}
  </button>
{/if}

<style>
  /* The row root is rendered by the Link component, so scope it globally. */
  :global(.dashboard-item-row) {
    background-color: var(--ds-surface);
  }

  :global(.dashboard-item-row):hover {
    background-color: var(--ds-background-neutral-hovered);
  }

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
