<script>
  import IconAlertTriangle from '@tabler/icons-svelte-runes/icons/alert-triangle';
  import IconDatabase from '@tabler/icons-svelte-runes/icons/database';
  import IconGauge from '@tabler/icons-svelte-runes/icons/gauge';
  import IconStack2 from '@tabler/icons-svelte-runes/icons/stack-2';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import { getCacheMemory } from '../../api/diagnostics.js';
  import { formatBytes } from '../../utils/bytes.js';
  import DiagnosticsSection from './DiagnosticsSection.svelte';

  let view = $state({ loading: true, error: null, data: null });
  let previousEvictions = new Map();

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const data = await getCacheMemory();
      const caches = (data.caches ?? []).map((cache) => {
        const prior = previousEvictions.get(cache.name) ?? cache.no_space_evictions;
        const evictionDelta = Math.max(0, cache.no_space_evictions - prior);
        previousEvictions.set(cache.name, cache.no_space_evictions);
        return { ...cache, eviction_delta: evictionDelta };
      });
      view = { loading: false, error: null, data: { ...data, caches } };
    } catch (err) {
      view = { ...view, loading: false, error: err?.message ?? String(err) };
    }
  }

  const caches = $derived(view.data?.caches ?? []);
  const churn = $derived(caches.some((cache) => cache.eviction_delta > 0));
  const columns = [
    { key: 'name', label: 'Cache', render: (cache) => cache.name },
    { key: 'health', label: 'Pressure', slot: 'health' },
    { key: 'entries', label: 'Entries', render: (cache) => cache.entries.toLocaleString() },
    { key: 'allocation', label: 'Allocated / maximum', render: (cache) => `${formatBytes(cache.allocated_capacity_bytes)} / ${formatBytes(cache.maximum_capacity_bytes)}` },
    { key: 'traffic', label: 'Hits / misses', render: (cache) => `${cache.hits.toLocaleString()} / ${cache.misses.toLocaleString()}` },
    { key: 'evictions', label: 'Evictions (interval / total)', render: (cache) => `${cache.eviction_delta.toLocaleString()} / ${cache.no_space_evictions.toLocaleString()}` },
  ];
</script>

<DiagnosticsSection
  title="Cache memory"
  subtitle="Process-local Go memory and bounded cache capacity. Eviction deltas reset when this page moves to another replica or reloads."
  dataTestId="diagnostics-cache-memory"
  onLoad={load}
  bind:loading={view.loading}
  bind:error={view.error}
  refreshInterval={5_000}
>
  {#snippet children()}
    {#if view.data && (!view.data.healthy || churn)}
      <Card>
        <div class="flex items-start gap-3 p-3" style="color: var(--ds-accent-red);" data-testid="cache-memory-alert">
          <IconAlertTriangle class="w-4 h-4 mt-0.5 shrink-0" />
          <span class="text-sm">{view.data.healthy ? 'Cache eviction occurred during this interval; check hit rates and database load.' : 'The live heap is above 90% of its configured Go memory limit.'}</span>
        </div>
      </Card>
    {/if}

    {#if view.data}
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard icon={IconGauge} label="Process budget" value={`${view.data.budget.process_limit_mb} MiB`} color="blue" />
        <StatCard icon={IconStack2} label="Go heap target" value={formatBytes(view.data.budget.go_limit_bytes)} color="purple" />
        <StatCard icon={IconDatabase} label="Cache allocated" value={formatBytes(view.data.allocated_cache_bytes)} color="green" />
        <StatCard icon={IconDatabase} label="Cache maximum" value={formatBytes(view.data.maximum_cache_bytes)} color="orange" />
      </div>

      <DataTable data={caches} {columns} keyField="name" emptyMessage="No process caches registered.">
        {#snippet health(cache)}
          {#if cache.eviction_delta > 0}
            <Lozenge appearance="warning" size="sm">Evicting</Lozenge>
          {:else}
            <Lozenge appearance="success" size="sm">Stable</Lozenge>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  {/snippet}
</DiagnosticsSection>
