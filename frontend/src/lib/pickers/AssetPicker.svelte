<script>
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { untrack } from 'svelte';
  import { Box } from 'lucide-svelte';
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
    class: className = '',
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectAsset'));

  let searchQuery = $state('');
  let totalCount = $state(0);

  const assets = createAsyncLoader(async () => {
    if (!assetSetId) return [];
    const filters = { cql: cqlQuery || undefined };
    if (searchQuery) filters.search = searchQuery;
    const result = await api.assets.getAll(assetSetId, filters);
    // API returns { assets: [...], total, limit, offset }
    totalCount = result?.total ?? 0;
    return result?.assets || [];
  });

  // Reload when assetSetId, cqlQuery, or searchQuery changes
  $effect(() => {
    if (assetSetId) {
      const _ = [assetSetId, cqlQuery, searchQuery];
      untrack(() => assets.load());
    }
  });

  function handleSearchChange(query) {
    searchQuery = query;
  }
</script>

<BasePicker
  bind:value
  items={assets.data || []}
  loading={assets.loading}
  error={assets.error}
  placeholder={resolvedPlaceholder}
  {disabled}
  {allowClear}
  {showUnassigned}
  unassignedLabel={t('common.none')}
  class={className}
  serverSearch
  onSearchChange={handleSearchChange}
  getValue={(asset) => asset?.id}
  getLabel={(asset) => {
    if (!asset) return '';
    if (asset.asset_tag) return `${asset.asset_tag} - ${asset.title}`;
    return asset.title;
  }}
  onSelect={onSelect}
  onCancel={onCancel}
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

{#if !searchQuery && totalCount > (assets.data?.length || 0)}
  <DescriptionText as="div" variant="subtlest">
    {t('pickers.showingOfTotal', { shown: assets.data?.length || 0, total: totalCount })}
  </DescriptionText>
{/if}
