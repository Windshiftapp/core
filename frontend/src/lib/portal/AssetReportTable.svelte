<script>
  import { onMount } from 'svelte';
  import { X, Table2, ChevronLeft, ChevronRight, Loader2, AlertCircle, Package } from 'lucide-svelte';
  import { api } from '../api.js';
  import { portalStore, iconMap } from '../stores/portal.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    report,
    slug,
    sectionId,
    isEditing = false,
    onRemove = () => {}
  } = $props();

  // State
  let assets = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let page = $state(1);
  let pageSize = $state(10);
  let totalCount = $state(0);
  let totalPages = $state(0);

  // Computed columns - use column_config from report or defaults
  let displayColumns = $derived(() => {
    if (report.column_config && report.column_config.length > 0) {
      return report.column_config;
    }
    return ['title', 'asset_tag', 'status'];
  });

  // Load assets from execute endpoint
  async function loadAssets() {
    if (!slug || !report?.id) return;

    try {
      loading = true;
      error = null;

      const result = await api.assetReports.execute(slug, report.id, { page, pageSize });

      assets = result.assets || [];
      totalCount = result.total || 0;
      totalPages = result.total_pages || Math.ceil(totalCount / pageSize);
    } catch (err) {
      console.error('Failed to load asset report:', err);
      error = err.message || 'Failed to load assets';
      assets = [];
    } finally {
      loading = false;
    }
  }

  // Reload when page changes
  $effect(() => {
    page;
    loadAssets();
  });

  onMount(() => {
    loadAssets();
  });

  function nextPage() {
    if (page < totalPages) {
      page++;
    }
  }

  function prevPage() {
    if (page > 1) {
      page--;
    }
  }

  // Get column header label
  function getColumnLabel(col) {
    const labels = {
      title: t('common.name'),
      asset_tag: t('assets.assetTag'),
      status: t('common.status'),
      serial_number: t('assets.serialNumber'),
      description: t('common.description'),
      category: t('common.category'),
      type: t('common.type')
    };
    // Check for custom fields (cf_ prefix)
    if (col.startsWith('cf_')) {
      return col.replace('cf_', '').replace(/_/g, ' ');
    }
    return labels[col] || col;
  }

  // Get cell value for a column
  function getCellValue(asset, col) {
    // Handle standard fields
    if (col === 'status') {
      return asset.status?.name || '-';
    }
    if (col === 'category') {
      return asset.category?.name || '-';
    }
    if (col === 'type') {
      return asset.type?.name || '-';
    }
    // Handle custom fields
    if (col.startsWith('cf_')) {
      const fieldName = col.replace('cf_', '');
      const cfValue = asset.custom_fields?.[fieldName];
      if (cfValue === null || cfValue === undefined) return '-';
      return String(cfValue);
    }
    // Direct field access
    return asset[col] ?? '-';
  }

  // Get icon component
  const IconComponent = $derived(iconMap[report.icon] || Table2);
</script>

<div
  class="w-full rounded border transition-shadow relative group"
  style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
>
  <!-- Header -->
  <div class="flex items-center justify-between p-4 border-b" style="border-color: var(--ds-border);">
    <div class="flex items-center gap-3">
      <div
        class="w-10 h-10 rounded flex items-center justify-center"
        style="background-color: {report.color || '#6b7280'};"
      >
        <svelte:component this={IconComponent} size={20} color="white" />
      </div>
      <div>
        <h3 class="font-medium flex items-center gap-2" style="color: var(--ds-text);">
          {report.name}
          {#if !report.is_active}
            <span
              class="px-1.5 py-0.5 text-[10px] font-medium rounded"
              style="background-color: {portalStore.isDarkMode ? 'rgba(156, 163, 175, 0.2)' : '#f3f4f6'}; color: {portalStore.isDarkMode ? '#9ca3af' : '#6b7280'};"
            >
              INACTIVE
            </span>
          {/if}
        </h3>
        {#if report.description}
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {report.description}
          </p>
        {/if}
      </div>
    </div>

    {#if isEditing}
      <button
        onclick={() => onRemove(report.id)}
        class="p-2 rounded transition-opacity opacity-0 group-hover:opacity-100"
        style="background-color: {portalStore.isDarkMode ? 'rgba(220, 38, 38, 0.1)' : '#fee2e2'}; color: #dc2626;"
        title={t('portal.removeFromSection')}
      >
        <X class="w-4 h-4" />
      </button>
    {/if}
  </div>

  <!-- Table Content -->
  <div class="overflow-x-auto">
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
        <span class="ml-2 text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</span>
      </div>
    {:else if error}
      <div class="flex items-center justify-center py-12 gap-2">
        <AlertCircle class="w-5 h-5 text-red-500" />
        <span class="text-sm text-red-500">{error}</span>
      </div>
    {:else if assets.length === 0}
      <div class="flex flex-col items-center justify-center py-12">
        <Package class="w-8 h-8 mb-2" style="color: var(--ds-text-subtle);" />
        <p class="text-sm" style="color: var(--ds-text-subtle);">{t('portal.noAssetsFound')}</p>
      </div>
    {:else}
      <table class="w-full">
        <thead>
          <tr class="border-b" style="border-color: var(--ds-border);">
            {#each displayColumns() as col}
              <th
                class="text-left px-4 py-3 text-sm font-medium capitalize"
                style="color: var(--ds-text-subtle);"
              >
                {getColumnLabel(col)}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each assets as asset}
            <tr class="border-b last:border-b-0 hover:bg-black/5" style="border-color: var(--ds-border);">
              {#each displayColumns() as col}
                <td class="px-4 py-3 text-sm" style="color: var(--ds-text);">
                  {getCellValue(asset, col)}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>

  <!-- Pagination -->
  {#if !loading && !error && totalPages > 1}
    <div class="flex items-center justify-between p-4 border-t" style="border-color: var(--ds-border);">
      <span class="text-sm" style="color: var(--ds-text-subtle);">
        {t('common.showingXofY', { from: (page - 1) * pageSize + 1, to: Math.min(page * pageSize, totalCount), total: totalCount })}
      </span>
      <div class="flex items-center gap-2">
        <button
          onclick={prevPage}
          disabled={page <= 1}
          class="p-2 rounded transition-colors disabled:opacity-40"
          style="color: var(--ds-text-subtle);"
        >
          <ChevronLeft class="w-4 h-4" />
        </button>
        <span class="text-sm" style="color: var(--ds-text);">
          {page} / {totalPages}
        </span>
        <button
          onclick={nextPage}
          disabled={page >= totalPages}
          class="p-2 rounded transition-colors disabled:opacity-40"
          style="color: var(--ds-text-subtle);"
        >
          <ChevronRight class="w-4 h-4" />
        </button>
      </div>
    </div>
  {/if}
</div>
