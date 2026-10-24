<script>
  import IconAlertTriangle from '@tabler/icons-svelte-runes/icons/alert-triangle';
  import IconCircleCheck from '@tabler/icons-svelte-runes/icons/circle-check';
  import IconDatabase from '@tabler/icons-svelte-runes/icons/database';
  import IconTargetArrow from '@tabler/icons-svelte-runes/icons/target-arrow';
  import Card from '../../components/Card.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import { getFracIndexState } from '../../api/diagnostics.js';
  import DiagnosticsSection from './DiagnosticsSection.svelte';
  import { formatUtcTime, formatValue } from './format-utils.js';

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

  const healthy = $derived(view.data?.healthy === true);
  const db = $derived(view.data?.db ?? null);

  const verdict = $derived(() => {
    if (!view.data) return null;
    if (view.data.healthy) {
      return {
        tone: 'green',
        title: 'Healthy',
        body:
          'Column collation matches byte ordering and the next key the generator would produce is not present in the table — appends will succeed.',
      };
    }
    if (db?.collation_mismatch) {
      return {
        tone: 'orange',
        title: 'Column collation mismatch',
        body:
          'The frac_index column is not using COLLATE "C". The linguistic max differs from the byte-wise max, which means the generator will produce successors that already exist. Fix: ALTER TABLE items ALTER COLUMN frac_index TYPE TEXT COLLATE "C", then DROP INDEX idx_items_frac_index and recreate it.',
      };
    }
    if (db?.predicted_collision) {
      return {
        tone: 'orange',
        title: 'Predicted next-key collision',
        body: `The next append would produce "${db.predicted_next}", but that key already exists in the table. The retry path will self-correct on the first conflict, but persistent collisions point at a bug in KeyBetween or a stale duplicate row to investigate.`,
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

<DiagnosticsSection
  title="Fractional index health"
  subtitle={`Inspects the persisted state of <code>items.frac_index</code> — column collation, linguistic vs byte-wise ordering, and the next key the generator would emit. Auto-refreshes every 30s.`}
  dataTestId="diagnostics-frac-index"
  onLoad={load}
  lastRefreshed={lastRefreshed}
  bind:loading={view.loading}
  bind:error={view.error}
>
  {#snippet children()}
    {#if view.data}
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
          <IconDatabase class="w-4 h-4" />
          Database state
        </h4>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <StatCard
            icon={IconDatabase}
            label="Column collation"
            value={formatValue(db?.column_collation)}
            color={db?.collation_mismatch ? 'orange' : 'blue'}
          />
          <StatCard
            icon={IconDatabase}
            label="DB default collation"
            value={formatValue(db?.default_collation)}
            color="blue"
          />
          <StatCard
            icon={IconTargetArrow}
            label="Linguistic max"
            value={formatValue(db?.linguistic_max)}
            color={db?.collation_mismatch ? 'orange' : 'green'}
          />
          <StatCard
            icon={IconTargetArrow}
            label="Byte-wise max"
            value={formatValue(db?.byte_max)}
            color="green"
          />
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 mt-4">
          <StatCard
            icon={IconDatabase}
            label="Rows with frac_index"
            value={formatValue(db?.not_null_count)}
          />
          <StatCard
            icon={IconTargetArrow}
            label="Predicted next"
            value={formatValue(db?.predicted_next)}
            color={db?.predicted_collision ? 'orange' : 'green'}
          />
          <StatCard
            icon={IconDatabase}
            label="DB last updated"
            value={formatUtcTime(db?.last_updated_at)}
          />
        </div>
      </div>

      <div>
        <h3 class="text-sm font-semibold mb-2" style="color: var(--ds-text);">Top 10 keys (byte-ordered)</h3>
        <DataTable
          columns={top10Columns}
          data={top10Rows}
          keyField="rank"
        />
      </div>
    {/if}
  {/snippet}
</DiagnosticsSection>
