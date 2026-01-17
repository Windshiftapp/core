<script>
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { onMount } from 'svelte';
  import { Box } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

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

  const assets = createAsyncLoader(async () => {
    if (!assetSetId) return [];
    const result = await api.assets.getAll(assetSetId, { cql: cqlQuery || undefined });
    // API returns { assets: [...], total, limit, offset }
    return result?.assets || [];
  });

  onMount(() => {
    if (assetSetId) assets.load();
  });

  // Reload if assetSetId or cqlQuery changes
  $effect(() => {
    if (assetSetId) {
      const _ = [assetSetId, cqlQuery];
      assets.load();
    }
  });
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
  searchFields={['title', 'asset_tag', 'description']}
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
</BasePicker>
