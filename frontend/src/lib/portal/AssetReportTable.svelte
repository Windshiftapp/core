<script>
  import { X, Table2, ChevronLeft, ChevronRight, Loader2, AlertCircle, Package } from '@lucide/svelte';
  import { api } from '../api.js';
  import { portalCustomizationStore as portalStore } from '../stores/portal.svelte.js';
  import { iconMap } from '../stores/portalPresentation.js';
  import { t } from '../stores/i18n.svelte.js';
  import Input from '../components/Input.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';

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

  // Form-mode state. For run_mode='form' reports the user submits values that
  // get substituted into the report's CQL before the query runs. Until they
  // submit, we render the form instead of running the query.
  let isFormMode = $derived(report?.run_mode === 'form');
  let formFields = $state([]);
  let formValues = $state({});
  let formSubmitted = $state(false);
  let formLoading = $state(false);

  // Stale-response guard: rapid prev/next clicks fire concurrent loads, and
  // whichever resolves last would otherwise win regardless of which page the
  // user is now on. Each load grabs a token; only the latest commits state.
  let loadToken = 0;

  // Computed columns - use column_config from report or defaults
  let displayColumns = $derived(
    report?.column_config?.length ? report.column_config : ['title', 'asset_tag', 'status']
  );

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

      let result;
      if (isFormMode && formSubmitted) {
        result = await api.assetReports.submit(slug, report.id, {
          params: formValues,
          page,
          perPage: pageSize
        });
      } else if (!isFormMode) {
        result = await api.assetReports.execute(slug, report.id, { page, pageSize });
      } else {
        // Form mode, not yet submitted — don't run the query.
        loading = false;
        return;
      }

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

  // Fetch the form schema when this is a form-mode report. Mirrors the same
  // visibility check on the backend, so a 404 here means the customer isn't
  // allowed to see (or run) this report.
  async function loadFormFields() {
    if (!isFormMode || !slug || !report?.id) return;
    try {
      formLoading = true;
      error = null;
      formFields = await api.assetReports.getPortalFields(slug, report.id);
      const initial = {};
      for (const f of formFields) initial[f.field_identifier] = '';
      formValues = initial;
    } catch (err) {
      console.error('Failed to load asset report fields:', err);
      error = err.message || 'Failed to load form fields';
    } finally {
      formLoading = false;
    }
  }

  async function loadCustomFieldDefinitions() {
    if (!slug) {
      customFieldDefinitions = [];
      return;
    }
    const myToken = ++definitionLoadToken;
    try {
      const definitions = await api.portal.getCustomFields(slug);
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

  function submitForm(event) {
    event?.preventDefault?.();
    formSubmitted = true;
    page = 1;
    loadAssets();
  }

  function resetForm() {
    formSubmitted = false;
    assets = [];
    totalCount = 0;
    totalPages = 0;
    page = 1;
  }

  // Reload when page changes. Fires once on mount too — no separate onMount.
  // For form-mode reports the effect drives both the schema fetch (once) and
  // the query (after submit).
  $effect(() => {
    page;
    if (isFormMode && !formSubmitted && formFields.length === 0) {
      loadFormFields();
      loading = false;
      return;
    }
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
        style="background-color: var(--ds-danger-subtle); color: var(--ds-text-danger);"
        title={t('portal.removeFromSection')}
      >
        <X class="w-4 h-4" />
      </button>
    {/if}
  </div>

  {#if isFormMode && !formSubmitted}
    <!-- Form-mode: render the field schema as a simple form. On submit, the
         values get substituted into the report's CQL (server-side) and the
         results render via the table branch below. -->
    <div class="p-4">
      {#if formLoading}
        <div class="flex items-center justify-center py-8">
          <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
        </div>
      {:else if error}
        <div class="flex items-center gap-2 py-4">
          <AlertCircle class="w-5 h-5 text-red-500" />
          <span class="text-sm text-red-500">{error}</span>
        </div>
      {:else if formFields.length === 0}
        <p class="text-sm text-center py-6" style="color: var(--ds-text-subtle);">
          {t('portal.noFormFieldsConfigured')}
        </p>
      {:else}
        <form onsubmit={submitForm} class="space-y-3">
          {#each formFields as field (field.id)}
            <div>
              <label
                for={`ar-field-${field.id}`}
                class="block text-sm font-medium mb-1"
                style="color: var(--ds-text);"
              >
                {field.field_label || field.field_name || field.field_identifier}
                {#if field.is_required}<span class="text-red-500">*</span>{/if}
              </label>
              <Input
                id={`ar-field-${field.id}`}
                type="text"
                bind:value={formValues[field.field_identifier]}
                required={field.is_required}
                size="small"
              />
              {#if field.description}
                <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{field.description}</p>
              {/if}
            </div>
          {/each}
          <div class="pt-2">
            <button
              type="submit"
              class="px-4 py-2 rounded text-sm font-medium"
              style="background-color: {report.color || '#6b7280'}; color: white;"
            >
              {t('common.search')}
            </button>
          </div>
        </form>
      {/if}
    </div>
  {:else}

  {#if isFormMode && formSubmitted}
    <!-- Form-mode results have a back-to-form button so the user can change
         their inputs without reloading the page. -->
    <div class="px-4 py-2 border-b flex items-center justify-end" style="border-color: var(--ds-border);">
      <button
        onclick={resetForm}
        class="text-xs px-2 py-1 rounded"
        style="color: var(--ds-text-subtle);"
      >
        ← {t('common.back')}
      </button>
    </div>
  {/if}

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
            {#each displayColumns as col}
              <th
                class="text-left px-4 py-3 text-sm font-medium capitalize"
                style="color: var(--ds-text-subtle);"
                data-testid={`asset-report-column-${col}`}
              >
                {getColumnLabel(col)}
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
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
  {/if}

  <!-- Pagination -->
  {#if !loading && !error && totalPages > 1 && (!isFormMode || formSubmitted)}
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
