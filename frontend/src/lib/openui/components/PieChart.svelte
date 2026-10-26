<script>
  import Chart from '../../widgets/Chart.svelte';
  import { groupRows } from '../aggregate.js';
  import SourceBoundary from './SourceBoundary.svelte';
  import { useChartSource } from './chartSource.svelte.js';

  /**
   * Share-of-total as a pie: one slice per distinct value of `group_by`.
   *
   * @type {{
   *   props: {
   *     query?: string,
   *     sort?: string,
   *     limit?: number,
   *     group_by: string,
   *     metric?: 'count'|'sum',
   *     field?: string,
   *     group_limit?: number,
   *     donut?: boolean,
   *     link_mode?: 'fanout'|'primary',
   *     entity?: 'item'|'asset'
   *   }
   * }}
   */
  let { props } = $props();

  const chart = useChartSource(() => props);

  const buckets = $derived.by(() => {
    const rows = chart.source?.rows ?? [];
    if (rows.length === 0) return [];
    return groupRows(rows, {
      groupBy: props.group_by,
      metric: props.metric ?? 'count',
      field: props.field,
      mode: props.link_mode ?? 'fanout',
      limit: props.group_limit ?? 8,
    });
  });

  const slices = $derived(
    buckets.map((bucket) => ({ label: bucket.label, value: bucket.value }))
  );
</script>

<SourceBoundary source={chart.source}>
  {#snippet ready()}
    <Chart
      type="pie"
      {slices}
      minHeight={200}
      maxHeight={240}
      maxSlices={props.group_limit ?? 8}
      donut={props.donut === true}
    />
  {/snippet}
</SourceBoundary>
