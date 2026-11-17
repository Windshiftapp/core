<script>
  import SourceBoundary from './SourceBoundary.svelte';
  import { useChartSource } from './chartSource.svelte.js';

  /**
   * A single completion percentage: the `completed_query` count over the
   * `query` count. Both totals come from the server, so a one-row sample is
   * enough and truncation does not skew the number.
   *
   * @type {{
   *   props: {
   *     query?: string,
   *     completed_query: string,
   *     label?: string,
   *     limit?: number,
   *     entity?: 'item'|'asset'
   *   }
   * }}
   */
  let { props } = $props();

  const total = useChartSource(() => ({
    entity: props.entity,
    query: props.query,
    limit: props.limit,
  }));
  const completed = useChartSource(() => ({
    entity: props.entity,
    query: props.completed_query,
    limit: props.limit,
  }));

  const totalCount = $derived(total.source?.total ?? total.source?.rows?.length ?? 0);
  const completedCount = $derived(completed.source?.total ?? completed.source?.rows?.length ?? 0);
  const percent = $derived(
    totalCount > 0
      ? Math.min(100, Math.max(0, Math.round((completedCount / totalCount) * 100)))
      : 0
  );
</script>

<SourceBoundary source={total.source} loadingRows={1} showTruncation={false}>
  {#snippet ready()}
    <SourceBoundary source={completed.source} loadingRows={1} showTruncation={false}>
      {#snippet ready()}
        <div class="flex flex-col gap-2" data-testid="openui-progress-chart">
          {#if props.label}
            <span class="text-xs text-ds-text-subtle">{props.label}</span>
          {/if}
          <div class="flex items-baseline justify-between gap-2">
            <span class="text-2xl font-semibold tabular-nums text-ds-text">{percent}%</span>
            <span class="text-xs tabular-nums text-ds-text-subtle">
              {completedCount}/{totalCount}
            </span>
          </div>
          <div
            class="h-2 w-full overflow-hidden rounded-full bg-ds-surface-raised"
            role="progressbar"
            aria-valuenow={percent}
            aria-valuemin="0"
            aria-valuemax="100"
            data-testid="openui-progress-track"
          >
            <div class="h-full rounded-full bg-ds-interactive" style="width: {percent}%;"></div>
          </div>
        </div>
      {/snippet}
    </SourceBoundary>
  {/snippet}
</SourceBoundary>
