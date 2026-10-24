<script>
  import Chart from '../../widgets/Chart.svelte';
  import { getSource, getWidgetDataContext } from '../context.js';
  import { itemSpec, assetSpec, sourceKey } from '../querySpec.js';
  import { bucketByDate } from '../aggregate.js';
  import SourceBoundary from './SourceBoundary.svelte';

  /**
   * @type {{
   *   props: {
   *     query?: string,
   *     sort?: string,
   *     limit?: number,
   *     field?: string,
   *     bucket?: 'day'|'week'|'month',
   *     metric?: 'count'|'sum',
   *     entity?: 'item'|'asset'
   *   }
   * }}
   */
  let { props } = $props();

  const context = getWidgetDataContext();
  const entity = $derived(props.entity === 'asset' ? 'asset' : 'item');
  const spec = $derived(
    entity === 'asset'
      ? assetSpec({
          query: props.query,
          sort: props.sort,
          limit: props.limit,
          assetSetId: context?.assetSetId,
        })
      : itemSpec({ query: props.query, sort: props.sort, limit: props.limit })
  );
  const source = $derived(getSource(sourceKey(spec)));

  $effect(() => {
    if (source) return;
    context?.requestSource?.(spec, sourceKey(spec));
  });
  const dateField = $derived(props.field ?? 'created_at');

  const buckets = $derived.by(() =>
    bucketByDate(source?.rows ?? [], dateField, props.bucket ?? 'week', {
      metric: props.metric ?? 'count',
      sumField: dateField,
    })
  );

  const categories = $derived(
    buckets.map((bucket) => {
      const date = new Date(bucket.label);
      return `${String(date.getMonth() + 1).padStart(2, '0')}/${String(date.getDate()).padStart(2, '0')}`;
    })
  );
  const series = $derived([
    {
      key: 'value',
      label: props.metric === 'sum' ? dateField : 'Count',
      color: '#6366f1',
      values: buckets.map((bucket) => bucket.value),
    },
  ]);
</script>

<SourceBoundary {source}>
  {#snippet ready()}
    <Chart type="line" {series} {categories} minHeight={160} showYAxis={false} />
  {/snippet}
</SourceBoundary>
