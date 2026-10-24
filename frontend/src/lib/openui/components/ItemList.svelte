<script>
  import { getSource, getWidgetDataContext } from '../context.js';
  import { itemSpec, assetSpec, sourceKey, formatValue } from '../querySpec.js';
  import SourceBoundary from './SourceBoundary.svelte';

  /**
   * @type {{ props: { query?: string, sort?: string, limit?: number, entity?: 'item'|'asset' } }}
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
</script>

<SourceBoundary {source}>
  {#snippet ready(rows)}
    <ul class="divide-y" style="border-color: var(--ds-border);" data-testid="openui-item-list">
      {#each rows as row, index (row.id ?? index)}
        <li class="flex items-center justify-between gap-3 py-1.5">
          <button
            type="button"
            class="truncate text-left text-sm hover:underline"
            style="color: var(--ds-text);"
            onclick={() => context?.openItem?.(Number(row.id))}
          >
            {row.title}
          </button>
          <span class="shrink-0 text-xs" style="color: var(--ds-text-subtle);">
            {formatValue(row.status ?? row.key)}
          </span>
        </li>
      {/each}
    </ul>
  {/snippet}
</SourceBoundary>
