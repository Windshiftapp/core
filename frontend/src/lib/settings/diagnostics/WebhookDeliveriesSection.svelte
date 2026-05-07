<script>
  import { onDestroy, onMount } from 'svelte';
  import { IconRefresh, IconAlertCircle, IconTrash, IconActivity, IconCheck, IconX, IconClock } from '@tabler/icons-svelte-runes';
  import Card from '../../components/Card.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import {
    getWebhookDeliveries,
    getWebhookStats,
    purgeWebhookDeliveries,
  } from '../../api/diagnostics.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';

  let state = $state({ loading: true, error: null, recent: [], stats: [] });
  let lastRefreshed = $state(null);
  let purgeOlderThan = $state('30d');
  let purging = $state(false);

  async function load() {
    state = { ...state, loading: true, error: null };
    try {
      const [recent, stats] = await Promise.all([
        getWebhookDeliveries({ since: '24h', limit: 50 }),
        getWebhookStats({ since: '24h' }),
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
    if (!confirm(`Permanently delete all webhook delivery rows older than ${purgeOlderThan}? This cannot be undone.`)) {
      return;
    }
    purging = true;
    try {
      const res = await purgeWebhookDeliveries(purgeOlderThan);
      successToast(`Deleted ${res?.deleted ?? 0} delivery rows`);
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

  function formatLatency(ms) {
    if (ms == null) return '—';
    if (ms < 1000) return `${ms} ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }

  function truncate(s, n) {
    if (!s) return '';
    return s.length > n ? `${s.slice(0, n - 1)}…` : s;
  }

  // Aggregate totals across all channels for the headline tiles.
  const totals = $derived.by(() => {
    let total = 0;
    let success = 0;
    let failed = 0;
    let latencyWeighted = 0;
    let latencyCount = 0;
    for (const s of state.stats) {
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

  const failuresOnly = $derived(state.recent.filter((d) => !d.success));

  const statsColumns = [
    { key: 'channel_name', label: 'Channel', render: (s) => s.channel_name || `#${s.channel_id}` },
    { key: 'total', label: 'Total', align: 'text-right', textColor: 'var(--ds-text-subtle)' },
    { key: 'successes', label: 'Success', align: 'text-right', slot: 'successes' },
    { key: 'failures', label: 'Failed', align: 'text-right', slot: 'failures' },
    { key: 'avg_latency_ms', label: 'Avg latency', align: 'text-right', render: (s) => formatLatency(s.avg_latency_ms), textColor: 'var(--ds-text-subtle)' },
    { key: 'last_failure_at', label: 'Last failure', render: (s) => formatTime(s.last_failure_at), textColor: 'var(--ds-text-subtle)' },
  ];

  const failureColumns = [
    { key: 'requested_at', label: 'When', render: (d) => formatTime(d.requested_at), textColor: 'var(--ds-text-subtle)' },
    { key: 'channel_name', label: 'Channel', render: (d) => d.channel_name || `#${d.channel_id}` },
    { key: 'event_type', label: 'Event', textColor: 'var(--ds-text-subtle)' },
    { key: 'transport', label: 'Transport', textColor: 'var(--ds-text-subtle)' },
    { key: 'response_status_code', label: 'Status', align: 'text-right', render: (d) => String(d.response_status_code ?? '—') },
    { key: 'error_message', label: 'Error', slot: 'errorMessage' },
  ];
</script>

<section class="space-y-6" data-testid="diagnostics-webhook-deliveries">
  <div class="flex items-start justify-between gap-4">
    <div>
      <h3 class="text-base font-semibold" style="color: var(--ds-text);">Outbound webhook deliveries (last 24h)</h3>
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        Every send attempt is recorded — HTTP and plugin transports, success and failure. Auto-refreshes every 30s.
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
          <p class="font-medium">Failed to load webhook deliveries</p>
          <p style="color: var(--ds-text-subtle);">{state.error}</p>
        </div>
      </div>
    </Card>
  {/if}

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
        value={totals.avgLatency == null ? '—' : formatLatency(totals.avgLatency)}
        color="purple"
      />
    </div>
  </div>

  <div>
    <h4 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Per-channel summary</h4>
    <DataTable
      columns={statsColumns}
      data={state.stats}
      keyField="channel_id"
      emptyMessage="No webhook deliveries in the last 24h."
    >
      {#snippet successes(s)}
        {@const rate = s.total > 0 ? Math.round((s.successes / s.total) * 100) : null}
        <span style="color: var(--ds-text);">{s.successes}{rate != null ? ` (${rate}%)` : ''}</span>
      {/snippet}
      {#snippet failures(s)}
        <span style="color: {s.failures > 0 ? 'var(--ds-text-danger)' : 'var(--ds-text-subtle)'};">{s.failures}</span>
      {/snippet}
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
        <span style="color: var(--ds-text);" title={d.error_message}>{truncate(d.error_message, 80) || '—'}</span>
      {/snippet}
    </DataTable>
  </div>

  <Card variant="outlined">
    <div class="flex items-center gap-3 flex-wrap">
      <IconTrash size={16} stroke={1.75} style="color: var(--ds-icon-subtle);" />
      <div class="text-sm flex-1" style="color: var(--ds-text);">
        Manual purge — delete delivery rows older than
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
