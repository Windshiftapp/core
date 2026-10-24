<script>
  import { useEventListener } from 'runed';
  import { sla } from '../../api/sla.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import Gauge from '@lucide/svelte/icons/gauge';
  import Clock from '@lucide/svelte/icons/clock';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatInstant } from '../../utils/dateFormatter.js';

  let { itemId = null } = $props();

  // Server-computed remaining/breach state has to be re-read while a timer is
  // visible; elapsed time and business-hours transitions never reach the client
  // otherwise.
  const SLA_REFRESH_MS = 30000;

  let states = $state([]);
  let loading = $state(false);
  let requestToken = 0;

  async function load() {
    if (!itemId) return;
    const token = ++requestToken;
    const requestedItemId = itemId;
    loading = true;
    try {
      const result = (await sla.getItemSLA(requestedItemId)) ?? [];
      // A newer request or an item switch supersedes this response.
      if (token !== requestToken) return;
      states = result;
    } catch {
      // SLA state is supplementary; never break the item detail on a read error.
      if (token === requestToken) states = [];
    } finally {
      if (token === requestToken) loading = false;
    }
  }

  $effect(() => {
    if (itemId) void load();
  });

  let hasOngoing = $derived(states.some((state) => state.ongoing));

  $effect(() => {
    // Only poll while a timer is actually running; completed history is static.
    if (!itemId || !hasOngoing) return;
    const timer = setInterval(() => void load(), SLA_REFRESH_MS);
    return () => clearInterval(timer);
  });

  // Item mutations (status, assignee, comments, etc.) are broadcast globally;
  // refresh the derived SLA state when they happen.
  useEventListener(() => window, 'refresh-work-items', () => void load());

  function formatDuration(ms) {
    if (ms == null) return '';
    const negative = ms < 0;
    let remaining = Math.abs(ms);
    const days = Math.floor(remaining / 86400000);
    remaining -= days * 86400000;
    const hours = Math.floor(remaining / 3600000);
    remaining -= hours * 3600000;
    const minutes = Math.round(remaining / 60000);
    const parts = [];
    if (days) parts.push(`${days}d`);
    if (hours) parts.push(`${hours}h`);
    if (!days && minutes) parts.push(`${minutes}m`);
    const text = parts.join(' ') || '0m';
    return negative ? `-${text}` : text;
  }

  function cycleRemaining(cycle) {
    if (cycle.display_format === 'due_date' && cycle.next_deadline_at) {
      const date = formatInstant(cycle.next_deadline_at, cycle.calendar_timezone || 'UTC', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
      });
      return t('items.sla.dueOn', { date });
    }
    if (cycle.remaining_ms != null) {
      return formatDuration(cycle.remaining_ms);
    }
    return formatDuration(cycle.elapsed_ms);
  }
</script>

{#if states.length > 0}
  <section
    class="mb-4 rounded border p-3"
    style="border-color: var(--ds-border)"
    data-testid="item-sla-panel"
  >
    <div class="flex items-center gap-2 mb-2">
      <Gauge class="w-4 h-4" style="color: var(--ds-text-subtle)" />
      <h3 class="text-sm font-semibold" style="color: var(--ds-text)">{t('items.sla.title')}</h3>
    </div>

    <div class="space-y-3">
      {#each states as state (state.metric_id)}
        <div data-testid="item-sla-metric-{state.metric_id}">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="text-sm font-medium">{state.metric_name}</span>
            {#if state.recalculating}
              <Lozenge color="gray" text={t('items.sla.recalculating')} />
            {/if}
          </div>

          {#if state.ongoing}
            {@const cycle = { ...state.ongoing, display_format: state.display_format }}
            {#if cycle.goal_duration_ms > 0}
              <div class="flex items-center gap-2 flex-wrap mt-1 text-sm">
                <Clock class="w-3.5 h-3.5" style="color: var(--ds-text-subtle)" />
                <span data-testid="item-sla-remaining-{state.metric_id}">{cycleRemaining(cycle)}</span>
                {#if cycle.breached}
                  <Lozenge color="red" text={t('items.sla.breached')} />
                {/if}
                {#if cycle.paused}
                  <Lozenge color="yellow" text={t('items.sla.paused')} />
                {/if}
                {#if !cycle.within_calendar_hours && !cycle.paused}
                  <Tooltip content={t('items.sla.outsideHours')}>
                    <Lozenge color="blue" text={t('items.sla.paused')} dataTestid="item-sla-paused-outside-hours-{state.metric_id}" />
                  </Tooltip>
                {/if}
              </div>
              <div class="text-xs mt-1" style="color: var(--ds-text-subtle)">
                {t('items.sla.elapsedOfGoal', {
                  elapsed: formatDuration(cycle.elapsed_ms),
                  goal: formatDuration(cycle.goal_duration_ms),
                })}
              </div>
              {#if cycle.coverage?.reference === 'team_service_hours' && cycle.coverage.discrepancy && cycle.coverage.discrepancy !== 'aligned'}
                <div
                  class="text-xs mt-1"
                  style="color: var(--ds-text-subtle)"
                  data-testid="item-sla-coverage-{state.metric_id}"
                >
                  {t(`items.sla.coverageDiscrepancies.${cycle.coverage.discrepancy}`)}
                  {#if cycle.coverage.note}
                    · {t(`items.sla.coverageNotes.${cycle.coverage.note}`)}
                  {/if}
                </div>
              {/if}
            {:else}
              <div
                class="text-xs mt-1"
                style="color: var(--ds-text-subtle)"
                data-testid="item-sla-no-goal-{state.metric_id}"
              >
                {t('items.sla.noGoal')}
              </div>
            {/if}
          {:else}
            <div class="text-xs mt-1" style="color: var(--ds-text-subtle)">
              {t('items.sla.notRunning')}
            </div>
          {/if}

          {#if state.completed?.length}
            <details class="mt-1">
              <summary class="text-xs cursor-pointer" style="color: var(--ds-text-subtle)">
                {t('items.sla.completedCycles', { count: state.completed.length })}
              </summary>
              <ul class="mt-1 space-y-1">
                {#each state.completed as cycle (cycle.cycle_no)}
                  <li
                    class="text-xs flex items-center gap-2"
                    style="color: var(--ds-text-subtle)"
                    data-testid="item-sla-completed-{state.metric_id}-{cycle.cycle_no}"
                  >
                    <span>#{cycle.cycle_no}</span>
                    <span>{formatDuration(cycle.elapsed_ms)}</span>
                    {#if cycle.breached}
                      <Lozenge color="red" text={t('items.sla.breached')} />
                    {/if}
                  </li>
                {/each}
              </ul>
            </details>
          {/if}
        </div>
      {/each}
    </div>
  </section>
{:else if loading}
  <div class="mb-4" aria-hidden="true"></div>
{/if}
