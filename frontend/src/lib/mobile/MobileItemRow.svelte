<script>
  import { navigate } from '../router.js';
  import DueMark from '../widgets/dashboard/DueMark.svelte';
  import StatusPill from '../components/StatusPill.svelte';
  import { t } from '../stores/i18n.svelte.js';

  /**
   * Touch-sized work-item row for the mobile lists. Derived from
   * features/items/WorkItemRow.svelte but trimmed to phone-useful fields and
   * a single full-width tap target that opens the mobile detail.
   *
   * @type {{
   *   itemId: number,
   *   itemKey?: string | null,
   *   title?: string,
   *   statusName?: string | null,
   *   statusColor?: string | null,
   *   priorityName?: string | null,
   *   priorityColor?: string | null,
   *   dueDate?: Date | null,
   *   timestamp?: string | null,
   *   unread?: boolean,
   * }}
   */
  let {
    itemId,
    itemKey = null,
    title = '',
    statusName = null,
    statusColor = null,
    priorityName = null,
    priorityColor = null,
    dueDate = null,
    timestamp = null,
    unread = false,
  } = $props();
</script>

<button
  id={`mobile-item-row-${itemId}`}
  class="row"
  data-testid="mobile-item-row"
  onclick={() => navigate(`/m/items/${itemId}`)}
  type="button"
>
  <div class="main">
    {#if priorityColor}
      <span class="prio" style={`background-color: ${priorityColor};`} title={priorityName} aria-label={priorityName ? t('mobile.item.priorityLabel', { name: priorityName }) : undefined}></span>
    {/if}
    <span class="title">{title}</span>
    {#if unread}
      <span class="unread" data-testid="mobile-item-unread" aria-label={t('mobile.common.unread')}></span>
    {/if}
  </div>
  <div class="meta">
    {#if itemKey}
      <span class="key">{itemKey}</span>
    {/if}
    <StatusPill name={statusName} color={statusColor} />
    {#if timestamp}
      <span class="ts">{timestamp}</span>
    {/if}
    <DueMark {dueDate} />
  </div>
</button>

<style>
  .row {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 0.75rem;
    text-align: left;
    background-color: var(--ds-surface);
    border: none;
    border-bottom: 1px solid var(--ds-border);
    cursor: pointer;
    min-height: 56px;
  }

  .row:active {
    background-color: var(--ds-background-neutral-hovered);
  }

  .main {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
  }

  .prio {
    width: 8px;
    height: 8px;
    border-radius: var(--radius-full, 9999px);
    flex-shrink: 0;
  }

  .title {
    flex: 1 1 auto;
    min-width: 0;
    font-size: 0.9375rem;
    line-height: 1.3;
    color: var(--ds-text);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .unread {
    flex-shrink: 0;
    width: 9px;
    height: 9px;
    border-radius: var(--radius-full, 9999px);
    background-color: var(--ds-interactive);
  }

  .meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.5rem;
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }

  .key {
    font-family: var(--font-mono, ui-monospace, monospace);
  }

</style>
