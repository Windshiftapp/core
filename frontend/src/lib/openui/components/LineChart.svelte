<script>
  import Chart from '../../widgets/Chart.svelte';
  import { alignBuckets, bucketByDate } from '../aggregate.js';
  import SourceBoundary from './SourceBoundary.svelte';
  import { useChartSource } from './chartSource.svelte.js';

  /**
   * @type {{
   *   props: {
   *     query?: string,
   *     sort?: string,
   *     limit?: number,
   *     field?: string,
   *     bucket?: 'day'|'week'|'month',
   *     metric?: 'count'|'sum',
   *     entity?: 'item'|'asset',
   *     label?: string,
   *     series2?: {
   *       query?: string,
   *       field?: string,
   *       metric?: 'count'|'sum',
   *       label?: string,
   *       color?: string,
   *       dashed?: boolean,
   *       sort?: string,
   *       limit?: number
   *     }
   *   }
   * }}
   */
  let { props } = $props();

  const chart = useChartSource(() => props);
  // Always resolved so the hook order is stable. With no series2 the spec
  // matches the chart's, so it reuses the same source instead of fetching.
  const second = useChartSource(() => ({
    entity: props.entity,
    query: props.series2?.query ?? props.query,
    sort: props.series2?.sort ?? props.sort,
    limit: props.series2?.limit ?? props.limit,
  }));

  const dateField = $derived(props.field ?? 'created_at');
  const secondField = $derived(props.series2?.field ?? dateField);

  const buckets = $derived.by(() =>
    bucketByDate(chart.source?.rows ?? [], dateField, props.bucket ?? 'week', {
      metric: props.metric ?? 'count',
      sumField: dateField,
    })
  );
  const secondBuckets = $derived.by(() => {
    if (!props.series2) return [];
    return bucketByDate(second.source?.rows ?? [], secondField, props.bucket ?? 'week', {
      metric: props.series2.metric ?? props.metric ?? 'count',
      sumField: secondField,
    });
  });

  // Both lines share one x-axis: a bucket only one series has still gets a
  // zero on the other, so the points line up by date instead of by index.
  const axis = $derived.by(() => alignBuckets([buckets, secondBuckets]));

  const categories = $derived(
    axis.labels.map((label) => {
      const date = new Date(label);
      return `${String(date.getMonth() + 1).padStart(2, '0')}/${String(date.getDate()).padStart(2, '0')}`;
    })
  );
  const series = $derived([
    {
      key: 'value',
      label: props.label ?? (props.metric === 'sum' ? dateField : 'Count'),
      color: '#6366f1',
      values: axis.values[0] ?? [],
    },
    ...(props.series2
      ? [
          {
            key: 'value2',
            label: props.series2.label ?? (props.series2.metric === 'sum' ? secondField : 'Count'),
            color: props.series2.color ?? '#22c55e',
            values: axis.values[1] ?? [],
            dashed: props.series2.dashed ?? false,
          },
        ]
      : []),
  ]);
</script>

<SourceBoundary source={chart.source}>
  {#snippet ready()}
    {#if props.series2}
      <SourceBoundary source={second.source}>
        {#snippet ready()}
          <Chart type="line" {series} {categories} minHeight={160} showYAxis={false} />
        {/snippet}
      </SourceBoundary>
    {:else}
      <Chart type="line" {series} {categories} minHeight={160} showYAxis={false} />
    {/if}
  {/snippet}
</SourceBoundary>
