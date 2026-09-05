<script>
  import { onMount, untrack } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { api } from '../../api.js';
  import { navigate, currentRoute, updateQueryParams } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import SidebarResizeHandle from '../../layout/SidebarResizeHandle.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import Select from '../../components/Select.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import { IconPlus, IconPackage, IconEdit, IconTrash, IconBox, IconChevronRight, IconChevronDown, IconFolder, IconFolderOpen, IconSearch, IconCode, IconUpload, IconShare, IconArrowLeft, IconSettings } from '@tabler/icons-svelte-runes';
  import { permissionStore } from '../../stores/permissions.svelte.js';
  import AssetRelationshipGraph from './AssetRelationshipGraph.svelte';
  import AssetImportWizard from './import/AssetImportWizard.svelte';
  import AssetSubFilterBar from './AssetSubFilterBar.svelte';
  import CustomFieldRenderer from '../items/CustomFieldRenderer.svelte';
  import AssetDetailContent from './AssetDetailContent.svelte';
  import { retainValuesForType } from './assetFormValues.js';
  import { isBooleanCustomFieldType } from '../../utils/customFieldTypes.js';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { formatDateSimple } from '../../utils/dateFormatter.js';
  import { fetchAssetCategories, fetchAssetStatuses, flattenCategories } from './shared/assetSetUtils.js';

  // Props for detail view
  let { assetId = null } = $props();

  // Direct asset detail mode (when assetId prop is set, e.g. /assets/:id)
  let directAsset = $state(null);
  let directAssetLoading = $state(false);

  // State for asset sets (only ones user has access to)
  let assetSets = $state([]);
  let selectedSetId = $state(null);
  let selectedSet = $derived(assetSets.find(s => s.id === selectedSetId));

  // Asset Types and Categories for filtering
  let assetTypes = $state([]);
  let assetCategories = $state([]);
  let expandedCategories = $state(new Set());

  // Assets state
  let assets = $state([]);
  let selectedAsset = $state(null);
  let showAssetForm = $state(false);
  let showImportWizard = $state(false);
  let isAdmin = $derived(selectedSet?.user_permission === 'Administrator');
  let canEdit = $derived(selectedSet?.user_permission === 'Editor' || selectedSet?.user_permission === 'Administrator');
  let showSettingsGear = $derived(isAdmin || permissionStore.canManageAssets);
  let editingAsset = $state(null);
  let assetFormData = $state({
    title: '',
    description: '',
    asset_tag: '',
    asset_type_id: null,
    category_id: null,
    status_id: null,
    custom_field_values: {}
  });
  let selectedTypeFields = $state([]);
  let statuses = $state([]);
  let displayTypeFields = $state([]);
  let showRelationshipGraph = $state(false);
  let savingAsset = $state(false);

  // Asset detail panel resize state
  let assetPanelWidth = $state(320);
  // Filter state
  let selectedCategoryId = $state(null);
  let searchMode = $state('simple'); // 'simple' or 'ql'
  let searchInput = $state(''); // Search input (either simple text or QL query)
  let activeQuery = $state(''); // The committed query that triggers API calls
  let filterBarQL = $state(''); // QL from the visual filter bar
  let allCustomFields = $state([]); // Aggregated custom fields from all asset types

  // Pagination state
  let currentPage = $derived(parseInt($currentRoute.query?.page) || 1);
  let selectedAssetId = $derived($currentRoute.query?.asset || null);
  let totalAssets = $state(0);
  const pageSize = 25;

  // Loading state
  let loading = $state(true);
  let assetTypesRequestSeq = 0;
  let assetCategoriesRequestSeq = 0;
  let assetStatusesRequestSeq = 0;
  let allCustomFieldsRequestSeq = 0;
  let selectedTypeFieldsRequestSeq = 0;
  let assetsRequestSeq = 0;

  onMount(async () => {
    await loadAssetSets();
    loading = false;
  });

  // Fetch asset directly when navigating to /assets/:id
  $effect(() => {
    if (assetId) {
      directAssetLoading = true;
      untrack(() => {
        api.assets.get(Number(assetId)).then(async (asset) => {
          directAsset = asset;
          if (asset?.asset_set_id && !selectedSetId) {
            selectedSetId = asset.asset_set_id;
            await loadAssetSets();
          }
          if (asset?.asset_type_id) {
            loadTypeFieldsForDisplay(asset.asset_type_id);
          }
        }).catch(err => {
          console.error('Failed to load asset:', err);
        }).finally(() => {
          directAssetLoading = false;
        });
      });
    }
  });

  async function loadAssetSets() {
    try {
      const sets = await api.assetSets.getAll();
      assetSets = sets || [];
      if (assetSets.length > 0 && !selectedSetId) {
        const defaultSet = assetSets.find(s => s.is_default) || assetSets[0];
        selectedSetId = defaultSet.id;
      }
    } catch (error) {
      console.error('Failed to load asset sets:', error);
    }
  }

  // Load metadata when set changes (browser mode only)
  $effect(() => {
    if (!assetId && selectedSetId) {
      loadAssetTypes();
      loadAssetCategories();
      loadStatuses();
    }
  });

  async function loadAssetTypes() {
    if (!selectedSetId) return;
    const setId = selectedSetId;
    const requestSeq = ++assetTypesRequestSeq;
    try {
      const types = await api.assetTypes.getAll(setId);
      if (requestSeq !== assetTypesRequestSeq || selectedSetId !== setId) return;
      assetTypes = (types || []).filter(t => t.is_active);
      void loadAllCustomFields(setId, assetTypes);
    } catch (error) {
      if (requestSeq !== assetTypesRequestSeq || selectedSetId !== setId) return;
      console.error('Failed to load asset types:', error);
    }
  }

  async function loadAllCustomFields(setId = selectedSetId, types = assetTypes) {
    const requestSeq = ++allCustomFieldsRequestSeq;
    if (types.length === 0) {
      allCustomFields = [];
      return;
    }
    try {
      const seenFieldIds = new Set();
      const fields = [];
      for (const type of types) {
        const typeFields = await api.assetTypes.getFields(type.id);
        for (const f of (typeFields || [])) {
          if (!seenFieldIds.has(f.custom_field_id)) {
            seenFieldIds.add(f.custom_field_id);
            fields.push(f);
          }
        }
      }
      if (requestSeq !== allCustomFieldsRequestSeq || selectedSetId !== setId) return;
      allCustomFields = fields;
    } catch (error) {
      if (requestSeq !== allCustomFieldsRequestSeq || selectedSetId !== setId) return;
      console.error('Failed to load custom fields for filter:', error);
      allCustomFields = [];
    }
  }

  async function loadAssetCategories() {
    const setId = selectedSetId;
    const requestSeq = ++assetCategoriesRequestSeq;
    const categories = await fetchAssetCategories(setId);
    if (requestSeq !== assetCategoriesRequestSeq || selectedSetId !== setId) return;
    assetCategories = categories;
  }

  async function loadStatuses() {
    const setId = selectedSetId;
    const requestSeq = ++assetStatusesRequestSeq;
    const nextStatuses = await fetchAssetStatuses(setId);
    if (requestSeq !== assetStatusesRequestSeq || selectedSetId !== setId) return;
    statuses = nextStatuses;
  }

  async function loadAssets() {
    if (!selectedSetId) return;
    const setId = selectedSetId;
    const requestSeq = ++assetsRequestSeq;
    try {
      const filters = {
        limit: pageSize,
        offset: (currentPage - 1) * pageSize
      };
      if (selectedCategoryId) {
        filters.category_id = selectedCategoryId;
        filters.include_subcategories = true;
      }
      // Build combined QL from search input and visual filter bar
      const qlParts = [];
      if (activeQuery) {
        if (searchMode === 'ql') {
          qlParts.push(activeQuery);
        } else {
          const escapedInput = activeQuery.replace(/"/g, '\\"');
          qlParts.push(`(title ~ "${escapedInput}" OR description ~ "${escapedInput}")`);
        }
      }
      if (filterBarQL) {
        qlParts.push(filterBarQL);
      }
      if (qlParts.length > 0) {
        filters.ql = qlParts.join(' AND ');
      }
      const result = await api.assets.getAll(setId, filters);
      if (requestSeq !== assetsRequestSeq || selectedSetId !== setId) return;
		assets = result?.data || [];
		totalAssets = result?.pagination?.total_items || 0;
    } catch (error) {
      if (requestSeq !== assetsRequestSeq || selectedSetId !== setId) return;
      console.error('Failed to load assets:', error);
    }
  }

  // Navigate to a new page via URL
  function updatePage(page) {
    updateQueryParams({ page: page > 1 ? page : null }, { push: true });
  }

  // Reset to page 1 when filters change (not on initial load)
  let filtersInitialized = false;
  $effect(() => {
    if (!assetId && selectedSetId) {
      const _ = [selectedCategoryId, activeQuery, filterBarQL];
      if (!filtersInitialized) {
        filtersInitialized = true;
        return;
      }
      untrack(() => {
        if (currentPage !== 1) {
          updatePage(1);
        }
      });
    }
  });

  // Reload assets when any dependency changes (set, page, filters)
  $effect(() => {
    if (!assetId && selectedSetId) {
      const _ = [currentPage, selectedCategoryId, activeQuery, filterBarQL];
      untrack(() => loadAssets());
    }
  });

  // Restore selection from URL param after assets load
  $effect(() => {
    if (selectedAssetId && assets.length > 0) {
      const found = assets.find(a => a.id === selectedAssetId);
      if (found) selectedAsset = found;
    }
  });

  // In simple mode, update activeQuery as user types (type-ahead)
  $effect(() => {
    if (searchMode === 'simple') {
      activeQuery = searchInput;
    }
  });

  // Handle page change from DataTable
  function handlePageChange(page) {
    updatePage(page);
  }

  // Load custom fields when asset type changes in form
  $effect(() => {
    if (assetFormData.asset_type_id && showAssetForm) {
      loadTypeFields(assetFormData.asset_type_id);
    } else {
      selectedTypeFields = [];
    }
  });

  async function loadTypeFields(typeId) {
    const requestSeq = ++selectedTypeFieldsRequestSeq;
    try {
      const fields = await api.assetTypes.getFields(typeId);
      if (
        requestSeq !== selectedTypeFieldsRequestSeq ||
        Number(assetFormData.asset_type_id) !== Number(typeId)
      ) return;
      selectedTypeFields = fields || [];
      if (editingAsset && Number(assetFormData.asset_type_id) === Number(typeId)) {
        assetFormData.custom_field_values = retainValuesForType(
          assetFormData.custom_field_values,
          selectedTypeFields,
        );
      }
    } catch (error) {
      if (
        requestSeq !== selectedTypeFieldsRequestSeq ||
        Number(assetFormData.asset_type_id) !== Number(typeId)
      ) return;
      console.error('Failed to load type fields:', error);
      selectedTypeFields = [];
    }
  }

  // Load custom fields for display when an asset is selected
  $effect(() => {
    if (selectedAsset?.asset_type_id) {
      loadTypeFieldsForDisplay(selectedAsset.asset_type_id);
    } else {
      displayTypeFields = [];
    }
  });

  async function loadTypeFieldsForDisplay(typeId) {
    try {
      const fields = await api.assetTypes.getFields(typeId);
      displayTypeFields = fields || [];
    } catch (error) {
      console.error('Failed to load type fields for display:', error);
      displayTypeFields = [];
    }
  }

  function showAddAssetForm() {
    showAssetForm = true;
    editingAsset = null;
    // Find default status
    const defaultStatus = statuses.find(s => s.is_default);
    assetFormData = {
      title: '',
      description: '',
      asset_tag: '',
      asset_type_id: assetTypes.length > 0 ? assetTypes[0].id : null,
      category_id: selectedCategoryId ?? null,
      status_id: defaultStatus?.id ?? null,
      custom_field_values: {}
    };
  }

  function showEditAssetForm(asset) {
    showAssetForm = true;
    editingAsset = asset;
    assetFormData = {
      title: asset.title,
      description: asset.description || '',
      asset_tag: asset.asset_tag || '',
      asset_type_id: asset.asset_type_id ?? null,
      category_id: asset.category_id ?? null,
      status_id: asset.status_id ?? null,
      custom_field_values: { ...(asset.custom_field_values || {}) }
    };
  }

  async function handleAssetSubmit() {
    if (savingAsset) return;
    savingAsset = true;

    try {
      // Validate required custom fields
      for (const field of selectedTypeFields) {
        if (field.is_required && !isBooleanCustomFieldType(field.field_type)) {
          const value = assetFormData.custom_field_values[field.custom_field_id];
          if (value === undefined || value === null || value === '') {
            errorToast(t('validation.requiredField', { field: field.field_name }));
            return;
          }
        }
      }

      if (editingAsset) {
        await api.assets.update(editingAsset.id, assetFormData);
        showAssetForm = false;
        if (assetId) {
          // Refresh direct asset view
          const refreshed = await api.assets.get(Number(assetId));
          directAsset = refreshed;
        } else {
          await loadAssets();
          const updated = assets.find(a => a.id === editingAsset.id);
          if (updated) {
            selectedAsset = updated;
            updateQueryParams({ asset: updated.id });
          }
        }
      } else {
        const created = await api.assets.create(selectedSetId, assetFormData);
        showAssetForm = false;
        if (created?.id) {
          await loadAssets();
          const newAsset = assets.find(a => a.id === created.id);
          if (newAsset) {
            selectedAsset = newAsset;
          }
          updateQueryParams({ asset: created.id });
        }
      }
    } catch (error) {
      console.error('Failed to save asset:', error);
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message }));
    } finally {
      savingAsset = false;
    }
  }

  async function deleteAsset(id) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteAsset'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await api.assets.delete(id);
        if (assetId) {
          navigate('/assets');
          return;
        }
        await loadAssets();
        if (selectedAsset?.id === id) {
          selectedAsset = null;
          updateQueryParams({ asset: null });
        }
      } catch (error) {
        console.error('Failed to delete asset:', error);
        errorToast(t('dialogs.alerts.failedToDelete', { error: error.message }));
      }
    }
  }

  function toggleCategory(categoryId) {
    const newExpanded = new Set(expandedCategories);
    if (newExpanded.has(categoryId)) {
      newExpanded.delete(categoryId);
    } else {
      newExpanded.add(categoryId);
    }
    expandedCategories = newExpanded;
  }

  function selectCategory(categoryId) {
    selectedCategoryId = categoryId;
  }

  const flatCategories = $derived(flattenCategories(assetCategories));

  // Column definitions for DataTable
  const assetColumns = [
    {
      key: 'title',
      label: 'NAME',
      slot: 'title'
    },
    {
      key: 'asset_type_name',
      label: 'TYPE',
      slot: 'type'
    },
    {
      key: 'category_name',
      label: 'CATEGORY',
      slot: 'category'
    },
    {
      key: 'status_name',
      label: 'STATUS',
      slot: 'status'
    },
    {
      key: 'created_at',
      label: 'CREATED',
      render: (asset) => formatDateSimple(asset.created_at)
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];

  function buildAssetDropdownItems(asset) {
    const items = [];
    if (canEdit) {
      items.push({
        id: 'edit',
        type: 'regular',
        icon: IconEdit,
        title: 'Edit',
        hoverClass: 'hover-bg',
        onClick: () => showEditAssetForm(asset)
      });
      items.push({
        id: 'delete',
        type: 'regular',
        icon: IconTrash,
        title: 'Delete',
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deleteAsset(asset.id)
      });
    }
    return items;
  }
</script>

{#if assetId}
  <!-- Direct asset detail view (/assets/:id) -->
  <div class="flex min-h-screen" style="background: var(--ds-surface);">
    <div class="flex-1 max-w-4xl mx-auto p-6">
      {#if directAssetLoading}
        <div class="flex items-center justify-center h-64">
          <span style="color: var(--ds-text-subtle);">{t('common.loading')}</span>
        </div>
      {:else if directAsset}
        <!-- Back button -->
        <button
          class="inline-flex items-center gap-1.5 mb-6 text-sm font-medium rounded-lg px-3 py-1.5 transition-colors"
          style="color: var(--ds-text-subtle); background: transparent;"
          onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
          onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
          onclick={() => navigate('/assets')}
        >
          <IconArrowLeft class="w-4 h-4" />
          {t('common.back')}
        </button>

        <!-- Header -->
        <div class="flex items-center justify-between mb-6">
          <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">{directAsset.title}</h1>
          <div class="flex items-center gap-2">
            <button
              class="p-2 rounded-lg transition-colors"
              style="background: transparent;"
              onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
              onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
              onclick={() => { showRelationshipGraph = true; }}
              title="Relationship Graph"
            >
              <IconShare class="w-4 h-4" style="color: var(--ds-icon);" />
            </button>
            {#if canEdit}
              <button
                data-testid="asset-edit"
                class="p-2 rounded-lg transition-colors"
                style="background: transparent;"
                onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
                onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
                onclick={() => showEditAssetForm(directAsset)}
                title={t('common.edit')}
              >
                <IconEdit class="w-4 h-4" style="color: var(--ds-icon);" />
              </button>
              <button
                class="p-2 rounded-lg transition-colors"
                style="background: transparent; color: var(--ds-text-danger);"
                onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
                onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
                onclick={() => deleteAsset(directAsset.id)}
                title={t('common.delete')}
              >
                <IconTrash class="w-4 h-4" />
              </button>
            {/if}
          </div>
        </div>

        <!-- Detail content -->
        <div class="rounded-lg p-6" style="background: var(--ds-surface-raised); border: 1px solid var(--ds-border);">
          <AssetDetailContent asset={directAsset} fieldDefinitions={displayTypeFields} layout="grid" />
        </div>
      {:else}
        <EmptyState
          icon={IconPackage}
          title="Asset not found"
          description="The asset you're looking for doesn't exist or you don't have access to it."
        />
      {/if}
    </div>
  </div>

  <!-- Relationship Graph for direct view -->
  {#if directAsset}
    <AssetRelationshipGraph bind:isOpen={showRelationshipGraph} assetId={directAsset.id} />
  {/if}
{:else}
<div
  class="flex h-full min-h-screen"
  data-testid="asset-browser"
  data-total-assets={totalAssets}
  style="background: var(--ds-surface);"
>
  <!-- Left sidebar: Category tree -->
  <div class="w-64 flex flex-col" style="border-right: 1px solid var(--ds-border); background: var(--ds-surface-raised);">
    <!-- Set selector -->
    <div class="px-4 h-[80px] flex items-center" style="border-bottom: 1px solid var(--ds-border);">
      <Select id="asset-set-select" bind:value={selectedSetId} class="w-full" options={assetSets.length === 0 ? [{ value: null, label: 'No asset sets available', disabled: true }] : assetSets.map(set => ({ value: set.id, label: set.name }))} />
    </div>

    <!-- Category tree -->
    <div class="flex-1 overflow-auto p-4">
      <button
        class="w-full text-left px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-2"
        style={selectedCategoryId === null ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if (selectedCategoryId !== null) e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if (selectedCategoryId !== null) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
        onclick={() => selectCategory(null)}
      >
        <IconPackage class="w-4 h-4" />
        {t('common.all')}
      </button>

      {#if assetCategories.length > 0}
        <div class="mt-2">
          {#snippet renderCategoryNav(category, level = 0)}
            <div style="padding-left: {level * 16}px">
              <div
                role="button"
                tabindex="0"
                class="w-full text-left px-3 py-1.5 rounded-lg text-sm font-medium transition-all flex items-center gap-1 cursor-pointer"
                style={selectedCategoryId === category.id ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
                onmouseenter={(e) => { if (selectedCategoryId !== category.id) e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
                onmouseleave={(e) => { if (selectedCategoryId !== category.id) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
                onclick={() => selectCategory(category.id)}
                onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectCategory(category.id); } }}
              >
                {#if category.has_children}
                  <button
                    type="button"
                    class="p-0.5 rounded"
                    style="background: transparent;"
                    onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
                    onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
                    onclick={(e) => { e.stopPropagation(); toggleCategory(category.id); }}
                  >
                    {#if expandedCategories.has(category.id)}
                      <IconChevronDown class="w-3 h-3" />
                    {:else}
                      <IconChevronRight class="w-3 h-3" />
                    {/if}
                  </button>
                {:else}
                  <span class="w-4"></span>
                {/if}
                {#if expandedCategories.has(category.id)}
                  <IconFolderOpen class="w-4 h-4 text-yellow-500" />
                {:else}
                  <IconFolder class="w-4 h-4 text-yellow-500" />
                {/if}
                <span class="truncate">{category.name}</span>
                {#if category.asset_count > 0}
                  <span class="text-xs text-gray-400 ml-auto">{category.asset_count}</span>
                {/if}
              </div>
              {#if category.has_children && expandedCategories.has(category.id) && category.children}
                {#each category.children as child}
                  {@render renderCategoryNav(child, level + 1)}
                {/each}
              {/if}
            </div>
          {/snippet}
          {#each assetCategories as category}
            {@render renderCategoryNav(category)}
          {/each}
        </div>
      {/if}
    </div>
  </div>

  <!-- Main content -->
  <div class="flex-1 flex flex-col overflow-hidden">
    <!-- Header with search -->
    <div class="px-4 h-[80px] flex items-center gap-4" style="border-bottom: 1px solid var(--ds-border);">
      <div class="flex-1 min-w-0 relative flex items-center gap-2">
        <div class="flex-1 relative">
          <IconSearch class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2" style="color: var(--ds-icon);" />
          <Input
            dataTestid="asset-search"
            type="text"
            placeholder={searchMode === 'ql' ? 'Query: status = "Active" (press Enter)' : 'Search by name...'}
            bind:value={searchInput}
            onkeydown={(e) => { if (searchMode === 'ql' && e.key === 'Enter') activeQuery = searchInput; }}
            class={`pl-9 ${searchMode === 'ql' ? 'font-mono' : ''}`}
            size="small"
            title={searchMode === 'ql' ? 'QL Query - Press Enter to search. Examples: status = "Active", type IN ("Laptop", "Desktop"), title ~ "server"' : 'Search by title or description'}
          />
        </div>
        <button
          onclick={() => {
            searchMode = searchMode === 'simple' ? 'ql' : 'simple';
            searchInput = '';
            activeQuery = '';
          }}
          class="p-2 rounded-lg transition-colors"
          style="background: {searchMode === 'ql' ? 'var(--ds-interactive-subtle)' : 'var(--ds-background-input)'}; border: 1px solid {searchMode === 'ql' ? 'var(--ds-border-selected)' : 'var(--ds-border)'}; color: {searchMode === 'ql' ? 'var(--ds-interactive)' : 'var(--ds-text)'};"
          title={searchMode === 'ql' ? 'Switch to simple search' : 'Switch to QL query mode'}
        >
          <IconCode class="w-4 h-4" />
        </button>
        {#if selectedSetId}
          <AssetSubFilterBar
            {statuses}
            {assetTypes}
            categories={assetCategories}
            customFields={allCustomFields}
            onApply={(ql) => { filterBarQL = ql; }}
          />
        {/if}
      </div>
      {#if selectedSetId}
        {#if isAdmin}
          <Button dataTestid="asset-import-open" onclick={() => { showImportWizard = true; }} variant="default" class="whitespace-nowrap">
            <IconUpload class="w-4 h-4 mr-1" />
            Import
          </Button>
        {/if}
        {#if canEdit}
          <Button dataTestid="asset-create" onclick={showAddAssetForm} class="whitespace-nowrap" keyboardHint="A" hotkeyConfig={{ key: toHotkeyString('assets', 'upload'), guard: () => !!(selectedSetId && !showAssetForm) }}>
            <IconPlus class="w-4 h-4 mr-1" />
            {t('common.create')}
          </Button>
        {/if}
      {/if}
      {#if showSettingsGear}
        <button
          onclick={() => navigate('/assets/settings')}
          class="p-2 rounded-lg transition-colors hover:bg-[var(--ds-background-input)]"
          title="Asset Settings"
        >
          <IconSettings class="w-4 h-4" style="color: var(--ds-icon);" />
        </button>
      {/if}
    </div>

    <!-- Asset list -->
    <div class="flex-1 overflow-auto p-4">
      {#snippet createAssetAction()}
        <Button onclick={showAddAssetForm}>
          <IconPlus class="w-4 h-4 mr-1" />
          Create Asset
        </Button>
      {/snippet}

      {#if loading}
        <div class="flex items-center justify-center h-full">
          <div class="text-gray-500">{t('common.loading')}</div>
        </div>
      {:else if assetSets.length === 0}
        <EmptyState
          icon={IconPackage}
          title={t('common.noItems')}
          description={t('common.noItems')}
        />
      {:else if assets.length === 0}
        <EmptyState
          icon={IconBox}
          title={t('common.noItems')}
          description={activeQuery || selectedCategoryId ? t('common.noItems') : t('common.noItems')}
          action={selectedSetId && canEdit && !activeQuery && !selectedCategoryId ? createAssetAction : null}
        />
      {:else}
        <DataTable
          columns={assetColumns}
          data={assets}
          keyField="id"
          selectedItemId={selectedAsset?.id}
          emptyMessage={t('common.noItems')}
          emptyIcon={IconBox}
          actionItems={buildAssetDropdownItems}
          onRowClick={(asset) => { selectedAsset = asset; updateQueryParams({ asset: asset.id }); }}
          pagination={true}
          {pageSize}
          {currentPage}
          totalItems={totalAssets}
          onPageChange={handlePageChange}
          rowAttrs={(asset) => ({
            'data-asset-id': asset.id,
            'data-asset-tag': asset.asset_tag || ''
          })}
        >
          {#snippet title(item)}
            <span data-testid="asset-row">
              <span data-testid={`asset-title-${item.id}`}>{item.title}</span>
            </span>
          {/snippet}
          {#snippet type(item)}
            <div class="flex items-center gap-2">
            {#if item.asset_type_name}
              <ColorDot color={item.asset_type_color || '#6b7280'} size="sm" />
              <span>{item.asset_type_name}</span>
            {:else}
              <span style="color: var(--ds-text-subtlest);">—</span>
            {/if}
            </div>
          {/snippet}

          {#snippet category(item)}
            {#if item.category_name}
              <span class="inline-flex items-center gap-1">
                <IconFolder class="w-3 h-3 text-yellow-500" />
                {item.category_name}
              </span>
            {:else}
              <span style="color: var(--ds-text-subtlest);">—</span>
            {/if}
          {/snippet}

          {#snippet status(item)}
            {#if item.status_name}
              <span class="inline-flex items-center gap-1.5">
                <span class="w-2 h-2 rounded-full" style="background-color: {item.status_color || '#6b7280'};"></span>
                {item.status_name}
              </span>
            {:else}
              <span style="color: var(--ds-text-subtlest);">—</span>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </div>
  </div>

  <!-- Right sidebar: Asset detail (when selected) -->
  {#if selectedAsset}
    <div class="flex-shrink-0 flex flex-col relative" style="width: {assetPanelWidth}px; min-width: 280px; max-width: 600px; border-left: 1px solid var(--ds-border);">
      <SidebarResizeHandle
        width={assetPanelWidth}
        minWidth={280}
        maxWidth={600}
        defaultWidth={320}
        edge="left"
        label="Resize asset details"
        title="Drag to resize, double-click to reset"
        onresize={(width) => assetPanelWidth = width}
      />
      <div class="p-4 flex items-center justify-between" style="border-bottom: 1px solid var(--ds-border);">
        <h2 class="font-semibold truncate" style="color: var(--ds-text);">{selectedAsset.title}</h2>
        <div class="flex items-center gap-1">
          <button
            class="p-1 rounded"
            style="background: transparent;"
            onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
            onclick={() => { showRelationshipGraph = true; }}
            title="Relationship Graph"
          >
            <IconShare class="w-4 h-4" style="color: var(--ds-icon);" />
          </button>
          {#if canEdit}
            <button
              data-testid="asset-edit"
              class="p-1 rounded"
              style="background: transparent;"
              onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
              onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
              onclick={() => showEditAssetForm(selectedAsset)}
              title={t('common.edit')}
            >
              <IconEdit class="w-4 h-4" style="color: var(--ds-icon);" />
            </button>
          {/if}
          <button
            class="p-1 rounded"
            style="background: transparent;"
            onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
            onclick={() => { selectedAsset = null; updateQueryParams({ asset: null }); }}
          >
            <IconChevronRight class="w-4 h-4" style="color: var(--ds-icon);" />
          </button>
        </div>
      </div>
      <div class="flex-1 overflow-auto p-4">
        <AssetDetailContent asset={selectedAsset} fieldDefinitions={displayTypeFields} />
      </div>
    </div>
  {/if}
</div>
{/if}

<!-- Asset Form Modal -->
<Modal
  isOpen={showAssetForm}
  preventClose={savingAsset}
  submitDisabled={savingAsset}
  onclose={() => showAssetForm = false}
  onSubmit={handleAssetSubmit}
>
  <ModalHeader title={editingAsset ? 'Edit Asset' : 'New Asset'} onClose={() => showAssetForm = false} />
  <form onsubmit={(e) => { e.preventDefault(); handleAssetSubmit(); }} class="p-6">
    <div class="space-y-4">
      <div>
        <Label color="default" class="mb-1">Title</Label>
        <Input
          id="asset-title-input"
          type="text"
          bind:value={assetFormData.title}
          required
          size="small"
        />
      </div>
      <div>
        <Label color="default" class="mb-1">Description</Label>
        <Textarea
          bind:value={assetFormData.description}
          rows={3}
          size="small"
        />
      </div>
      <div>
        <Label color="default" class="mb-1">Asset Tag</Label>
        <Input
          id="asset-tag-input"
          type="text"
          bind:value={assetFormData.asset_tag}
          size="small"
        />
      </div>
      <div>
        <Label color="default" class="mb-1">Asset Type</Label>
        <Select bind:value={assetFormData.asset_type_id} options={[{ value: null, label: 'No Type' }, ...assetTypes.map(type => ({ value: type.id, label: type.name }))]} />
      </div>
      <div>
        <Label color="default" class="mb-1">Category</Label>
        <Select bind:value={assetFormData.category_id} options={[{ value: null, label: 'No Category' }, ...flatCategories.map(cat => ({ value: cat.id, label: '  '.repeat(cat.level) + cat.name }))]} />
      </div>
      <div>
        <Label color="default" class="mb-1">Status</Label>
        <Select bind:value={assetFormData.status_id} options={statuses.map(status => ({ value: status.id, label: status.name }))} />
      </div>
      {#if selectedTypeFields.length > 0}
        <div class="border-t pt-4 mt-4" style="border-color: var(--ds-border);">
          <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Custom Fields</h4>
          {#each selectedTypeFields as field}
            <div class="mb-4">
              <Label color="default" class="mb-1">
                {field.field_name}
                {#if field.is_required}
                  <span style="color: var(--ds-text-danger, #ef4444);">*</span>
                {/if}
              </Label>
              <CustomFieldRenderer
                field={{
                  id: field.custom_field_id,
                  name: field.field_name,
                  field_type: field.field_type,
                  options: field.options
                }}
                value={assetFormData.custom_field_values[field.custom_field_id]}
                readonly={false}
                onChange={(val) => assetFormData.custom_field_values[field.custom_field_id] = val}
                required={field.is_required}
              />
            </div>
          {/each}
        </div>
      {/if}
    </div>
    <div class="flex justify-end gap-2 mt-6">
      <Button variant="outline" type="button" onclick={() => showAssetForm = false} keyboardHint="Esc">{t('common.cancel')}</Button>
      <Button
        dataTestid="asset-submit"
        type="submit"
        disabled={savingAsset}
        loading={savingAsset}
        keyboardHint="↵"
      >{editingAsset ? t('common.save') : t('common.create')}</Button>
    </div>
  </form>
</Modal>

<!-- Import Wizard -->
{#if isAdmin}
  <AssetImportWizard
    bind:isOpen={showImportWizard}
    setId={selectedSetId}
    onComplete={() => { loadAssets(); }}
  />
{/if}

<!-- Relationship Graph (browser mode) -->
{#if selectedAsset}
  <AssetRelationshipGraph bind:isOpen={showRelationshipGraph} assetId={selectedAsset.id} />
{/if}
