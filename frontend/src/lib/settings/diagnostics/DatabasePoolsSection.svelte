<script>
  import IconAlertTriangle from '@tabler/icons-svelte-runes/icons/alert-triangle';
  import IconActivity from '@tabler/icons-svelte-runes/icons/activity';
  import IconBrandDatabricks from '@tabler/icons-svelte-runes/icons/brand-databricks';
  import IconCpu from '@tabler/icons-svelte-runes/icons/cpu';
  import IconDatabase from '@tabler/icons-svelte-runes/icons/database';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import { getDatabasePools } from '../../api/diagnostics.js';
  import DiagnosticsSection from './DiagnosticsSection.svelte';
  import { formatAuthenticatedDateTime } from '../../utils/authenticatedDateFormatter.js';
  import { formatBytes } from '../../utils/bytes.js';

  let view = $state({ loading: true, error: null, data: null });
  let previousInstance = '';
  const previous = new Map();
  const peaks = new Map();

  function formatDuration(value) {
    if (!Number.isFinite(value)) return '—';
    if (value < 1000) return `${value} ms`;
    return `${(value / 1000).toFixed(2)} s`;
  }

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const data = await getDatabasePools();
      if (previousInstance && previousInstance !== data.instance) {
        previous.clear();
        peaks.clear();
      }
      previousInstance = data.instance;
      const pools = (data.pools ?? []).map((pool) => {
        const prior = previous.get(pool.name);
        const waitCountDelta = prior ? Math.max(0, pool.wait_count - prior.wait_count) : 0;
        const waitDurationDelta = prior ? Math.max(0, pool.wait_duration_ms - prior.wait_duration_ms) : 0;
        const peakInUse = Math.max(peaks.get(pool.name) ?? 0, pool.in_use);
        peaks.set(pool.name, peakInUse);
        previous.set(pool.name, pool);
        return { ...pool, wait_count_delta: waitCountDelta, wait_duration_delta_ms: waitDurationDelta, peak_in_use: peakInUse };
      });
      view = { loading: false, error: null, data: { ...data, pools } };
    } catch (err) {
      view = { ...view, loading: false, error: err?.message ?? String(err) };
    }
  }

  const pools = $derived(view.data?.pools ?? []);
  const capacity = $derived(view.data?.capacity);
  const process = $derived(view.data?.process);
  const columns = [
    { key: 'name', label: 'Pool', render: (pool) => pool.name },
    { key: 'driver', label: 'Driver', render: (pool) => pool.driver },
    { key: 'health', label: 'Health', slot: 'health' },
    { key: 'connections', label: 'Connections (in use / idle / open / max)', render: (pool) => `${pool.in_use} / ${pool.idle} / ${pool.open_connections} / ${pool.max_open_connections}` },
    { key: 'peak', label: 'Peak observed', render: (pool) => `${pool.peak_in_use} / ${pool.max_open_connections}` },
    { key: 'utilization', label: 'Utilization', render: (pool) => `${pool.utilization_percent.toFixed(1)}%` },
    { key: 'waits', label: 'Waits (interval / total)', render: (pool) => `${pool.wait_count_delta} / ${pool.wait_count}` },
    { key: 'waitDuration', label: 'Wait duration (interval / total)', render: (pool) => `${formatDuration(pool.wait_duration_delta_ms)} / ${formatDuration(pool.wait_duration_ms)}` },
    { key: 'closed', label: 'Closed (idle / idle timeout / lifetime)', render: (pool) => `${pool.max_idle_closed} / ${pool.max_idle_time_closed} / ${pool.max_lifetime_closed}` },
  ];
</script>

<DiagnosticsSection
  title="Database pools"
  subtitle="Process-local SQL pool occupancy and cumulative wait counters. Interval deltas and observed peaks reset when this tab moves to another replica or reloads. Pool saturation is diagnostic and does not fail readiness."
  dataTestId="diagnostics-database-pools"
  onLoad={load}
  bind:loading={view.loading}
  bind:error={view.error}
  refreshInterval={5_000}
>
  {#snippet children()}
    {#if view.data && !view.data.healthy}
      <Card>
        <div class="flex items-start gap-3 p-3" style="color: var(--ds-accent-red);" data-testid="database-pools-alert">
          <IconAlertTriangle class="w-4 h-4 mt-0.5 shrink-0" />
          <span class="text-sm">
            Database capacity needs attention. Check current pool utilization, new wait deltas, and the declared PostgreSQL deployment budget below.
          </span>
        </div>
      </Card>
    {/if}

    {#if view.data}
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard icon={IconDatabase} label="Registered pools" value={String(pools.length)} color="blue" />
        <StatCard icon={IconCpu} label="Goroutines" value={String(process?.goroutines ?? 0)} color="purple" />
        <StatCard icon={IconActivity} label="Heap allocated" value={formatBytes(process?.heap_alloc_bytes)} color="green" />
        <StatCard icon={IconBrandDatabricks} label="Runtime reserved" value={formatBytes(process?.system_bytes)} color="orange" />
      </div>

      <Card>
        <div class="p-3 text-sm grid grid-cols-1 md:grid-cols-2 gap-2" style="color: var(--ds-text);">
          <div><span style="color: var(--ds-text-subtle);">Instance:</span> <span class="font-mono">{view.data.instance}</span></div>
          <div><span style="color: var(--ds-text-subtle);">Sample:</span> {formatAuthenticatedDateTime(view.data.sampled_at)}</div>
          {#if capacity}
            <div><span style="color: var(--ds-text-subtle);">Deployment budget:</span> {capacity.required_connections} / {capacity.server_max_connections} connections ({capacity.utilization_percent.toFixed(1)}%)</div>
            <div><span style="color: var(--ds-text-subtle);">Formula:</span> {capacity.replica_count} × {capacity.connections_per_replica} + {capacity.headroom_connections} headroom</div>
          {:else}
            <div class="md:col-span-2" style="color: var(--ds-text-subtle);">Replica capacity budgeting applies to PostgreSQL deployments.</div>
          {/if}
        </div>
      </Card>

      <DataTable data={pools} {columns} keyField="name" emptyMessage="No database pools registered.">
        {#snippet health(pool)}
          {#if pool.saturated}
            <Lozenge appearance="error" size="sm">Saturated</Lozenge>
          {:else if pool.wait_count_delta > 0}
            <Lozenge appearance="warning" size="sm">Waiting</Lozenge>
          {:else}
            <Lozenge appearance="success" size="sm">Healthy</Lozenge>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  {/snippet}
</DiagnosticsSection>
