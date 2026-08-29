<script>
  import { Plus, RefreshCw, Search } from '@lucide/svelte';
  import { api } from '../../api.js';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { navigate } from '../../router.js';
  import DashboardItemRow from './DashboardItemRow.svelte';
  import {
    resolveDensity,
    resolveRowCount,
    rowCountToLimit,
  } from './taskWidgetState.js';

  let { workspaceId = null, config = {}, onconfigchange = null } = $props();

  let collections = $state([]);
  let workspaces = $state([]);
  let items = $state([]);
  let loadingCollections = $state(false);
  let loading = $state(false);
  let collectionError = $state(null);
  let error = $state(null);
  let collectionLoadVersion = 0;
  let itemLoadVersion = 0;
  let lastCollectionScope = null;
  let lastItemLoadKey = null;

  const selectedCollectionId = $derived(config?.collectionId ? String(config.collectionId) : '');
  const rowCount = $derived(resolveRowCount(config, 12));
  const density = $derived(resolveDensity(config));
  const fetchLimit = $derived(rowCountToLimit(rowCount));
  const collectionScope = $derived(workspaceId ? `workspace:${workspaceId}` : 'global');
  const workspaceMap = $derived(new Map(workspaces.map((workspace) => [Number(workspace.id), workspace])));
  const selectedCollection = $derived(
    collections.find((collection) => String(collection.id) === selectedCollectionId) ?? null
  );
  const collectionPickerItems = $derived.by(() => {
    if (selectedCollectionId && !selectedCollection) {
      return [
        {
          id: selectedCollectionId,
          name: t('widgets.savedSearch.collectionUnavailable'),
          unavailable: true,
        },
        ...collections,
      ];
    }
    return collections;
  });

  $effect(() => {
    if (collectionScope === lastCollectionScope) return;
    lastCollectionScope = collectionScope;
    loadCollections();
  });

  $effect(() => {
    const loadKey = `${selectedCollectionId}:${rowCount}`;
    if (loadKey === lastItemLoadKey) return;
    lastItemLoadKey = loadKey;
    loadItems();
  });

  function formatCollectionLabel(collection) {
    if (workspaceId || !collection.workspace_id) return collection.name;
    const workspace = workspaceMap.get(Number(collection.workspace_id));
    const workspaceLabel =
      workspace?.name ||
      workspace?.key ||
      t('dashboard.states.workspaceWithId', { id: collection.workspace_id });
    return `${collection.name} · ${workspaceLabel}`;
  }

  async function loadCollections() {
    const version = ++collectionLoadVersion;
    loadingCollections = true;
    collectionError = null;

    try {
      const filters = workspaceId ? { workspace_id: workspaceId } : {};
      const collectionResponse = await api.collections.getAll(filters);
      if (version !== collectionLoadVersion) return;

      collections = Array.isArray(collectionResponse) ? collectionResponse : [];
      if (!workspaceId) {
        try {
          const workspaceResponse = await api.workspaces.getAll();
          if (version === collectionLoadVersion) {
            workspaces = Array.isArray(workspaceResponse) ? workspaceResponse : [];
          }
        } catch (workspaceLoadError) {
          console.warn('Failed to load workspace names for saved search:', workspaceLoadError);
          workspaces = [];
        }
      } else {
        workspaces = [];
      }
    } catch (loadError) {
      if (version !== collectionLoadVersion) return;
      console.error('Failed to load saved search collections:', loadError);
      collections = [];
      collectionError = t('widgets.savedSearch.loadError');
    } finally {
      if (version === collectionLoadVersion) loadingCollections = false;
    }
  }

  async function loadItems() {
    const version = ++itemLoadVersion;
    if (!selectedCollectionId) {
      items = [];
      loading = false;
      error = null;
      return;
    }

    loading = true;
    error = null;

    try {
      const response = await api.items.getAll({
        collection_id: selectedCollectionId,
        limit: fetchLimit,
        order_by: 'updated_at',
        sort_direction: 'desc',
        fields: 'summary',
      });
      if (version !== itemLoadVersion) return;

      const rawItems = Array.isArray(response) ? response : response?.items ?? [];
      items = rawItems.filter((item) => item?.id);
    } catch (loadError) {
      if (version !== itemLoadVersion) return;
      if (loadError?.name === 'AbortError') return;
      console.error('Failed to load saved search items:', loadError);
      items = [];
      error = t('widgets.savedSearch.loadError');
    } finally {
      if (version === itemLoadVersion) loading = false;
    }
  }

  function handleCollectionChange(value) {
    const collectionId = value ? Number(value) : null;
    onconfigchange?.({ collectionId });
  }

  function createCollection() {
    window.dispatchEvent(new CustomEvent('show-create-modal', {
      detail: { type: 'collection', workspaceId },
    }));
  }

  function getItemKey(item) {
    if (item.workspace_key && item.workspace_item_number != null) {
      return `${item.workspace_key}-${item.workspace_item_number}`;
    }
    return `#${item.id}`;
  }

  function openItem(item) {
    navigate(`/workspaces/${item.workspace_id}/items/${item.id}`);
  }
