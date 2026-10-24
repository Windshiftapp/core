<script>
  import IconActivity from '@tabler/icons-svelte-runes/icons/activity';
  import IconCheck from '@tabler/icons-svelte-runes/icons/check';
  import IconX from '@tabler/icons-svelte-runes/icons/x';
  import IconClock from '@tabler/icons-svelte-runes/icons/clock';
  import StatCard from '../../components/StatCard.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import {
    getWebhookDeliveries,
    getWebhookDispatchStats,
    getWebhookStats,
    purgeWebhookDeliveries,
  } from '../../api/diagnostics.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import DiagnosticsSection from './DiagnosticsSection.svelte';
  import { formatLatencyMs, formatUtcTime, runDiagnosticsPurge, successSummary, truncateText } from './format-utils.js';

  let view = $state({ loading: true, error: null, recent: [], stats: [], dispatch: null });
  let lastRefreshed = $state(null);
  let purgeOlderThan = $state('30d');
  let purging = $state(false);

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const [recent, stats, dispatch] = await Promise.all([
        getWebhookDeliveries({ since: '24h', limit: 50 }),
        getWebhookStats({ since: '24h' }),
        getWebhookDispatchStats(),
      ]);
      view.loading = false;
      view.error = null;
      view.recent = recent ?? [];
      view.stats = stats ?? [];
      view.dispatch = dispatch ?? null;
      lastRefreshed = new Date();
    } catch (err) {
      view.loading = false;
      view.error = err?.message ?? String(err);
    }
  }

  async function purge() {
    await runDiagnosticsPurge({ olderThan: purgeOlderThan, confirmMessage: `Permanently delete all webhook delivery rows older than ${purgeOlderThan}? This cannot be undone.`, execute: () => purgeWebhookDeliveries(purgeOlderThan), successMessage: (res) => `Deleted ${res?.deleted ?? 0} delivery rows`, reload: load, setPurging: (value) => (purging = value), errorToast, successToast });
  }

  // Aggregate totals across all channels for the headline tiles.
  const totals = $derived.by(() => {
    let total = 0;
    let success = 0;
    let failed = 0;
    let latencyWeighted = 0;
    let latencyCount = 0;
    for (const s of view.stats) {
      total += s.total ?? 0;
      success += s.successes ?? 0;
      failed += s.failures ?? 0;
      if (s.avg_latency_ms != null && s.total) {
        latencyWeighted += s.avg_latency_ms * s.total;
        latencyCount += s.total;
      }
    }
    const successRate = total > 0 ? Math.round((success / total) * 100) : null;
    const avgLatency = latencyCount > 0 ? Math.round(latencyWeighted / latencyCount) : null;
    return { total, success, failed, successRate, avgLatency };
  });

  const failuresOnly = $derived(view.recent.filter((d) => !d.success));

  const statsColumns = [
    { key: 'channel_name', label: 'Channel', render: (s) => s.channel_name || `#${s.channel_id}` },
    { key: 'total', label: 'Total', align: 'text-right', textColor: 'var(--ds-text-subtle)' },
    { key: 'successes', label: 'Success', align: 'text-right', render: successSummary },
    { key: 'failures', label: 'Failed', align: 'text-right', render: (s) => String(s.failures ?? 0) },
    { key: 'avg_latency_ms', label: 'Avg latency', align: 'text-right', render: (s) => formatLatencyMs(s.avg_latency_ms), textColor: 'var(--ds-text-subtle)' },
    { key: 'last_failure_at', label: 'Last failure', render: (s) => formatUtcTime(s.last_failure_at), textColor: 'var(--ds-text-subtle)' },
  ];

  const failureColumns = [
    { key: 'requested_at', label: 'When', render: (d) => formatUtcTime(d.requested_at), textColor: 'var(--ds-text-subtle)' },
    { key: 'channel_name', label: 'Channel', render: (d) => d.channel_name || `#${d.channel_id}` },
    { key: 'event_type', label: 'Event', textColor: 'var(--ds-text-subtle)' },
    { key: 'transport', label: 'Transport', textColor: 'var(--ds-text-subtle)' },
    { key: 'response_status_code', label: 'Status', align: 'text-right', render: (d) => String(d.response_status_code ?? '—') },
    { key: 'error_message', label: 'Error', slot: 'errorMessage' },
  ];
