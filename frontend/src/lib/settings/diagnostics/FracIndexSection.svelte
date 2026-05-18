<script>
  import { onDestroy, onMount } from 'svelte';
  import {
    IconRefresh,
    IconAlertTriangle,
    IconCircleCheck,
    IconDatabase,
    IconBrain,
    IconTargetArrow,
  } from '@tabler/icons-svelte-runes';
  import Card from '../../components/Card.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import { getFracIndexState } from '../../api/diagnostics.js';

  /** @type {{loading: boolean, error: string|null, data: any|null}} */
  let view = $state({ loading: true, error: null, data: null });
  let lastRefreshed = $state(null);

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const data = await getFracIndexState();
      view = { loading: false, error: null, data };
      lastRefreshed = new Date();
    } catch (err) {
      view = { ...view, loading: false, error: err?.message ?? String(err) };
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

  function fmtVal(v) {
    if (v === null || v === undefined || v === '') return '—';
    return String(v);
  }

  function fmtTime(d) {
    if (!d) return '—';
    return d.toISOString().replace('T', ' ').replace(/\..*Z$/, ' UTC');
  }

  const healthy = $derived(view.data?.healthy === true);
  const cache = $derived(view.data?.cache ?? null);
  const db = $derived(view.data?.db ?? null);

  const hitRate = $derived(() => {
    if (!cache) return '—';
    const total = (cache.hits ?? 0) + (cache.misses ?? 0);
    if (total === 0) return '—';
    return `${Math.round((100 * cache.hits) / total)}%`;
  });

  const verdict = $derived(() => {
    if (!view.data) return null;
    if (view.data.healthy) {
      return {
        tone: 'green',
        title: 'Healthy',
        body:
          'In-memory cache and database agree. The next generator output is not present in the table — INSERTs will succeed.',
      };
    }
    if (db?.collation_mismatch) {
      return {
        tone: 'orange',
        title: 'Column collation mismatch',
        body:
          'The frac_index column is not using COLLATE "C". The linguistic max differs from the byte-wise max, which means the algorithm will produce successors that already exist. Fix: ALTER TABLE items ALTER COLUMN frac_index TYPE TEXT COLLATE "C", then DROP INDEX idx_items_frac_index and recreate it.',
      };
    }
    if (db?.predicted_collision) {
      return {
        tone: 'orange',
        title: 'Generator cache is poisoned',
        body: `The cache will hand out "${db.predicted_collision}" on the next create, but that key already exists in the table. Restart the server to clear the cache; the retry path will also self-correct on the first conflict.`,
      };
    }
    return {
      tone: 'orange',
      title: 'Unhealthy — cause unclear',
      body: 'Health is false but no specific cause was identified. Inspect the raw values below.',
    };
  });

  const top10Columns = [
    { key: 'rank', label: '#', render: (row) => row.rank, align: 'text-right', textColor: 'var(--ds-text-subtle)' },
    { key: 'value', label: 'frac_index', render: (row) => row.value },
  ];
  const top10Rows = $derived(
    (db?.top_10_by_byte ?? []).map((v, i) => ({ rank: i + 1, value: v }))
  );
</script>

<section class="space-y-6" data-testid="diagnostics-frac-index">
  <div class="flex items-start justify-between gap-4">
    <div>
      <h3 class="text-base font-semibold" style="color: var(--ds-text);">Fractional index health</h3>
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        Inspects the generator that assigns <code>items.frac_index</code> on create / reorder. Auto-refreshes every 30s.
      </p>
    </div>
    <button
      type="button"
      class="inline-flex items-center gap-1.5 text-sm px-2.5 py-1.5 rounded-md transition-colors"
      style="color: var(--ds-text-subtle); background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border-subtle);"
      onclick={load}
      disabled={view.loading}
      data-testid="frac-index-refresh"
    >
      <IconRefresh class="w-4 h-4" />
      <span>{view.loading ? 'Loading…' : 'Refresh'}</span>
    </button>
  </div>

  {#if view.error}
    <Card>
      <div class="flex items-start gap-3 p-3" style="color: var(--ds-accent-red);">
        <IconAlertTriangle class="w-5 h-5 flex-shrink-0 mt-0.5" />
        <div>
          <div class="font-semibold">Failed to load diagnostics</div>
          <div class="text-sm" style="color: var(--ds-text-subtle);">{view.error}</div>
        </div>
      </div>
    </Card>
  {:else if view.data}
    {#if verdict()}
      <Card>
        <div
          class="flex items-start gap-3 p-4"
          data-testid="frac-index-verdict"
          data-verdict={healthy ? 'healthy' : 'unhealthy'}
        >
          {#if healthy}
            <IconCircleCheck class="w-6 h-6 flex-shrink-0 mt-0.5" style="color: var(--ds-accent-green);" />
          {:else}
            <IconAlertTriangle class="w-6 h-6 flex-shrink-0 mt-0.5" style="color: var(--ds-accent-orange);" />
          {/if}
          <div>
            <div class="font-semibold" style="color: var(--ds-text);">{verdict().title}</div>
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{verdict().body}</div>
          </div>
        </div>
      </Card>
    {/if}

    <div>
      <h4 class="text-sm font-semibold mb-3 flex items-center gap-1.5" style="color: var(--ds-text);">
        <IconBrain class="w-4 h-4" />
        In-memory generator cache
      </h4>
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard icon={IconBrain} label="Cached key" value={fmtVal(cache?.cached)} color="blue" />
        <StatCard
          icon={IconTargetArrow}
          label="Next would be"
          value={fmtVal(cache?.next_would_be)}
          color={db?.predicted_collision ? 'orange' : 'blue'}
        />
        <StatCard icon={IconCircleCheck} label="Cache hits" value={fmtVal(cache?.hits)} color="green" />
        <StatCard icon={IconRefresh} label="Cache misses / hit rate" value={`${fmtVal(cache?.misses)} · ${hitRate()}`} color="purple" />
      </div>
      {#if cache?.next_error}
        <p class="text-sm mt-2" style="color: var(--ds-accent-red);">
          KeyBetween error on cached value: {cache.next_error}
        </p>
      {/if}
    </div>

    <div>
      <h4 class="text-sm font-semibold mb-3 flex items-center gap-1.5" style="color: var(--ds-text);">
        <IconDatabase class="w-4 h-4" />
        Database state
      </h4>
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          icon={IconDatabase}
          label="Column collation"
          value={fmtVal(db?.column_collation)}
          color={db?.collation_mismatch ? 'orange' : 'blue'}
        />
        <StatCard
          icon={IconDatabase}
          label="DB default collation"
          value={fmtVal(db?.default_collation)}
          color="blue"
        />
        <StatCard
          icon={IconTargetArrow}
          label="Linguistic max"
          value={fmtVal(db?.linguistic_max)}
          color={db?.collation_mismatch ? 'orange' : 'green'}
        />
        <StatCard
          icon={IconTargetArrow}
          label="Byte-wise max"
          value={fmtVal(db?.byte_max)}
          color="green"
        />
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mt-4">
        <StatCard
          icon={IconDatabase}
          label="Rows with frac_index"
          value={fmtVal(db?.not_null_count)}
          color="blue"
        />
        <StatCard
          icon={db?.predicted_collision ? IconAlertTriangle : IconCircleCheck}
          label="Predicted collision"
          value={fmtVal(db?.predicted_collision) === '—' ? 'none' : `would hit ${db.predicted_collision}`}
          color={db?.predicted_collision ? 'orange' : 'green'}
        />
      </div>
    </div>

    <div>
      <div class="flex items-baseline justify-between mb-2">
        <h4 class="text-sm font-semibold" style="color: var(--ds-text);">Top 10 frac_index values (byte order)</h4>
        <span class="text-xs" style="color: var(--ds-text-subtle);">Last refreshed {fmtTime(lastRefreshed)}</span>
      </div>
      <DataTable
        columns={top10Columns}
        data={top10Rows}
        keyField="rank"
        emptyMessage="No items with a non-null frac_index."
      />
    </div>
  {/if}
</section>