</script>

{#snippet collectionSelector()}
  <BasePicker
    id="saved-search-collection-select"
    value={selectedCollectionId || null}
    items={collectionPickerItems}
    loading={loadingCollections}
    placeholder={t('widgets.savedSearch.selectCollection')}
    ariaLabel={t('widgets.savedSearch.selectCollection')}
    disabled={loadingCollections}
    positioning={{ strategy: 'fixed', placement: 'bottom-start', sameWidth: true }}
    searchFields={['name', 'description', (collection) => formatCollectionLabel(collection)]}
    getValue={(collection) => String(collection?.id ?? '')}
    getLabel={formatCollectionLabel}
    onSelect={(collection) => handleCollectionChange(collection?.id)}
  />
{/snippet}

<div class="saved-search-widget" data-testid="saved-search-widget">
  {#if selectedCollectionId}
    <div
      class="mb-3 flex items-center justify-between gap-3"
      data-testid="saved-search-collection-toolbar"
    >
      <div class="min-w-0 flex-1">
        {@render collectionSelector()}
      </div>
      {#if !loading && !error}
        <span class="shrink-0 text-xs" style="color: var(--ds-text-subtle);">
          {t('widgets.savedSearch.itemCount', { count: items.length })}
        </span>
      {/if}
    </div>
  {/if}

  {#if !selectedCollectionId}
    <div
      class="flex flex-col items-center gap-3 rounded-xl border border-dashed px-4 py-6 text-center"
      style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
      data-testid="saved-search-setup"
    >
      <Search class="h-7 w-7 opacity-60" />
      <div>
        <p class="text-sm font-medium" style="color: var(--ds-text);">
          {t('widgets.savedSearch.setupTitle')}
        </p>
        <p class="mt-1 text-xs">{t('widgets.savedSearch.setupSubtitle')}</p>
      </div>

      {#if loadingCollections}
        <p class="text-xs">{t('widgets.savedSearch.loadingCollections')}</p>
      {:else if collectionError}
        <div class="flex items-center gap-2 text-xs" style="color: var(--ds-status-danger-text);">
          <span>{collectionError}</span>
          <button
            type="button"
            class="inline-flex items-center gap-1 underline"
            onclick={loadCollections}
            data-testid="saved-search-retry-collections"
          >
            <RefreshCw class="h-3 w-3" />
            {t('common.retry')}
          </button>
        </div>
      {:else if collections.length === 0}
        <div class="flex flex-col items-center gap-3">
          <p class="text-xs">{t('widgets.savedSearch.noCollections')}</p>
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-[var(--ds-background-neutral-hovered)] focus:outline-none focus:ring-2 focus:ring-[var(--ds-border-focused)]"
            style="border-color: var(--ds-border); color: var(--ds-text); background-color: var(--ds-surface-raised);"
            onclick={createCollection}
            data-testid="saved-search-create-collection"
          >
            <Plus class="h-4 w-4" />
            {t('collections.newCollection')}
          </button>
        </div>
      {:else}
        <div class="w-full max-w-sm text-left">
          {@render collectionSelector()}
        </div>
      {/if}
    </div>
  {:else if loading && items.length === 0}
    <div class="space-y-2 animate-pulse" data-testid="saved-search-loading">
      {#each Array(3) as _}
        <div class="h-11 rounded" style="background-color: var(--ds-background-neutral);"></div>
      {/each}
    </div>
  {:else if error}
    <div
      class="flex flex-col items-center gap-2 rounded-xl border border-dashed px-4 py-6 text-center"
      style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
      data-testid="saved-search-error"
    >
      <Search class="h-6 w-6 opacity-60" />
      <p class="text-sm">{error}</p>
      <button
        type="button"
        class="inline-flex items-center gap-1 text-xs underline"
        onclick={loadItems}
        data-testid="saved-search-retry-items"
      >
        <RefreshCw class="h-3 w-3" />
        {t('common.retry')}
      </button>
    </div>
  {:else if items.length === 0}
    <div
      class="flex flex-col items-center rounded-xl border border-dashed px-4 py-6 text-center"
      style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
      data-testid="saved-search-empty"
    >
      <Search class="mb-2 h-6 w-6 opacity-60" />
      <p class="text-sm">{t('widgets.savedSearch.emptyTitle')}</p>
      <p class="text-xs">{t('widgets.savedSearch.emptySubtitle')}</p>
    </div>
  {:else}
    <ul class="flex flex-col gap-1.5">
      {#each items as item (item.id)}
        <li>
          <DashboardItemRow
            title={item.title}
            itemKey={getItemKey(item)}
            statusName={item.status_name}
            statusBuiltinKey={item.status_builtin_key}
            statusColor={item.status_color}
            priorityName={item.priority_name}
            priorityBuiltinKey={item.priority_builtin_key}
            priorityColor={item.priority_color}
            dueDate={item.due_date}
            {density}
            onclick={() => openItem(item)}
          />
        </li>
      {/each}
    </ul>
  {/if}
</div>
