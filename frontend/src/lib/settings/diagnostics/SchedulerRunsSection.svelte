<script>
  import { onDestroy, onMount } from 'svelte';
  import { IconRefresh, IconAlertCircle, IconTrash, IconActivity, IconCheck, IconX, IconClock } from '@tabler/icons-svelte-runes';
  import Card from '../../components/Card.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import {
    getSchedulerRuns,
    getSchedulerStats,
    purgeSchedulerRuns,
  } from '../../api/diagnostics.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';

  let state = $state({ loading: true, error: null, recent: [], stats: [] });
  let lastRefreshed = $state(null);
  let purgeOlderThan = $state('30d');
  let purging = $state(false);

  // Display order — keep stable even when backend returns alphabetical.
  const SCHEDULER_ORDER = ['briefing', 'email', 'recurrence', 'notification'];
  const SCHEDULER_LABELS = {
    briefing: 'Briefing (6h)',
    email: 'Email IMAP (5m)',
    recurrence: 'Recurrence (5m)',
    notification: 'Notification batch (5m)',
  };
  const ITEM_LABELS = {
    briefing: 'users',
    email: 'channels',
    recurrence: 'instances',
    notification: 'batches',
  };

  async function load() {
    state = { ...state, loading: true, error: null };
    try {
      const [recent, stats] = await Promise.all([
        getSchedulerRuns({ since: '24h', limit: 50 }),
        getSchedulerStats({ since: '24h' }),
      ]);
      state = { loading: false, error: null, recent: recent ?? [], stats: stats ?? [] };
      lastRefreshed = new Date();
    } catch (err) {
      state = { ...state, loading: false, error: err?.message ?? String(err) };
    }
  }

  async function purge() {
    if (!purgeOlderThan || !/^\d+[dhm]$/.test(purgeOlderThan)) {
      errorToast('Use a duration like 30d, 168h, or 60m');
      return;
    }
    if (!confirm(`Permanently delete scheduler run rows older than ${purgeOlderThan}? This cannot be undone.`)) {
      return;
    }
    purging = true;
    try {
      const res = await purgeSchedulerRuns(purgeOlderThan);
      successToast(`Deleted ${res?.deleted ?? 0} run rows`);
      await load();
    } catch (err) {
      errorToast(err?.message ?? 'Purge failed');
    } finally {
      purging = false;
    }
  }

  let interval;
  onMount(() => {
    load();
    interval = setInterval(load, 30_000);
  });
  onDestroy(() => {
    if (interval) clearInterval(interval);
  });

  function formatTime(iso) {
    if (!iso) return '—';
    return new Date(iso).toISOString().replace('T', ' ').replace(/\..*Z$/, ' UTC');
  }

  function formatDuration(ms) {
    if (ms == null) return '—';
    if (ms < 1000) return `${ms} ms`;
    const sec = ms / 1000;
    if (sec < 60) return `${sec.toFixed(sec < 10 ? 2 : 1)}s`;
    const min = Math.floor(sec / 60);
    return `${min}m ${Math.round(sec - min * 60)}s`;
  }

  function truncate(s, n) {
    if (!s) return '';
    return s.length > n ? `${s.slice(0, n - 1)}…` : s;
  }

  // Headline tile aggregates across all schedulers in the window.
  const totals = $derived.by(() => {
    let total = 0, success = 0, failed = 0, durWeighted = 0, durCount = 0;
    for (const s of state.stats) {
      total += s.total ?? 0;
      success += s.successes ?? 0;
      failed += s.failures ?? 0;
      if (s.avg_duration_ms != null && s.total) {
        durWeighted += s.avg_duration_ms * s.total;
        durCount += s.total;
      }
    }
    const successRate = total > 0 ? Math.round((success / total) * 100) : null;
    const avgDuration = durCount > 0 ? Math.round(durWeighted / durCount) : null;
    return { total, success, failed, successRate, avgDuration };
  });

  // Order stats by SCHEDULER_ORDER so the page is stable.
  const orderedStats = $derived.by(() => {
    const map = new Map(state.stats.map((s) => [s.scheduler_name, s]));
    return SCHEDULER_ORDER
      .filter((k) => map.has(k))
      .map((k) => map.get(k))
      .concat(state.stats.filter((s) => !SCHEDULER_ORDER.includes(s.scheduler_name)));
  });

  const failuresOnly = $derived(state.recent.filter((r) => !r.success));

  const statsColumns = [
    { key: 'scheduler_name', label: 'Scheduler', render: (s) => SCHEDULER_LABELS[s.scheduler_name] || s.scheduler_name },
    { key: 'total', label: 'Runs', align: 'text-right', textColor: 'var(--ds-text-subtle)' },
    { key: 'successes', label: 'Success', align: 'text-right', slot: 'successes' },
    { key: 'failures', label: 'Failed', align: 'text-right', slot: 'failures' },
    { key: 'avg_duration_ms', label: 'Avg duration', align: 'text-right', render: (s) => formatDuration(s.avg_duration_ms), textColor: 'var(--ds-text-subtle)' },
    { key: 'total_processed', label: 'Items', align: 'text-right', slot: 'items' },
    { key: 'last_failure_at', label: 'Last failure', render: (s) => formatTime(s.last_failure_at), textColor: 'var(--ds-text-subtle)' },
  ];

  const failureColumns = [
    { key: 'started_at', label: 'When', render: (run) => formatTime(run.started_at), textColor: 'var(--ds-text-subtle)' },
    { key: 'scheduler_name', label: 'Scheduler', render: (run) => SCHEDULER_LABELS[run.scheduler_name] || run.scheduler_name },
    { key: 'duration_ms', label: 'Duration', align: 'text-right', render: (run) => formatDuration(run.duration_ms), textColor: 'var(--ds-text-subtle)' },
    { key: 'error_message', label: 'Error', slot: 'errorMessage' },
  ];
