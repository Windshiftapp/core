<script>
  import DataTable from '../../components/DataTable.svelte';
  import {
    getSchedulerRuns,
    getSchedulerStats,
    purgeSchedulerRuns,
  } from '../../api/diagnostics.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import DiagnosticsSection from './DiagnosticsSection.svelte';
  import DiagnosticsTotalsGrid from './DiagnosticsTotalsGrid.svelte';
  import DiagnosticsFailuresTable from './DiagnosticsFailuresTable.svelte';
  import { diagnosticsTotals, formatDurationMs, formatUtcTime, runDiagnosticsPurge, successSummary } from './format-utils.js';

  let view = $state({ loading: true, error: null, recent: [], stats: [] });
  let lastRefreshed = $state(null);
  let purgeOlderThan = $state('30d');
  let purging = $state(false);

  // Display order — keep stable even when backend returns alphabetical.
  const SCHEDULER_ORDER = ['briefing', 'email', 'email_outbox', 'recurrence', 'notification'];
  const SCHEDULER_LABELS = {
    briefing: 'Briefing (6h)',
    email: 'Email IMAP (5m)',
    email_outbox: 'Outbound email (1m)',
    recurrence: 'Recurrence (5m)',
    notification: 'Notification batch (5m)',
  };
  const ITEM_LABELS = {
    briefing: 'users',
    email: 'channels',
    email_outbox: 'emails',
    recurrence: 'instances',
    notification: 'batches',
  };

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const [recent, stats] = await Promise.all([
        getSchedulerRuns({ since: '24h', limit: 50 }),
        getSchedulerStats({ since: '24h' }),
      ]);
      view = { loading: false, error: null, recent: recent ?? [], stats: stats ?? [] };
      lastRefreshed = new Date();
    } catch (err) {
      view = { ...view, loading: false, error: err?.message ?? String(err) };
    }
  }

  async function purge() {
    await runDiagnosticsPurge({
      olderThan: purgeOlderThan,
      confirmMessage: `Permanently delete scheduler run rows older than ${purgeOlderThan}? This cannot be undone.`,
      execute: () => purgeSchedulerRuns(purgeOlderThan),
      successMessage: (res) => `Deleted ${res?.deleted ?? 0} run rows`,
      reload: load,
      setPurging: (value) => {
        purging = value;
      },
      errorToast,
      successToast,
    });
  }

  // Headline tile aggregates across all schedulers in the window.
  const totals = $derived(diagnosticsTotals(view.stats, 'avg_duration_ms'));

  // Order stats by SCHEDULER_ORDER so the page is stable.
  const orderedStats = $derived.by(() => {
    const map = new Map(view.stats.map((s) => [s.scheduler_name, s]));
    return SCHEDULER_ORDER
      .filter((k) => map.has(k))
      .map((k) => map.get(k))
      .concat(view.stats.filter((s) => !SCHEDULER_ORDER.includes(s.scheduler_name)));
  });

  const failuresOnly = $derived(view.recent.filter((r) => !r.success));

  const statsColumns = [
    { key: 'scheduler_name', label: 'Scheduler', render: (s) => SCHEDULER_LABELS[s.scheduler_name] || s.scheduler_name },
    { key: 'total', label: 'Runs', align: 'text-right', textColor: 'var(--ds-text-subtle)' },
    { key: 'successes', label: 'Success', align: 'text-right', render: successSummary },
    { key: 'failures', label: 'Failed', align: 'text-right', render: (s) => String(s.failures ?? 0) },
    { key: 'avg_duration_ms', label: 'Avg duration', align: 'text-right', render: (s) => formatDurationMs(s.avg_duration_ms), textColor: 'var(--ds-text-subtle)' },
    { key: 'total_processed', label: 'Items', align: 'text-right', slot: 'items' },
    { key: 'last_failure_at', label: 'Last failure', render: (s) => formatUtcTime(s.last_failure_at), textColor: 'var(--ds-text-subtle)' },
  ];

  const failureColumns = [
    { key: 'started_at', label: 'When', render: (run) => formatUtcTime(run.started_at), textColor: 'var(--ds-text-subtle)' },
    { key: 'scheduler_name', label: 'Scheduler', render: (run) => SCHEDULER_LABELS[run.scheduler_name] || run.scheduler_name },
    { key: 'duration_ms', label: 'Duration', align: 'text-right', render: (run) => formatDurationMs(run.duration_ms), textColor: 'var(--ds-text-subtle)' },
    { key: 'error_message', label: 'Error', slot: 'errorMessage' },
  ];
</script>

<DiagnosticsSection
  title="Background scheduler runs (last 24h)"
  subtitle="Every tick of every in-process scheduler is recorded. Auto-refreshes every 30s."
  dataTestId="diagnostics-scheduler-runs"
  onLoad={load}
  lastRefreshed={lastRefreshed}
  onPurge={purge}
  bind:purgeOlderThan
  purging={purging}
  purgeLabel="Manual purge — delete scheduler run rows older than"
  bind:loading={view.loading}
  bind:error={view.error}
>
  {#snippet children()}
    <DiagnosticsTotalsGrid
      {totals}
      testids={{
        total: 'scheduler-stat-total',
        successRate: 'scheduler-stat-success-rate',
        failures: 'scheduler-stat-failures',
        avg: 'scheduler-stat-avg-duration',
      }}
      totalLabel="Total runs"
      avgLabel="Avg duration"
      formatAvg={formatDurationMs}
    />

    <div>
      <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Per-scheduler summary</h4>
      <DataTable
        columns={statsColumns}
        data={orderedStats}
        keyField="scheduler_name"
        emptyMessage="No scheduler runs in the last 24h. Most schedulers tick every 5 minutes; the briefing scheduler ticks every 6 hours."
      >
        {#snippet items(s)}
          {@const itemLabel = ITEM_LABELS[s.scheduler_name] || ''}
          <span style="color: var(--ds-text-subtle);">{s.total_processed != null ? `${s.total_processed} ${itemLabel}` : '—'}</span>
        {/snippet}
      </DataTable>
    </div>

    <div>
      <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Recent failures</h4>
      <DiagnosticsFailuresTable
        columns={failureColumns}
        rows={failuresOnly}
        emptyMessage="No failed scheduler runs in the last 24h."
      />
    </div>
  {/snippet}
</DiagnosticsSection>
