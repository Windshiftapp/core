<script>
  import Chart from '../../widgets/Chart.svelte';
  import { getSource, getWidgetDataContext } from '../context.js';
  import { itemSpec, assetSpec, sourceKey } from '../querySpec.js';
  import { groupRows } from '../aggregate.js';
  import SourceBoundary from './SourceBoundary.svelte';

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

  const buckets = $derived.by(() => {
    const rows = source?.rows ?? [];
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

<SourceBoundary {source}>
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
