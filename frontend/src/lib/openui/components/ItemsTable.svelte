<script>
  import ItemKey from '../../components/ItemKey.svelte';
  import Link from '../../components/Link.svelte';
  import { getSource, getWidgetDataContext } from '../context.js';
  import { itemSpec, assetSpec, sourceKey, fieldValue, formatValue, parseColumns } from '../querySpec.js';
  import SourceBoundary from './SourceBoundary.svelte';

  /**
   * @type {{
   *   props: {
   *     query?: string,
   *     sort?: string,
   *     limit?: number,
   *     columns?: string,
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
  /**
   * Deep link to the row's item. Only items have one, and only when the server
   * sent the workspace; anything else stays plain text.
   */
  function itemHref(row) {
    if (entity !== 'item') return null;
    // Coerced, so only digits can ever reach the path.
    const id = Number(row?.id);
    const workspaceId = Number(row?.workspace_id);
    if (!Number.isInteger(id) || id <= 0) return null;
    if (!Number.isInteger(workspaceId) || workspaceId <= 0) return null;
    return `/workspaces/${workspaceId}/items/${id}`;
  }

  function openRow(event, row) {
    if (!context?.openItem) return;
    event?.preventDefault?.();
    context.openItem(Number(row.id));
  }

  const columns = $derived(
    parseColumns(props.columns ?? (entity === 'asset' ? 'key,title,status,type' : 'key,title,status,assignee,due_date'))
  );
</script>

<SourceBoundary {source}>
  {#snippet ready(rows)}
    <div class="overflow-x-auto">
      <table class="w-full text-sm" data-testid="openui-items-table">
        <thead>
          <tr style="color: var(--ds-text-subtle);">
            {#each columns as column (column)}
              <th class="px-2 py-1 text-left text-xs font-medium">{column}</th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each rows as row, index (row.id ?? index)}
            <tr style="border-top: 1px solid var(--ds-border);">
              {#each columns as column (column)}
                <td class="px-2 py-1" style="color: var(--ds-text);">
                  {#if itemHref(row) && column === 'key'}
                    <ItemKey
                      itemKey={row.key ?? String(row.id)}
                      href={itemHref(row)}
                      onClick={(event) => openRow(event, row)}
                      size="compact"
                    />
                  {:else if itemHref(row) && column === 'title'}
                    <Link href={itemHref(row)} onClick={(event) => openRow(event, row)} class="hover:underline">
                      {row.title}
                    </Link>
                  {:else}
                    {formatValue(fieldValue(row, column))}
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/snippet}
</SourceBoundary>
