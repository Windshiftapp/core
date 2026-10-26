<script>
  import { formatValue } from '../querySpec.js';
  import SourceBoundary from './SourceBoundary.svelte';
  import { useChartSource } from './chartSource.svelte.js';

  /**
   * @type {{ props: { query?: string, sort?: string, limit?: number, entity?: 'item'|'asset' } }}
   */
  let { props } = $props();

  const chart = useChartSource(() => props);
</script>

<SourceBoundary source={chart.source}>
  {#snippet ready(rows)}
    <ul class="divide-y" style="border-color: var(--ds-border);" data-testid="openui-item-list">
      {#each rows as row, index (row.id ?? index)}
        <li class="flex items-center justify-between gap-3 py-1.5">
          <button
            type="button"
            class="truncate text-left text-sm hover:underline"
            style="color: var(--ds-text);"
            onclick={() => chart.context?.openItem?.(Number(row.id))}
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
