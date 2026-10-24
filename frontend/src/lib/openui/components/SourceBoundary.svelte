<script>
  import { t } from '../../stores/i18n.svelte.js';

  /**
   * Renders the loading, error, and truncation states around a widget data
   * source, and hands ready rows to the `ready` snippet.
   *
   * @type {{
   *   source: import('../context.js').SourceResult | null,
   *   loadingRows?: number,
   *   showTruncation?: boolean,
   *   ready: import('svelte').Snippet<[Array<object>]>,
   *   empty?: import('svelte').Snippet
   * }}
   */
  let { source, loadingRows = 3, showTruncation = true, ready, empty } = $props();

  const rows = $derived(source?.rows ?? []);
  const truncated = $derived(Boolean(source?.truncated) && Number(source?.total) > rows.length);
</script>

{#if !source || source.status === 'loading'}
  <div class="space-y-2" data-testid="openui-source-loading">
    {#each Array.from({ length: loadingRows }) as _, index (index)}
      <div class="h-4 animate-pulse rounded" style="background-color: var(--ds-surface-raised);"></div>
    {/each}
  </div>
{:else if source.status === 'error'}
  <div
    class="rounded-md px-3 py-2 text-sm"
    style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
    data-testid="openui-source-error"
  >
    {source.error?.message || t('dashboard.customWidgets.sourceError')}
  </div>
{:else if rows.length === 0 && empty}
  {@render empty()}
{:else}
  {@render ready(rows)}
  {#if showTruncation && truncated}
    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);" data-testid="openui-truncation-badge">
      {t('dashboard.customWidgets.truncated', {
        shown: rows.length,
        total: source.total,
      })}
    </p>
  {/if}
{/if}
