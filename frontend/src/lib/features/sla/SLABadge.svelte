<script>
  import { useEventListener } from 'runed';
  import Lozenge from '../../components/Lozenge.svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatInstant } from '../../utils/dateFormatter.js';
  import { getItemSLA, getSLAThresholds, onSLAThresholdsInvalidated } from './slaState.js';

  let { itemId = null, workspaceId = null, refreshToken = 0 } = $props();

  let states = $state([]);
  let thresholds = $state([]);
  let loaded = $state(false);
  // Plain (non-reactive) guard: hide the previous item's badge while its
  // replacement loads, but keep the current badge visible across the board's
  // periodic refresh so it does not blank every tick.
  let loadedItemId = null;
  let requestGeneration = 0;

  async function load(id, workspace, token) {
    if (!id) return;
    const generation = ++requestGeneration;
    if (id !== loadedItemId) loaded = false;
    const [stateList, thresholdList] = await Promise.all([getItemSLA(id, workspace), getSLAThresholds(workspace)]);
    if (generation !== requestGeneration || token !== refreshToken) return;
    states = stateList ?? [];
    thresholds = thresholdList ?? [];
    loaded = true;
    loadedItemId = id;
  }

  $effect(() => {
    // refreshToken is read so the board can force a reload on its own cadence.
    void load(itemId, workspaceId, refreshToken);
    return () => {
      requestGeneration++;
    };
  });

  useEventListener(() => window, 'refresh-work-items', () => void load(itemId, workspaceId, refreshToken));

  // A threshold mutation in this tab must refresh mounted badges immediately;
  // failed mutations never emit, and other tabs pick the change up once the
  // five-minute threshold freshness expires.
  $effect(() => {
    return onSLAThresholdsInvalidated((invalidatedWorkspaceId) => {
      if (
        invalidatedWorkspaceId != null &&
        Number(invalidatedWorkspaceId) !== Number(workspaceId)
      ) {
        return;
      }
      void load(itemId, workspaceId, refreshToken);
    });
  });

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

  function warningPercentFor(metricId) {
    const applicable = thresholds.filter(
      (threshold) =>
        threshold.is_active !== false && (threshold.metric_id == null || threshold.metric_id === metricId)
    );
    if (applicable.length === 0) return null;
    return Math.min(...applicable.map((threshold) => threshold.percent));
  }

  function formatDeadline(cycle) {
    return formatInstant(cycle.next_deadline_at, cycle.calendar_timezone || 'UTC', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
    });
  }

  // Pick the single worst ongoing cycle so a row never shows more than one badge.
  const signal = $derived.by(() => {
    let result = null;
    for (const state of states) {
      const cycle = state.ongoing;
      if (!cycle) continue;
      const breached = !!cycle.breached;
      const paused = !!cycle.paused;
      // A cycle without a matched goal reports within_calendar_hours=false,
      // but that is the synthetic 24/7 placeholder, not a service-hours pause.
      const outsideHours =
        !paused && cycle.goal_duration_ms > 0 && cycle.within_calendar_hours === false;
      const percent = warningPercentFor(state.metric_id);
      const warning =
        !breached &&
        percent != null &&
        cycle.goal_duration_ms > 0 &&
        cycle.elapsed_ms >= (cycle.goal_duration_ms * percent) / 100;
      const rank = breached ? 4 : paused ? 3 : warning ? 2 : outsideHours ? 1 : 0;
      if (!result || rank > result.rank) {
        result = {
          rank,
          breached,
          paused,
          warning,
          outsideHours,
          displayFormat: state.display_format,
          cycle,
        };
      }
    }
    return result;
  });
</script>

{#if loaded && signal}
  {#if signal.breached}
    <Lozenge color="red" text={t('items.sla.breached')} />
  {:else if signal.paused}
    <Lozenge color="yellow" text={t('items.sla.paused')} />
  {:else if signal.warning}
    <Lozenge color="yellow" text={t('items.sla.warning')} />
  {:else if signal.outsideHours}
    <Tooltip content={t('items.sla.outsideHours')}>
      <Lozenge color="blue" text={t('items.sla.paused')} dataTestid="sla-badge-paused-outside-hours" />
    </Tooltip>
  {:else if signal.displayFormat === 'due_date' && signal.cycle.next_deadline_at}
    <span class="text-xs" style="color: var(--ds-text-subtle)">
      {t('items.sla.dueOn', { date: formatDeadline(signal.cycle) })}
    </span>
  {:else if signal.cycle.remaining_ms != null}
    <span class="text-xs" style="color: var(--ds-text-subtle)">
      {formatDuration(signal.cycle.remaining_ms)}
    </span>
  {/if}
{/if}