</script>

<section class="space-y-6" data-testid="diagnostics-scheduler-runs">
  <div class="flex items-start justify-between gap-4">
    <div>
      <h3 class="text-base font-semibold" style="color: var(--ds-text);">Background scheduler runs (last 24h)</h3>
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        Every tick of every in-process scheduler is recorded. Auto-refreshes every 30s.
      </p>
    </div>
    <button
      onclick={load}
      disabled={state.loading}
      class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md border"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
    >
      <IconRefresh size={14} stroke={1.75} />
      {state.loading ? 'Refreshing…' : 'Refresh'}
    </button>
  </div>

  {#if state.error}
    <Card variant="outlined">
      <div class="flex items-start gap-3" style="color: var(--ds-text-danger);">
        <IconAlertCircle size={18} stroke={1.75} style="flex-shrink: 0; margin-top: 2px;" />
        <div class="text-sm">
          <p class="font-medium">Failed to load scheduler runs</p>
          <p style="color: var(--ds-text-subtle);">{state.error}</p>
        </div>
      </div>
    </Card>
  {/if}

  <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
    <div data-testid="scheduler-stat-total">
      <StatCard icon={IconActivity} label="Total runs" value={totals.total.toString()} color="blue" />
    </div>
    <div data-testid="scheduler-stat-success-rate">
      <StatCard
        icon={IconCheck}
        label="Success rate"
        value={totals.successRate == null ? '—' : `${totals.successRate}%`}
        color={totals.successRate != null && totals.successRate < 95 ? 'orange' : 'green'}
      />
    </div>
    <div data-testid="scheduler-stat-failures">
      <StatCard
        icon={IconX}
        label="Failures"
        value={totals.failed.toString()}
        color={totals.failed > 0 ? 'orange' : 'green'}
      />
    </div>
    <div data-testid="scheduler-stat-avg-duration">
      <StatCard
        icon={IconClock}
        label="Avg duration"
        value={totals.avgDuration == null ? '—' : formatDuration(totals.avgDuration)}
        color="purple"
      />
    </div>
  </div>

  <div>
    <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Per-scheduler summary</h4>
    <DataTable
      columns={statsColumns}
      data={orderedStats}
      keyField="scheduler_name"
      emptyMessage="No scheduler runs in the last 24h. Most schedulers tick every 5 minutes; the briefing scheduler ticks every 6 hours."
    >
      {#snippet successes(s)}
        {@const rate = s.total > 0 ? Math.round((s.successes / s.total) * 100) : null}
        <span style="color: var(--ds-text);">{s.successes}{rate != null ? ` (${rate}%)` : ''}</span>
      {/snippet}
      {#snippet failures(s)}
        <span style="color: {s.failures > 0 ? 'var(--ds-text-danger)' : 'var(--ds-text-subtle)'};">{s.failures}</span>
      {/snippet}
      {#snippet items(s)}
        {@const itemLabel = ITEM_LABELS[s.scheduler_name] || ''}
        <span style="color: var(--ds-text-subtle);">{s.total_processed != null ? `${s.total_processed} ${itemLabel}` : '—'}</span>
      {/snippet}
    </DataTable>
  </div>

  <div>
    <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Recent failures</h4>
    <DataTable
      columns={failureColumns}
      data={failuresOnly}
      keyField="id"
      emptyMessage="No failed scheduler runs in the last 24h."
    >
      {#snippet errorMessage(run)}
        <span style="color: var(--ds-text);" title={run.error_message}>{truncate(run.error_message, 80) || '—'}</span>
      {/snippet}
    </DataTable>
  </div>

  <Card variant="outlined">
    <div class="flex items-center gap-3 flex-wrap">
      <IconTrash size={16} stroke={1.75} style="color: var(--ds-icon-subtle);" />
      <div class="text-sm flex-1" style="color: var(--ds-text);">
        Manual purge — delete scheduler run rows older than
        <input
          type="text"
          bind:value={purgeOlderThan}
          placeholder="30d"
          class="px-2 py-1 text-sm rounded border mx-2 w-20 font-mono"
          style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
        />
        <span style="color: var(--ds-text-subtle);">(format: 30d, 168h, 60m)</span>
      </div>
      <button
        onclick={purge}
        disabled={purging}
        class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md border"
        style="border-color: var(--ds-border-danger); background-color: var(--ds-surface-raised); color: var(--ds-text-danger);"
      >
        {purging ? 'Purging…' : 'Purge old rows'}
      </button>
    </div>
  </Card>

  {#if lastRefreshed}
    <p class="text-xs" style="color: var(--ds-text-subtle);">
      Last refreshed {lastRefreshed.toLocaleTimeString()}
    </p>
  {/if}
</section>
