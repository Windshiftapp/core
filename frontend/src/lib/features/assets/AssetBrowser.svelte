<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import Select from '../../components/Select.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import { Plus, Package, Edit, Trash2, Box, ChevronRight, ChevronDown, Folder, FolderOpen, Search, ExternalLink, Code } from 'lucide-svelte';
  import CustomFieldRenderer from '../items/CustomFieldRenderer.svelte';
  import { matchesShortcut } from '../../utils/keyboardShortcuts.js';

  // Props for detail view
  let { assetId = null } = $props();

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
  let editingAsset = $state(null);
  let assetFormData = $state({
    title: '',
    description: '',
    asset_type_id: null,
    category_id: null,
    status_id: null,
    custom_field_values: {}
  });
  let selectedTypeFields = $state([]);
  let statuses = $state([]);
  let displayTypeFields = $state([]);

  // Asset detail panel resize state
  let assetPanelWidth = $state(320);
  let isResizingAssetPanel = $state(false);

  function startAssetPanelResize(event) {
    isResizingAssetPanel = true;
    const startX = event.clientX;
    const startWidth = assetPanelWidth;

    function handleMouseMove(e) {
      if (!isResizingAssetPanel) return;
      const deltaX = startX - e.clientX;
      assetPanelWidth = Math.max(280, Math.min(600, startWidth + deltaX));
    }

    function handleMouseUp() {
      isResizingAssetPanel = false;
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    }

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    event.preventDefault();
  }

  // Filter state
  let selectedCategoryId = $state(null);
  let searchMode = $state('simple'); // 'simple' or 'ql'
  let searchInput = $state(''); // Search input (either simple text or QL query)
  let activeQuery = $state(''); // The committed query that triggers API calls

  // Pagination state
  let currentPage = $state(1);
  let totalAssets = $state(0);
  const pageSize = 25;

  // Loading state
  let loading = $state(true);

  onMount(async () => {
    await loadAssetSets();
    loading = false;
    window.addEventListener('keydown', handleGlobalKeydown);
  });

  onDestroy(() => {
    window.removeEventListener('keydown', handleGlobalKeydown);
  });

  function handleGlobalKeydown(event) {
    // 'a' to add new asset (only when a set is selected)
    if (matchesShortcut(event, { key: 'a' }) && selectedSetId && !showAssetForm) {
      const target = event.target;
      if (target.tagName !== 'INPUT' && target.tagName !== 'TEXTAREA' && !target.isContentEditable) {
        event.preventDefault();
        showAddAssetForm();
      }
    }
  }

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

  // Load data when set changes
  $effect(() => {
    if (selectedSetId) {
      loadAssetTypes();
      loadAssetCategories();
      loadStatuses();
      loadAssets();
    }
  });

  async function loadAssetTypes() {
    if (!selectedSetId) return;
    try {
      const types = await api.assetTypes.getAll(selectedSetId);
      assetTypes = (types || []).filter(t => t.is_active);
    } catch (error) {
      console.error('Failed to load asset types:', error);
    }
  }

  async function loadAssetCategories() {
    if (!selectedSetId) return;
    try {
      const categories = await api.assetCategories.getAll(selectedSetId, true);
      assetCategories = categories || [];
    } catch (error) {
      console.error('Failed to load asset categories:', error);
    }
  }

  async function loadStatuses() {
    if (!selectedSetId) return;
    try {
      const result = await api.assetStatuses.getAll(selectedSetId);
      statuses = result || [];
    } catch (error) {
      console.error('Failed to load statuses:', error);
    }
  }

  async function loadAssets() {
    if (!selectedSetId) return;
    try {
      const filters = {
        limit: pageSize,
        offset: (currentPage - 1) * pageSize
      };
      if (selectedCategoryId) {
        filters.category_id = selectedCategoryId;
        filters.include_subcategories = true;
      }
      // Use activeQuery for API calls
      if (activeQuery) {
        if (searchMode === 'ql') {
          // QL mode: pass query directly
          filters.ql = activeQuery;
        } else {
          // Simple mode: translate to title/description search
          const escapedInput = activeQuery.replace(/"/g, '\\"');
          filters.ql = `title ~ "${escapedInput}" OR description ~ "${escapedInput}"`;
        }
      }
      const result = await api.assets.getAll(selectedSetId, filters);
      assets = result?.assets || [];
      totalAssets = result?.total || 0;
    } catch (error) {
      console.error('Failed to load assets:', error);
    }
  }

  // Reload assets when filters change (reset to page 1)
  $effect(() => {
    if (selectedSetId) {
      // Dependencies: selectedCategoryId, activeQuery
      const _ = [selectedCategoryId, activeQuery];
      currentPage = 1;
      loadAssets();
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
    currentPage = page;
    loadAssets();
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
    try {
      const fields = await api.assetTypes.getFields(typeId);
      selectedTypeFields = fields || [];
    } catch (error) {
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
      asset_type_id: asset.asset_type_id ?? null,
      category_id: asset.category_id ?? null,
      status_id: asset.status_id ?? null,
      custom_field_values: { ...(asset.custom_field_values || {}) }
    };
  }

  async function handleAssetSubmit() {
    try {
      // Validate required custom fields
      for (const field of selectedTypeFields) {
        if (field.is_required) {
          const value = assetFormData.custom_field_values[field.custom_field_id];
          if (value === undefined || value === null || value === '') {
            alert(`"${field.field_name}" is required`);
            return;
          }
        }
      }

      if (editingAsset) {
        await api.assets.update(editingAsset.id, assetFormData);
      } else {
        await api.assets.create(selectedSetId, assetFormData);
      }
      await loadAssets();
      showAssetForm = false;
    } catch (error) {
      console.error('Failed to save asset:', error);
      alert('Failed to save asset: ' + error.message);
    }
  }

  async function deleteAsset(id) {
    if (confirm('Are you sure you want to delete this asset?')) {
      try {
        await api.assets.delete(id);
        await loadAssets();
        if (selectedAsset?.id === id) {
          selectedAsset = null;
        }
      } catch (error) {
        console.error('Failed to delete asset:', error);
        alert('Failed to delete asset: ' + error.message);
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

  // Helper to flatten categories for select
  function flattenCategories(categories, level = 0) {
    let result = [];
    for (const cat of categories) {
      result.push({ ...cat, level });
      if (cat.children?.length > 0) {
        result = result.concat(flattenCategories(cat.children, level + 1));
      }
    }
    return result;
  }

  const flatCategories = $derived(flattenCategories(assetCategories));

  // Column definitions for DataTable
  const assetColumns = [
    {
      key: 'title',
      label: 'NAME'
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
      render: (asset) => new Date(asset.created_at).toLocaleDateString()
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];

  function buildAssetDropdownItems(asset) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => showEditAssetForm(asset)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteAsset(asset.id)
      }
    ];
  }
</script>

<div class="flex h-full min-h-screen" style="background: var(--ds-surface);">
  <!-- Left sidebar: Category tree -->
  <div class="w-64 flex flex-col" style="border-right: 1px solid var(--ds-border); background: var(--ds-surface-raised);">
    <!-- Set selector -->
    <div class="p-4" style="border-bottom: 1px solid var(--ds-border);">
      <Select bind:value={selectedSetId} class="w-full">
        {#if assetSets.length === 0}
          <option value={null} disabled>No asset sets available</option>
        {:else}
          {#each assetSets as set}
            <option value={set.id}>{set.name}</option>
          {/each}
        {/if}
      </Select>
    </div>

    <!-- Category tree -->
    <div class="flex-1 overflow-auto p-4">
      <button
        class="w-full text-left px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-2"
        style={selectedCategoryId === null ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if (selectedCategoryId !== null) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if (selectedCategoryId !== null) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
        onclick={() => selectCategory(null)}
      >
        <Package class="w-4 h-4" />
        All Assets
      </button>

      {#if assetCategories.length > 0}
        <div class="mt-2">
          {#snippet renderCategoryNav(category, level = 0)}
            <div style="padding-left: {level * 16}px">
              <button
                class="w-full text-left px-3 py-1.5 rounded-lg text-sm font-medium transition-all flex items-center gap-1"
                style={selectedCategoryId === category.id ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
                onmouseenter={(e) => { if (selectedCategoryId !== category.id) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
                onmouseleave={(e) => { if (selectedCategoryId !== category.id) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
                onclick={() => selectCategory(category.id)}
              >
                {#if category.has_children}
                  <button
                    class="p-0.5 rounded"
                    style="background: transparent;"
                    onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-pressed)'}
                    onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
                    onclick={(e) => { e.stopPropagation(); toggleCategory(category.id); }}
                  >
                    {#if expandedCategories.has(category.id)}
                      <ChevronDown class="w-3 h-3" />
                    {:else}
                      <ChevronRight class="w-3 h-3" />
                    {/if}
                  </button>
                {:else}
                  <span class="w-4"></span>
                {/if}
                {#if expandedCategories.has(category.id)}
                  <FolderOpen class="w-4 h-4 text-yellow-500" />
                {:else}
                  <Folder class="w-4 h-4 text-yellow-500" />
                {/if}
                <span class="truncate">{category.name}</span>
                {#if category.asset_count > 0}
                  <span class="text-xs text-gray-400 ml-auto">{category.asset_count}</span>
                {/if}
              </button>
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
    <div class="p-4 flex items-center gap-4" style="border-bottom: 1px solid var(--ds-border);">
      <div class="flex-1 relative max-w-lg flex items-center gap-2">
        <div class="flex-1 relative">
          <Search class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2" style="color: var(--ds-icon);" />
          <input
            type="text"
            placeholder={searchMode === 'ql' ? 'Query: status = "Active" (press Enter)' : 'Search by name...'}
            bind:value={searchInput}
            onkeydown={(e) => { if (searchMode === 'ql' && e.key === 'Enter') activeQuery = searchInput; }}
            class="w-full pl-9 pr-4 py-2 rounded-lg text-sm {searchMode === 'ql' ? 'font-mono' : ''}"
            style="background: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
            title={searchMode === 'ql' ? 'QL Query - Press Enter to search. Examples: status = "Active", type IN ("Laptop", "Desktop"), title ~ "server"' : 'Search by title or description'}
          />
        </div>
        <button
          onclick={() => {
            searchMode = searchMode === 'simple' ? 'cql' : 'simple';
            searchInput = '';
            activeQuery = '';
          }}
          class="p-2 rounded-lg transition-colors"
          style="background: {searchMode === 'ql' ? 'var(--ds-interactive-subtle)' : 'var(--ds-background-input)'}; border: 1px solid {searchMode === 'ql' ? 'var(--ds-border-selected)' : 'var(--ds-border)'}; color: {searchMode === 'ql' ? 'var(--ds-interactive)' : 'var(--ds-text)'};"
          title={searchMode === 'ql' ? 'Switch to simple search' : 'Switch to QL query mode'}
        >
          <Code class="w-4 h-4" />
        </button>
      </div>
      <div class="flex-1"></div>
      {#if selectedSetId}
        <Button onclick={showAddAssetForm} class="whitespace-nowrap" keyboardHint="A">
          <Plus class="w-4 h-4 mr-1" />
          New Asset
        </Button>
      {/if}
    </div>

    <!-- Asset list -->
    <div class="flex-1 overflow-auto p-4">
      {#snippet createAssetAction()}
        <Button onclick={showAddAssetForm}>
          <Plus class="w-4 h-4 mr-1" />
          Create Asset
        </Button>
      {/snippet}

      {#if loading}
        <div class="flex items-center justify-center h-full">
          <div class="text-gray-500">Loading...</div>
        </div>
      {:else if assetSets.length === 0}
        <EmptyState
          icon={Package}
          title="No Asset Sets Available"
          description="You don't have access to any asset sets. Contact an administrator to get access."
        />
      {:else if assets.length === 0}
        <EmptyState
          icon={Box}
          title="No Assets Found"
          description={activeQuery || selectedCategoryId ? "Try adjusting your query or filters." : "Create your first asset to get started."}
          action={selectedSetId && !activeQuery && !selectedCategoryId ? createAssetAction : null}
        />
      {:else}
        <DataTable
          columns={assetColumns}
          data={assets}
          keyField="id"
          emptyMessage="No assets yet."
          emptyIcon={Box}
          actionItems={buildAssetDropdownItems}
          onRowClick={(asset) => selectedAsset = asset}
          pagination={true}
          {pageSize}
          bind:currentPage
          totalItems={totalAssets}
          onPageChange={handlePageChange}
        >
          <div slot="type" let:item class="flex items-center gap-2">
            {#if item.asset_type_name}
              <ColorDot color={item.asset_type_color || '#6b7280'} size="sm" />
              <span>{item.asset_type_name}</span>
            {:else}
              <span style="color: var(--ds-text-subtlest);">—</span>
            {/if}
          </div>

          <div slot="category" let:item>
            {#if item.category_name}
              <span class="inline-flex items-center gap-1">
                <Folder class="w-3 h-3 text-yellow-500" />
                {item.category_name}
              </span>
            {:else}
              <span style="color: var(--ds-text-subtlest);">—</span>
            {/if}
          </div>

          <div slot="status" let:item>
            {#if item.status_name}
              <span class="inline-flex items-center gap-1.5">
                <span class="w-2 h-2 rounded-full" style="background-color: {item.status_color || '#6b7280'};"></span>
                {item.status_name}
              </span>
            {:else}
              <span style="color: var(--ds-text-subtlest);">—</span>
            {/if}
          </div>
        </DataTable>
      {/if}
    </div>
  </div>

  <!-- Right sidebar: Asset detail (when selected) -->
  {#if selectedAsset}
    <div class="flex-shrink-0 flex flex-col relative" style="width: {assetPanelWidth}px; min-width: 280px; max-width: 600px; border-left: 1px solid var(--ds-border);">
      <!-- Resize handle -->
      <div
        class="absolute left-0 top-0 bottom-0 w-1 cursor-ew-resize transition-colors z-10"
        style="background-color: transparent;"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = '#3b82f6'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
        onmousedown={startAssetPanelResize}
      ></div>
      <div class="p-4 flex items-center justify-between" style="border-bottom: 1px solid var(--ds-border);">
        <h2 class="font-semibold truncate" style="color: var(--ds-text);">{selectedAsset.title}</h2>
        <button
          class="p-1 rounded"
          style="background: transparent;"
          onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-hovered)'}
          onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
          onclick={() => selectedAsset = null}
        >
          <ChevronRight class="w-4 h-4" style="color: var(--ds-icon);" />
        </button>
      </div>
      <div class="flex-1 overflow-auto p-4">
        {#if selectedAsset.description}
          <div class="mb-4">
            <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Description</h4>
            <p class="text-sm" style="color: var(--ds-text);">{selectedAsset.description}</p>
          </div>
        {/if}
        <div class="space-y-3">
          {#if selectedAsset.type_name}
            <div>
              <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Type</h4>
              <span class="inline-flex items-center gap-1" style="color: var(--ds-text);">
                <ColorDot color={selectedAsset.type_color || '#6b7280'} />
                {selectedAsset.type_name}
              </span>
            </div>
          {/if}
          {#if selectedAsset.category_name}
            <div>
              <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Category</h4>
              <span class="inline-flex items-center gap-1" style="color: var(--ds-text);">
                <Folder class="w-4 h-4 text-yellow-500" />
                {selectedAsset.category_name}
              </span>
            </div>
          {/if}
          {#if selectedAsset.status_name}
            <div>
              <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Status</h4>
              <span class="inline-flex items-center gap-1.5" style="color: var(--ds-text);">
                <span class="w-2 h-2 rounded-full" style="background-color: {selectedAsset.status_color || '#6b7280'};"></span>
                {selectedAsset.status_name}
              </span>
            </div>
          {/if}
          {#if selectedAsset.asset_tag}
            <div>
              <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Asset Tag</h4>
              <span class="text-sm font-mono" style="color: var(--ds-text);">{selectedAsset.asset_tag}</span>
            </div>
          {/if}
          {#if selectedAsset.creator_name}
            <div>
              <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Created By</h4>
              <span class="text-sm" style="color: var(--ds-text);">{selectedAsset.creator_name}</span>
            </div>
          {/if}
          <div>
            <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Created</h4>
            <span class="text-sm" style="color: var(--ds-text);">{new Date(selectedAsset.created_at).toLocaleDateString()}</span>
          </div>
          <div>
            <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Updated</h4>
            <span class="text-sm" style="color: var(--ds-text);">{new Date(selectedAsset.updated_at).toLocaleDateString()}</span>
          </div>
          {#if selectedAsset.linked_item_count > 0}
            <div>
              <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Linked Items</h4>
              <span class="text-sm" style="color: var(--ds-text);">{selectedAsset.linked_item_count}</span>
            </div>
          {/if}
        </div>
        {#if selectedAsset.custom_field_values && Object.keys(selectedAsset.custom_field_values).length > 0}
          <div class="border-t pt-4 mt-4" style="border-color: var(--ds-border);">
            <h4 class="text-xs font-medium uppercase mb-3" style="color: var(--ds-text-subtlest);">Custom Fields</h4>
            {#each Object.entries(selectedAsset.custom_field_values) as [fieldId, value]}
              {@const fieldDef = displayTypeFields.find(f => String(f.custom_field_id) === String(fieldId))}
              {#if fieldDef && value !== null && value !== ''}
                <div class="mb-3">
                  <CustomFieldRenderer
                    field={{
                      id: fieldDef.custom_field_id,
                      name: fieldDef.field_name,
                      field_type: fieldDef.field_type,
                      options: fieldDef.field_options
                    }}
                    value={value}
                    readonly={true}
                  />
                </div>
              {/if}
            {/each}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- Asset Form Modal -->
<Modal isOpen={showAssetForm} onclose={() => showAssetForm = false} onSubmit={handleAssetSubmit}>
  <ModalHeader title={editingAsset ? 'Edit Asset' : 'New Asset'} onClose={() => showAssetForm = false} />
  <form onsubmit={(e) => { e.preventDefault(); handleAssetSubmit(); }} class="p-6">
    <div class="space-y-4">
      <div>
        <Label color="default" class="mb-1">Title</Label>
        <input
          type="text"
          bind:value={assetFormData.title}
          required
          class="w-full px-3 py-2 rounded-lg"
          style="background: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        />
      </div>
      <div>
        <Label color="default" class="mb-1">Description</Label>
        <textarea
          bind:value={assetFormData.description}
          rows="3"
          class="w-full px-3 py-2 rounded-lg"
          style="background: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        ></textarea>
      </div>
      <div>
        <Label color="default" class="mb-1">Asset Type</Label>
        <Select bind:value={assetFormData.asset_type_id}>
          <option value={null}>No Type</option>
          {#each assetTypes as type}
            <option value={type.id}>{type.name}</option>
          {/each}
        </Select>
      </div>
      <div>
        <Label color="default" class="mb-1">Category</Label>
        <Select bind:value={assetFormData.category_id}>
          <option value={null}>No Category</option>
          {#each flatCategories as cat}
            <option value={cat.id}>{'  '.repeat(cat.level)}{cat.name}</option>
          {/each}
        </Select>
      </div>
      <div>
        <Label color="default" class="mb-1">Status</Label>
        <Select bind:value={assetFormData.status_id}>
          {#each statuses as status}
            <option value={status.id}>{status.name}</option>
          {/each}
        </Select>
      </div>
      {#if selectedTypeFields.length > 0}
        <div class="border-t pt-4 mt-4" style="border-color: var(--ds-border);">
          <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Custom Fields</h4>
          {#each selectedTypeFields as field}
            <div class="mb-4">
              <CustomFieldRenderer
                field={{
                  id: field.custom_field_id,
                  name: field.field_name,
                  field_type: field.field_type,
                  options: field.field_options
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
      <Button variant="outline" type="button" onclick={() => showAssetForm = false} keyboardHint="Esc">Cancel</Button>
      <Button type="submit" keyboardHint="↵">{editingAsset ? 'Save' : 'Create'}</Button>
    </div>
  </form>
</Modal>

