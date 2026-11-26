<script>
import X from '@lucide/svelte/icons/x';
import Table2 from '@lucide/svelte/icons/table-2';
import ChevronLeft from '@lucide/svelte/icons/chevron-left';
import ChevronRight from '@lucide/svelte/icons/chevron-right';
import AlertCircle from '@lucide/svelte/icons/alert-circle';
import Package from '@lucide/svelte/icons/package';
import EmptyState from '../components/EmptyState.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { assetReports } from '../api/channels.js';
  import { portal } from '../api/portal.js';
  import { portalCustomizationStore as portalStore } from '../stores/portal.svelte.js';
  import { iconMap } from '../stores/portalPresentation.js';
  import { t } from '../stores/i18n.svelte.js';
  import Tooltip from '../components/Tooltip.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import { buildRowActionHref } from './rowActions.js';
  import { toExternal } from '../runtime/contextPath.js';

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
  let customFieldDefinitions = $state([]);
  let definitionLoadToken = 0;

  // Stale-response guard: rapid prev/next clicks fire concurrent loads, and
  // whichever resolves last would otherwise win regardless of which page the
  // user is now on. Each load grabs a token; only the latest commits state.
  let loadToken = 0;

  // Computed columns - use column_config from report or defaults
  let displayColumns = $derived(
    report?.column_config?.length ? report.column_config : ['title', 'asset_tag', 'status']
  );

  // Per-row action links configured on the report. The backend already drops
  // actions whose target request type is not visible to this visitor.
  let rowActions = $derived(report?.config?.row_actions ?? []);

  // Load assets from execute endpoint
  async function loadAssets() {
    if (!slug || !report?.id) {
      loading = false;
      return;
    }

    const myToken = ++loadToken;

    try {
      loading = true;
      error = null;

      const result = await assetReports.execute(slug, report.id, { page, pageSize });

      if (myToken !== loadToken) return;

      assets = result.assets || [];
      totalCount = result.total || 0;
      totalPages = result.total_pages || Math.ceil(totalCount / pageSize);
    } catch (err) {
      if (myToken !== loadToken) return;
      console.error('Failed to load asset report:', err);
      error = err.message || 'Failed to load assets';
      assets = [];
    } finally {
      if (myToken === loadToken) loading = false;
    }
  }

  async function loadCustomFieldDefinitions() {
    if (!slug) {
      customFieldDefinitions = [];
      return;
    }
    const myToken = ++definitionLoadToken;
    try {
      const definitions = await portal.getCustomFields(slug);
      if (myToken === definitionLoadToken) {
        customFieldDefinitions = definitions || [];
      }
    } catch (err) {
      if (myToken === definitionLoadToken) {
        console.error('Failed to load custom field definitions:', err);
        customFieldDefinitions = [];
      }
    }
  }

  // Reload when page changes. Fires once on mount too — no separate onMount.
  $effect(() => {
    page;
    loadAssets();
  });

  $effect(() => {
    slug;
    loadCustomFieldDefinitions();
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
    if (col.startsWith('cf_')) {
      return getCustomFieldDefinition(col)?.name || col.slice(3).replace(/_/g, ' ');
    }
    return labels[col] || col;
  }

  function getCustomFieldDefinition(col) {
    if (!col.startsWith('cf_')) return null;
    const fieldID = Number.parseInt(col.slice(3), 10);
    if (!Number.isFinite(fieldID)) return null;
    return customFieldDefinitions.find((field) => field.id === fieldID) || null;
  }

  function getCustomFieldValue(asset, col) {
    if (!col.startsWith('cf_')) return null;
    return asset.custom_field_values?.[col.slice(3)] ?? null;
  }

  function hasCustomFieldValue(value) {
    if (value === null || value === '') return false;
    return !Array.isArray(value) || value.length > 0;
  }

  // Get cell value for a column. Logical column names ("status", "type",
  // "category") map to the joined name fields the backend emits — see
  // ExecuteAssetReport in portal_assets.go for the response shape.
  function getCellValue(asset, col) {
    if (col === 'status') {
      return asset.status_name ?? '-';
    }
    if (col === 'category') {
      return asset.category_name ?? '-';
    }
    if (col === 'type') {
      return asset.asset_type_name ?? '-';
    }
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
        <IconComponent size={20} color="white" />
      </div>
      <div>
        <h3 class="font-medium flex items-center gap-2" dir="auto" style="color: var(--ds-text);">
          {report.name}
          {#if report.is_active === false}
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
        class="p-2 rounded transition-opacity opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
        style="background-color: var(--ds-danger-subtle); color: var(--ds-text-danger);"
        title={t('portal.removeFromSection')}
      >
        <X class="w-4 h-4" />
      </button>
    {/if}
  </div>

  <!-- Table Content -->
  <div class="overflow-x-auto">
    {#if loading}
      <StateDisplay type="loading" />
    {:else if error}
      <div class="flex items-center justify-center py-12 gap-2">
        <AlertCircle class="w-5 h-5 text-ds-icon-danger" />
        <span class="text-sm text-ds-text-danger">{error}</span>
      </div>
    {:else if assets.length === 0}
      <EmptyState icon={Package} title={t('portal.noAssetsFound')} />
    {:else}
      <table class="w-full">
        <thead>
          <tr class="border-b" style="border-color: var(--ds-border);">
            {#each displayColumns as col}
              <th
                class="text-left px-4 py-3 text-sm font-medium capitalize"
                style="color: var(--ds-text-subtle);"
                data-testid={`asset-report-column-${col}`}
              >
                {getColumnLabel(col)}
              </th>
            {/each}
            {#each rowActions as action (action.id)}
              <th
                class="text-left px-4 py-3 text-sm font-medium"
                style="color: var(--ds-text-subtle);"
                data-testid={`asset-report-action-column-${action.id}`}
              >
                {action.label}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each assets as asset}
            <tr class="border-b last:border-b-0 hover:bg-black/5" style="border-color: var(--ds-border);">
              {#each displayColumns as col}
                {@const customFieldDefinition = getCustomFieldDefinition(col)}
                {@const customFieldValue = getCustomFieldValue(asset, col)}
                <td
                  class="px-4 py-3 text-sm"
                  style="color: var(--ds-text);"
                  data-testid={`asset-report-cell-${asset.id}-${col}`}
                >
                  {#if col.startsWith('cf_')}
                    {#if customFieldDefinition && hasCustomFieldValue(customFieldValue)}
                      <CustomFieldRenderer
                        field={customFieldDefinition}
                        value={customFieldValue}
                        readonly={true}
                        noPadding={true}
                      />
                    {:else}
                      -
                    {/if}
                  {:else}
                    {getCellValue(asset, col)}
                  {/if}
                </td>
              {/each}
              {#each rowActions as action (action.id)}
                <td
                  class="px-4 py-3 text-sm"
                  data-testid={`asset-report-action-cell-${asset.id}-${action.id}`}
                >
                  <a
                    href={toExternal(buildRowActionHref(slug, action, asset))}
                    class="font-medium hover:underline"
                    style="color: var(--ds-text-link);"
                    data-testid={`asset-report-action-${asset.id}-${action.id}`}
                  >
                    {action.label}
                  </a>
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
        <Tooltip content={t('components.pagination.previousPage')}>
          <button
            onclick={prevPage}
            disabled={page <= 1}
            aria-label={t('components.pagination.previousPage')}
            class="p-2 rounded transition-colors disabled:opacity-40"
            style="color: var(--ds-text-subtle);"
          >
            <ChevronLeft class="w-4 h-4" />
          </button>
        </Tooltip>
        <span class="text-sm" style="color: var(--ds-text);">
          {page} / {totalPages}
        </span>
        <Tooltip content={t('components.pagination.nextPage')}>
          <button
            onclick={nextPage}
            disabled={page >= totalPages}
            aria-label={t('components.pagination.nextPage')}
            class="p-2 rounded transition-colors disabled:opacity-40"
            style="color: var(--ds-text-subtle);"
          >
            <ChevronRight class="w-4 h-4" />
          </button>
        </Tooltip>
      </div>
    </div>
  {/if}
</div>
