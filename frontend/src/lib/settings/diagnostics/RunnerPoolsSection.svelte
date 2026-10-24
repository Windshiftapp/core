<script>
  // Runner-pool health (WI-272): for every pool, who could claim work
  // (heartbeat-live runners) versus what is waiting (queued/running runs).
  // The unhealthy signature is queued work with zero live runners — the
  // "assigned a ticket but nothing happens" case.
  import IconAlertTriangle from '@tabler/icons-svelte-runes/icons/alert-triangle';
  import IconCircleCheck from '@tabler/icons-svelte-runes/icons/circle-check';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { getRunnerPools } from '../../api/diagnostics.js';
  import DiagnosticsSection from './DiagnosticsSection.svelte';
  import { formatAgeSeconds, formatRelativeTime } from './format-utils.js';

  let view = $state({ loading: true, error: null, pools: [] });

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const pools = await getRunnerPools();
      view = { loading: false, error: null, pools: pools ?? [] };
    } catch (err) {
      view = { ...view, loading: false, error: err?.message ?? String(err) };
    }
  }

  const unhealthy = $derived(view.pools.filter((p) => !p.healthy));

  const columns = [
    { key: 'name', label: 'Pool', render: (p) => p.name || `#${p.id}` },
    { key: 'health', label: 'Health', slot: 'health' },
    { key: 'runners', label: 'Runners (live / stale / revoked)', render: (p) => `${p.live_runners} / ${p.stale_runners} / ${p.revoked_runners}` },
    { key: 'last_heartbeat_at', label: 'Last heartbeat', render: (p) => formatRelativeTime(p.last_heartbeat_at, 'never'), textColor: 'var(--ds-text-subtle)' },
    { key: 'queued_runs', label: 'Queued', render: (p) => String(p.queued_runs) },
    { key: 'oldest', label: 'Oldest queued', render: (p) => formatAgeSeconds(p.oldest_queued_seconds), textColor: 'var(--ds-text-subtle)' },
    { key: 'running_runs', label: 'Running', render: (p) => p.max_concurrent_runs > 0 ? `${p.running_runs} / ${p.max_concurrent_runs}` : String(p.running_runs) },
  ];
</script>

<DiagnosticsSection
  title="Runner pools"
  subtitle={`Per-pool coding-agent runner health: heartbeat-live runners versus queued and running work. A pool with queued runs and no live runner is the reason an assigned item "does nothing" — check the runner host, then Runner Pools for registration tokens and instances.`}
  dataTestId="diagnostics-runner-pools"
  onLoad={load}
  bind:loading={view.loading}
  bind:error={view.error}
  refreshInterval={15_000}
>
  {#snippet children()}
    {#if !view.loading && view.pools.length === 0}
      <Card>
        <p class="text-sm p-3" style="color: var(--ds-text-subtle);">
          No runner pools configured. Remote runners are optional — agents can also run on the server
          when it has git and docker available.
        </p>
      </Card>
    {:else}
      {#if unhealthy.length > 0}
        <Card>
          <div class="flex items-start gap-3 p-3" style="color: var(--ds-accent-red);" data-testid="runner-pools-alert">
            <IconAlertTriangle class="w-4 h-4 mt-0.5 shrink-0" />
            <span class="text-sm">
              {unhealthy.map((p) => p.name || `#${p.id}`).join(', ')}: queued runs with no live runner —
              nothing will claim this work until a runner heartbeats again.
            </span>
          </div>
        </Card>
      {/if}
      <DataTable data={view.pools} {columns} keyField="id">
        {#snippet health(p)}
          {#if !p.enabled}
            <Lozenge appearance="default" size="sm">Disabled</Lozenge>
          {:else if p.healthy}
            <span class="inline-flex items-center gap-1">
              <IconCircleCheck class="w-3.5 h-3.5" style="color: var(--ds-accent-green);" />
              <Lozenge appearance="success" size="sm">Healthy</Lozenge>
            </span>
          {:else}
            <Lozenge appearance="error" size="sm">Stalled</Lozenge>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  {/snippet}
</DiagnosticsSection>
