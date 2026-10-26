<script>
  import { formatValue } from '../querySpec.js';
  import SourceBoundary from './SourceBoundary.svelte';
  import { useChartSource } from './chartSource.svelte.js';

  /**
   * @type {{ props: { query?: string, sort?: string, limit?: number, entity?: 'item'|'asset' } }}
   */
  let { props } = $props();

  const chart = useChartSource(() => props, { defaultSort: '-updated_at' });
</script>

<SourceBoundary source={chart.source}>
  {#snippet ready(rows)}
    <ol class="space-y-2" data-testid="openui-activity-feed">
      {#each rows as row, index (row.id ?? index)}
        <li class="flex items-start gap-2">
          <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full" style="background-color: var(--ds-accent, #6366f1);"></span>
          <div class="min-w-0">
            <button
              type="button"
              class="block truncate text-left text-sm hover:underline"
              style="color: var(--ds-text);"
              onclick={() => chart.context?.openItem?.(Number(row.id))}
            >
              {row.title}
            </button>
            <span class="text-xs" style="color: var(--ds-text-subtle);">
              {formatValue(row.updated_at ?? row.created_at)}
            </span>
          </div>
        </li>
      {/each}
    </ol>
  {/snippet}
</SourceBoundary>