</script>

<DiagnosticsSection
  title="Outbound webhook deliveries (last 24h)"
  subtitle="Every send attempt is recorded — HTTP and plugin transports, success and failure. Auto-refreshes every 30s."
  dataTestId="diagnostics-webhook-deliveries"
  onLoad={load}
  lastRefreshed={lastRefreshed}
  onPurge={purge}
  bind:purgeOlderThan
  purging={purging}
  purgeLabel="Manual purge — delete delivery rows older than"
  bind:loading={view.loading}
  bind:error={view.error}
>
  {#snippet children()}
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      <div data-testid="webhook-stat-total">
        <StatCard icon={IconActivity} label="Total deliveries" value={totals.total.toString()} color="blue" />
      </div>
      <div data-testid="webhook-stat-success-rate">
        <StatCard
          icon={IconCheck}
          label="Success rate"
          value={totals.successRate == null ? '—' : `${totals.successRate}%`}
          color={totals.successRate != null && totals.successRate < 95 ? 'orange' : 'green'}
        />
      </div>
      <div data-testid="webhook-stat-failures">
        <StatCard
          icon={IconX}
          label="Failures"
          value={totals.failed.toString()}
          color={totals.failed > 0 ? 'orange' : 'green'}
        />
      </div>
      <div data-testid="webhook-stat-avg-latency">
        <StatCard
          icon={IconClock}
          label="Avg latency"
          value={totals.avgLatency == null ? '—' : formatLatencyMs(totals.avgLatency)}
          color="purple"
        />
      </div>
    </div>

    <div>
      <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Dispatch pipeline</h4>
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          icon={IconActivity}
          label="Queue depth"
          value={`${view.dispatch?.queue_depth ?? 0}/${view.dispatch?.queue_capacity ?? 0}`}
          color={(view.dispatch?.queue_depth ?? 0) > 0 ? 'orange' : 'green'}
        />
        <StatCard
          icon={IconCheck}
          label="Active workers"
          value={String(view.dispatch?.active_workers ?? 0)}
          color="blue"
        />
        <StatCard
          icon={IconX}
          label="Dropped events"
          value={String(view.dispatch?.dropped ?? 0)}
          color={(view.dispatch?.dropped ?? 0) > 0 ? 'orange' : 'green'}
        />
        <StatCard
          icon={IconClock}
          label="Oldest event"
          value={formatLatencyMs(view.dispatch?.oldest_event_age_ms ?? 0)}
          color={(view.dispatch?.oldest_event_age_ms ?? 0) > 5000 ? 'orange' : 'purple'}
        />
      </div>
      <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
        Processed {view.dispatch?.processed ?? 0} of {view.dispatch?.enqueued ?? 0} accepted events;
        {view.dispatch?.failed_events ?? 0} event failures, {view.dispatch?.retried ?? 0} retries.
        Subscription index: {view.dispatch?.subscription_cache_entries ?? 0} channels,
        {view.dispatch?.subscription_cache_hits ?? 0} hits / {view.dispatch?.subscription_cache_misses ?? 0} misses.
        Dispatch DB time: {formatLatencyMs(view.dispatch?.database_time_ms ?? 0)}.
      </p>
    </div>

    <div>
      <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Per-channel summary</h4>
      <DataTable
        columns={statsColumns}
        data={view.stats}
        keyField="channel_id"
        emptyMessage="No webhook deliveries in the last 24h."
      >
      </DataTable>
    </div>

    <div>
      <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Recent failures</h4>
      <DataTable
        columns={failureColumns}
        data={failuresOnly}
        keyField="id"
        emptyMessage="No failed deliveries in the last 24h."
      >
        {#snippet errorMessage(d)}
          <span style="color: var(--ds-text);" title={d.error_message}>{truncateText(d.error_message, 80) || '—'}</span>
        {/snippet}
      </DataTable>
    </div>
  {/snippet}
</DiagnosticsSection>
