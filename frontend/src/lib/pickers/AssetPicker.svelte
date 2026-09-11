<script>
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { onDestroy, untrack } from 'svelte';
  import { Box } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';
  import DescriptionText from '../components/DescriptionText.svelte';

  let {
    value = $bindable(null),
    assetSetId,
    cqlQuery = '',
    placeholder = '',
    disabled = false,
    allowClear = true,
    showUnassigned = false,
    autoOpen = false,
    multiple = false,
    optionLoader = null,
    class: className = '',
    onSelect = () => {},
    onCancel = () => {},
    onChange = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectAsset'));

  let searchQuery = $state('');
  let totalCount = $state(0);
  let externalAssets = $state([]);
  let externalLoading = $state(false);
  let externalError = $state(null);
  let opened = $state(false);
  let externalLoadToken = 0;

  // Ignore results from a row that disappeared because of scrolling,
  // pagination, a column change, or navigation while its request was active.
  onDestroy(() => {
    externalLoadToken += 1;
    assets.dispose();
  });

  const assets = createAsyncLoader(async () => {
    if (!assetSetId) return [];
    const filters = { cql: cqlQuery || undefined };
    if (searchQuery) filters.search = searchQuery;
    const result = await api.assets.getAll(assetSetId, filters);
	totalCount = result?.pagination?.total_items ?? 0;
	return result?.data || [];
  });

  // Reload when assetSetId, cqlQuery, or searchQuery changes
  $effect(() => {
    if (assetSetId) {
      const _ = [assetSetId, cqlQuery, searchQuery];
      if (!optionLoader) {
        untrack(() => assets.load());
      } else if (opened) {
        untrack(() => loadExternalOptions());
      }
    }
  });

  async function loadExternalOptions() {
    if (!optionLoader || !assetSetId) return;
    const token = ++externalLoadToken;
    externalLoading = true;
    externalError = null;
    try {
      const result = await optionLoader(searchQuery);
      if (token !== externalLoadToken) return;
      externalAssets = result?.assets || [];
      totalCount = result?.total ?? 0;
    } catch (error) {
      if (token !== externalLoadToken) return;
      externalError = error;
      externalAssets = [];
    } finally {
      if (token === externalLoadToken) externalLoading = false;
    }
  }

  function handleOpen() {
    if (!opened) {
      opened = true;
    } else if (optionLoader) {
      loadExternalOptions();
    }
  }

  const optionItems = $derived(optionLoader ? externalAssets : (assets.data || []));
  const optionLoading = $derived(optionLoader ? externalLoading : assets.loading);
  const optionError = $derived(optionLoader ? externalError : assets.error);

  function handleSearchChange(query) {
    searchQuery = query;
  }

  function assetSummary(asset) {
    if (!asset) return null;
    return {
      id: asset.id,
      title: asset.title,
      asset_tag: asset.asset_tag || ''
    };
  }

  function handleMultiChange(ids) {
    const selectedIDs = Array.isArray(ids) ? ids : [];
    const selectedAssets = selectedIDs.map((id) => {
      const asset = optionItems.find((entry) => entry.id === id);
      return assetSummary(asset) || { id };
    });
    onChange(selectedAssets);
  }
</script>

<BasePicker
  bind:value
  items={optionItems}
  loading={optionLoading}
  error={optionError}
  placeholder={resolvedPlaceholder}
  {disabled}
  {allowClear}
  {showUnassigned}
  unassignedLabel={t('common.none')}
  {multiple}
  class={className}
  serverSearch
  onOpen={handleOpen}
  onSearchChange={handleSearchChange}
  getValue={(asset) => asset?.id}
  getLabel={(asset) => {
    if (!asset) return '';
    if (asset.asset_tag) return `${asset.asset_tag} - ${asset.title}`;
    return asset.title;
  }}
  onSelect={onSelect}
  onCancel={onCancel}
  onChange={handleMultiChange}
>
  {#snippet itemSnippet({ item: asset, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <div class="w-8 h-8 rounded flex items-center justify-center flex-shrink-0"
           style="background: var(--ds-background-neutral);">
        <Box size={16} style="color: var(--ds-text-subtle);" />
      </div>
      <div class="flex flex-col min-w-0 flex-1">
        <span class="font-medium truncate">{asset.title}</span>
        <span class="text-xs truncate" style="color: var(--ds-text-subtle);">
          {asset.asset_tag || t('pickers.noTag')}
          {#if asset.asset_type_name} · {asset.asset_type_name}{/if}
        </span>
      </div>
    </div>
  {/snippet}

  {#snippet noResultsSnippet({ searchQuery: sq })}
    <div class="px-4 py-4 text-center text-sm" style="color: var(--ds-text-subtle);">
      {t('pickers.noResultsFor', { query: sq })}
    </div>
  {/snippet}
</BasePicker>

{#if !searchQuery && totalCount > optionItems.length}
  <DescriptionText as="div" variant="subtlest">
    {t('pickers.showingOfTotal', { shown: optionItems.length, total: totalCount })}
  </DescriptionText>
{/if}
