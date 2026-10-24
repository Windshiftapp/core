<script>
  import { customWidgets } from '../../api/customWidgets.js';
  import { loadCustomWidgetRenderer } from '../../openui/render.js';
  import { assetSpec, itemSpec, linksSpec, sourceKey } from '../../openui/querySpec.js';
  import { itemModalStore } from '../../stores/itemModalStore.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { config = {}, refreshKey = 0, title = '' } = $props();

  const widgetId = $derived(Number(config?.customWidgetId) || 0);

  let widget = $state(null);
  let unavailable = $state(false);
  let unconfigured = $state(false);
  let definitionError = $state(false);
  let Renderer = $state(null);
  let sources = $state({});

  /** Slots in flight, keyed by source key, so parallel components share one request. */
  const pending = new Map();

  function slotsFromQueries(queries) {
    const slots = [];
    if (queries?.item) {
      const spec = itemSpec({
        query: queries.item.ql,
        sort: queries.item.sort,
        limit: queries.item.limit,
      });
      slots.push({
        key: sourceKey(spec),
        entity: 'item',
        ql: queries.item.ql ?? '',
        sort: queries.item.sort ?? '',
        limit: queries.item.limit ?? 0,
      });
    }
    if (queries?.asset) {
      const spec = assetSpec({
        query: queries.asset.ql,
        sort: queries.asset.sort,
        limit: queries.asset.limit,
        assetSetId: queries.asset.asset_set_id,
      });
      slots.push({
        key: sourceKey(spec),
        entity: 'asset',
        ql: queries.asset.ql ?? '',
        sort: queries.asset.sort ?? '',
        limit: queries.asset.limit ?? 0,
        asset_set_id: queries.asset.asset_set_id,
      });
    }
    if (queries?.links) {
      const links = queries.links;
      const spec = linksSpec({
        itemQuery: links.item?.ql,
        itemSort: links.item?.sort,
        itemLimit: links.item?.limit,
        assetQuery: links.asset?.ql,
        assetSort: links.asset?.sort,
        assetLimit: links.asset?.limit,
        assetSetId: links.asset?.asset_set_id,
        maxEdges: links.max_edges,
      });
      slots.push({ key: sourceKey(spec), entity: 'links', links });
    }
    return slots;
  }

  /**
   * Resolve one spec on demand. Prefetching covers the stored slots; this path
   * covers a component whose props produced a spec the prefetch did not know
   * about, so the widget still renders instead of hanging.
   */
  function requestSource(spec, key) {
    if (!key || sources[key] || pending.has(key)) return;
    sources[key] = { status: 'loading' };
    const request = customWidgets
      .data({ widget_id: widgetId, slots: [toSlot(spec, key)] })
      .then((response) => {
        sources[key] = response?.results?.[key] ?? {
          status: 'error',
          error: { code: 'missing', message: t('dashboard.customWidgets.sourceError') },
        };
      })
      .catch(() => {
        sources[key] = {
          status: 'error',
          error: { code: 'unavailable', message: t('dashboard.customWidgets.sourceError') },
        };
      })
      .finally(() => pending.delete(key));
    pending.set(key, request);
  }

  function toSlot(spec, key) {
    if (spec.entity === 'links') return { key, entity: 'links', links: spec.links };
    const slot = { key, entity: spec.entity, ql: spec.ql ?? '', sort: spec.sort ?? '', limit: spec.limit ?? 0 };
    if (spec.entity === 'asset') slot.asset_set_id = spec.asset_set_id;
    return slot;
  }

  function assetSetIdFromQueries(queries) {
    return queries?.asset?.asset_set_id ?? null;
  }

  $effect(() => {
    // refreshKey changes after the builder edits this widget.
    void refreshKey;
    if (!widgetId) {
      // The builder sets config.customWidgetId when it creates the widget.
      unconfigured = true;
      return;
    }
    let cancelled = false;
    customWidgets
      .get(widgetId)
      .then(async (loaded) => {
        if (cancelled) return;
        widget = loaded;
        Renderer = await loadCustomWidgetRenderer();
        if (cancelled) return;

        const slots = slotsFromQueries(loaded.queries);
        if (slots.length === 0) return;
        const response = await customWidgets.data({ widget_id: widgetId, slots });
        if (cancelled) return;
        sources = response?.results ?? {};
      })
      .catch((error) => {
        if (cancelled) return;
        if (error?.status === 404) {
          unavailable = true;
          return;
        }
        definitionError = true;
      });
    return () => {
      cancelled = true;
    };
  });
</script>

{#if unconfigured}
  <p class="py-4 text-sm" style="color: var(--ds-text-subtle);" data-testid="custom-widget-unconfigured">
    {t('dashboard.customWidgets.notConfigured')}
  </p>
{:else if unavailable}
  <p class="py-4 text-sm" style="color: var(--ds-text-subtle);" data-testid="custom-widget-unavailable">
    {t('dashboard.customWidgets.unavailable')}
  </p>
{:else if definitionError}
  <p class="py-4 text-sm" style="color: var(--ds-text-subtle);" data-testid="custom-widget-error">
    {t('dashboard.customWidgets.sourceError')}
  </p>
{:else if widget && Renderer}
  <Renderer
    program={widget.program}
    tileTitle={title || widget.name || ''}
    {sources}
    assetSetId={assetSetIdFromQueries(widget.queries)}
    {requestSource}
    openItem={(itemId) => itemModalStore.open(itemId, null)}
  />
{:else}
  <div class="space-y-2" data-testid="custom-widget-loading">
    {#each [0, 1, 2] as row (row)}
      <div class="h-4 animate-pulse rounded" style="background-color: var(--ds-surface-raised);"></div>
    {/each}
  </div>
{/if}
