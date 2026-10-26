<script>
  import { fieldValue } from '../querySpec.js';
  import { useChartSource } from './chartSource.svelte.js';

  /**
   * A single number derived from a query. `count` uses the server total, so it
   * is exact even when the row sample is truncated; `sum` adds a numeric field
   * over the fetched rows and is therefore subject to the same truncation.
   *
   * @type {{
   *   props: {
   *     label: string,
   *     query?: string,
   *     aggregate?: 'count'|'sum',
   *     field?: string,
   *     entity?: 'item'|'asset',
   *     sort?: string,
   *     limit?: number
   *   }
   * }}
   */
  let { props } = $props();

  const chart = useChartSource(() => props);

  const value = $derived.by(() => {
    const source = chart.source;
    if (!source || source.status !== 'ready') return null;
    if ((props.aggregate ?? 'count') === 'count') {
      return source.total ?? source.rows?.length ?? 0;
    }
    return (source.rows ?? []).reduce((sum, row) => {
      const numeric = Number(fieldValue(row, props.field));
      return sum + (Number.isFinite(numeric) ? numeric : 0);
    }, 0);
  });
</script>

<div class="flex flex-col gap-0.5" data-testid="openui-metric">
  <span class="text-xs" style="color: var(--ds-text-subtle);">{props.label}</span>
  {#if chart.source?.status === 'error'}
    <span class="text-lg font-semibold" style="color: var(--ds-text-subtle);">—</span>
  {:else if value === null}
    <span class="h-7 w-12 animate-pulse rounded" style="background-color: var(--ds-surface-raised);"></span>
  {:else}
    <span class="text-xl font-semibold" style="color: var(--ds-text);">{value}</span>
  {/if}
</div>
