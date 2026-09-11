<script>
  import { AlertTriangle, Check, Edit3, X, Search } from '@lucide/svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import Input from '../../components/Input.svelte';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import ItemDetailBreadcrumbLevel from './ItemDetailBreadcrumbLevel.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import {
    canItemTypeBeChildOf,
    isGenericSubtaskType,
  } from '../../utils/hierarchy.js';

  let {
  workspace,
  parentHierarchy = [],
  currentItemType,
  currentHierarchyLevel,
  item,
  workspaceId,
  itemTypes: providedItemTypes = [],
  onnavigate = null,
  onparentChanged = null,
  oncopyKey = null,
  onitemtypechange = null
} = $props();
  
  // We need access to item types to filter by hierarchy level
  let itemTypes = $state([]);
  let validParentHierarchyLevel = $state(null);
  let showItemTypeSelector = $state(false);
  
  // Parent editing state
  let showParentSelector = $state(false);
  let searchQuery = $state('');
  let searchResults = $state([]);
  let searching = $state(false);
  let saving = $state(false);
  let searchTimeout;
  
  function navigate(path) {
    onnavigate?.(path);
  }

  let effectiveItemTypes = $derived(providedItemTypes?.length ? providedItemTypes : itemTypes);
  let directParent = $derived(parentHierarchy.length > 0 ? parentHierarchy[parentHierarchy.length - 1] : null);
  let currentItemHierarchyLevel = $derived(currentItemType?.hierarchy_level ?? currentHierarchyLevel?.level ?? null);
  let parentHierarchyLevel = $derived(directParent?.itemType?.hierarchy_level ?? null);
  let currentItemIsGenericSubtask = $derived(isGenericSubtaskType(currentItemType));
  let canEditParent = $derived(
    parentHierarchy.length > 0 || currentItemIsGenericSubtask || currentItemHierarchyLevel > 0
  );
  let hasHierarchyMismatch = $derived(
    currentItemHierarchyLevel !== null &&
    parentHierarchyLevel !== null &&
    (currentItemIsGenericSubtask
      ? isGenericSubtaskType(directParent?.itemType)
      : parentHierarchyLevel !== currentItemHierarchyLevel - 1)
  );
  let compatibleHierarchyLevel = $derived(parentHierarchyLevel !== null ? parentHierarchyLevel + 1 : null);
  let displayedItemTypes = $derived.by(() => {
    const types = effectiveItemTypes.filter(type => directParent || !isGenericSubtaskType(type));
    if (!hasHierarchyMismatch || compatibleHierarchyLevel === null) return types;

    return types.sort((a, b) => {
      const compatibilityOrder =
        Number(canItemTypeBeChildOf(b, directParent?.itemType)) -
        Number(canItemTypeBeChildOf(a, directParent?.itemType));
      if (compatibilityOrder !== 0) return compatibilityOrder;
      if (a.hierarchy_level !== b.hierarchy_level) return a.hierarchy_level - b.hierarchy_level;
      return (a.sort_order ?? 0) - (b.sort_order ?? 0);
    });
  });

  function getItemTypeInfo(itemTypeId) {
    if (!itemTypeId || !effectiveItemTypes.length) return null;
    return effectiveItemTypes.find(type => type.id === itemTypeId);
  }

  async function openItemTypeSelector() {
    showItemTypeSelector = true;
    if (effectiveItemTypes.length > 0) return;
    try {
      itemTypes = await api.itemTypes.getAll();
    } catch (error) {
      console.error('Failed to load item types:', error);
    }
  }

  function closeItemTypeSelector() {
    showItemTypeSelector = false;
  }

  function selectItemType(type) {
    if (!type || type.id === item.item_type_id) {
      closeItemTypeSelector();
      return;
    }
    closeItemTypeSelector();
    onitemtypechange?.(type);
  }

  async function openCompatibleItemTypes() {
    closeParentSelector();
    await openItemTypeSelector();
  }

  async function openParentSelector() {
    showParentSelector = true;
    searchQuery = '';
    searchResults = [];
    validParentHierarchyLevel = null;
    
    // Load item types and calculate valid parent hierarchy level
    try {
      itemTypes = await api.itemTypes.getAll();
      
      // Calculate the valid parent hierarchy level (current level - 1)
      if (
        currentItemType &&
        currentHierarchyLevel &&
        currentItemHierarchyLevel > 0 &&
        !currentItemIsGenericSubtask
      ) {
        validParentHierarchyLevel = currentHierarchyLevel.level - 1;
      }
    } catch (error) {
      console.error('Failed to load item types:', error);
    }
  }
  
  function closeParentSelector() {
    showParentSelector = false;
    searchQuery = '';
    searchResults = [];
    clearTimeout(searchTimeout);
  }
  
  // Reactive search when query changes
  $effect(() => {
  if (searchQuery && searchQuery.length >= 2) {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(async () => {
      try {
        searching = true;
        const results = await api.search.items({
          query: searchQuery,
          // Don't restrict to current workspace - allow cross-workspace parents
          limit: 20 // Get more results since we'll filter them
        });
        
        // Filter out the current item and any existing parents to prevent cycles
        const currentItemId = item.id;
        const parentIds = new Set(parentHierarchy.map(p => p.id));
        parentIds.add(currentItemId);
        
        let filteredResults = (results || []).filter(result => !parentIds.has(result.id));
        
        // Generic subtasks accept a parent at any regular level. Fixed types
        // retain the adjacent-level rule.
        if (currentItemIsGenericSubtask && itemTypes.length > 0) {
          filteredResults = filteredResults.filter(result => {
            const resultItemType = itemTypes.find(type => type.id === result.item_type_id);
            return resultItemType && !isGenericSubtaskType(resultItemType);
          });
        } else if (currentItemHierarchyLevel === 0) {
          filteredResults = [];
        } else if (validParentHierarchyLevel !== null && itemTypes.length > 0) {
          filteredResults = filteredResults.filter(result => {
            if (!result.item_type_id) return false;
            
            const resultItemType = itemTypes.find(type => type.id === result.item_type_id);
            return resultItemType && resultItemType.hierarchy_level === validParentHierarchyLevel;
          });
        }
        
        // Limit to 10 results after filtering
        searchResults = filteredResults.slice(0, 10);
      } catch (error) {
        console.error('Search failed:', error);
        searchResults = [];
      } finally {
        searching = false;
      }
    }, 300);
  } else {
    searchResults = [];
    searching = false;
  }
});
  
  async function selectParent(selectedItem) {
    if (saving) return;
    
    try {
      saving = true;
      await api.items.update(item.id, {
        parent_id: selectedItem.id
      });
      
      // Dispatch event to parent component to reload data
      onparentChanged?.();
      closeParentSelector();
    } catch (error) {
      console.error('Failed to update parent:', error);
      errorToast(t('items.failedToUpdateParent') + ': ' + (error.message || error));
    } finally {
      saving = false;
    }
  }

  async function removeParent() {
    if (saving) return;

    try {
      saving = true;
      await api.items.update(item.id, {
        parent_id: null
      });

      onparentChanged?.();
      closeParentSelector();
    } catch (error) {
      console.error('Failed to remove parent:', error);
      errorToast(t('items.failedToRemoveParent') + ': ' + (error.message || error));
    } finally {
      saving = false;
    }
  }
</script>

<!-- Breadcrumb Navigation -->
<div
  data-testid="item-detail-breadcrumbs"
  class="group flex items-center gap-2 text-sm {hasHierarchyMismatch ? 'mb-3' : 'mb-6'} min-w-0 overflow-visible flex-nowrap"
  style="color: var(--ds-text-subtle);"
>
  <a
    href={workspace?.is_personal ? '/personal' : `/workspaces/${workspaceId}`}
    class="transition-colors hover:underline flex-shrink-0 no-underline"
    style="color: inherit;"
  >
    {workspace.name}
  </a>
  <span class="flex-shrink-0">/</span>
  <!-- Related Work Item link (for personal tasks) -->
  {#if workspace?.is_personal && item.related_work_item_id}
    <div class="flex items-center gap-1.5">
      <span class="text-xs italic" style="color: var(--ds-text-subtlest);">{t('items.linkedTo')}</span>
      <a
        href={`/workspaces/${item.related_work_item_workspace_id}/items/${item.related_work_item_id}`}
        class="transition-colors flex items-center gap-1.5 hover:underline no-underline"
        style="color: inherit;"
        title={t('items.goToLinkedWorkItem')}
      >
        <span class="text-xs px-1.5 py-0.5 rounded font-mono" style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);">
          {item.related_work_item_workspace_key}-{item.related_work_item_number}
        </span>
        <span class="truncate max-w-48">{item.related_work_item_title}</span>
      </a>
    </div>
    <span class="flex-shrink-0">/</span>
  {/if}
  <!-- Parent Hierarchy in breadcrumb -->
  {#if parentHierarchy.length > 0}
    {#each parentHierarchy as parent}
      <ItemDetailBreadcrumbLevel
        itemType={parent.itemType}
        iconSlotTestId={`item-parent-type-icon-slot-${parent.id}`}
        iconTestId={`item-parent-type-icon-${parent.id}`}
      >
        <a
          data-testid={`item-parent-breadcrumb-${parent.id}`}
          href={`/workspaces/${parent.workspace_id}/items/${parent.id}`}
          class="transition-colors hover:underline truncate max-w-24 no-underline"
          style="color: inherit;"
          title={t('items.goTo', { title: parent.title })}
        >
          {parent.title}
        </a>
      </ItemDetailBreadcrumbLevel>
      <span class="flex-shrink-0">/</span>
    {/each}
  {:else if !item.parent_id && !(workspace?.is_personal && item.related_work_item_id)}
    <!-- Show placeholder for "no parent" scenario (not shown for personal tasks with linked work items) -->
    {#if canEditParent}
      <button
        onclick={openParentSelector}
        class="italic transition-colors hover:underline"
        style="color: var(--ds-text-subtlest);"
        title={t('items.setParent')}
      >
        {t('items.noParent')}
      </button>
    {:else}
      <span class="italic" style="color: var(--ds-text-subtlest);">{t('items.noParent')}</span>
    {/if}
    <span class="flex-shrink-0">/</span>
  {/if}

  <!-- Edit Parent Button (hidden for personal tasks with linked work items) -->
  {#if canEditParent && !(workspace?.is_personal && item.related_work_item_id)}
  <div class="relative">
    <div class="overflow-hidden transition-all duration-200 w-0 group-hover:w-4">
      <button
        data-testid="item-parent-edit"
        onclick={openParentSelector}
        class="w-4 h-4 rounded transition-colors flex items-center justify-center"
        style="color: var(--ds-text-subtlest);"
        title={parentHierarchy.length > 0 ? t('items.changeParent') : t('items.setParent')}
        disabled={saving}
      >
        <Edit3 class="w-3 h-3" />
      </button>
    </div>

    <!-- Parent Selector Popover -->
    {#if showParentSelector}
      <div
        data-testid="item-parent-selector"
        class="absolute left-0 top-6 w-96 rounded shadow-lg border z-50"
        style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); backdrop-filter: blur(8px);"
      >
        <!-- Header -->
        <div class="flex items-center justify-between p-3 border-b" style="border-color: var(--ds-border);">
          <h3 class="font-medium" style="color: var(--ds-text);">
            {parentHierarchy.length > 0 ? t('items.changeParent') : t('items.setParent')}
          </h3>
          <button
            data-testid="item-parent-selector-close"
            onclick={closeParentSelector}
            class="w-6 h-6 rounded transition-colors flex items-center justify-center"
            style="color: var(--ds-text-subtle);"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <!-- Search Input -->
        <div class="p-3 border-b" style="border-color: var(--ds-border);">
          <div class="relative">
            <Search class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4" style="color: var(--ds-text-subtlest);" />
            <Input
              dataTestid="item-parent-search"
              type="text"
              bind:value={searchQuery}
              placeholder={t('items.searchForParentItem')}
              class="w-full pl-9 pr-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
              size="small"
            />
          </div>
          {#if currentItemIsGenericSubtask}
            <div
              data-testid="item-parent-generic-subtask-hint"
              class="mt-2 text-xs"
              style="color: var(--ds-text-subtle);"
            >
              {t('items.genericSubtaskParentHint')}
            </div>
          {:else if validParentHierarchyLevel !== null}
            <div
              data-testid="item-parent-level-hint"
              data-parent-level={validParentHierarchyLevel}
              class="mt-2 text-xs"
              style="color: var(--ds-text-subtle);"
            >
              {t('items.showingItemsFromLevel', { level: validParentHierarchyLevel })}
              {#if currentHierarchyLevel}
                ({t('items.oneLevelAbove', { name: currentHierarchyLevel.name })})
              {/if}
            </div>
          {:else}
            <div class="mt-2 text-xs" style="color: var(--ds-text-subtle);">
              {t('items.searchParentAcrossWorkspaces')}
            </div>
          {/if}
          {#if hasHierarchyMismatch}
            <div
              data-testid="item-parent-hierarchy-mismatch"
              class="mt-3 rounded-md border p-3"
              style="border-color: var(--ds-border-warning); background: var(--ds-background-warning-subtle, var(--ds-surface));"
            >
              <div class="flex items-start gap-2">
                <AlertTriangle class="mt-0.5 h-4 w-4 flex-shrink-0" style="color: var(--ds-icon-warning);" />
                <div class="min-w-0">
                  <p class="text-xs font-medium" style="color: var(--ds-text);">
                    {t('items.parentPickerMismatchTitle')}
                  </p>
                  <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
                    {t('items.parentPickerMismatchDescription', {
                      type: currentItemType?.name,
                      requiredLevel: validParentHierarchyLevel,
                      currentParentLevel: parentHierarchyLevel,
                      compatibleLevel: compatibleHierarchyLevel
                    })}
                  </p>
                  <button
                    type="button"
                    data-testid="item-parent-change-type"
                    onclick={openCompatibleItemTypes}
                    class="mt-2 text-xs font-medium underline underline-offset-2"
                    style="color: var(--ds-link);"
                  >
                    {t('items.chooseCompatibleType')}
                  </button>
                </div>
              </div>
            </div>
          {/if}
        </div>

        <!-- Results -->
        <div class="max-h-60 overflow-y-auto">
          {#if parentHierarchy.length > 0 && !currentItemIsGenericSubtask}
            <!-- Remove parent option -->
            <button
              onclick={removeParent}
              disabled={saving}
              class="w-full px-3 py-2 text-left border-b text-ds-text-danger hover:opacity-80 disabled:opacity-50"
              style="border-color: var(--ds-border);"
            >
              <div class="flex items-center gap-2">
                <X class="w-4 h-4" />
                <span class="text-sm">{t('items.removeParent')}</span>
              </div>
            </button>
          {/if}

          {#if searching}
            <div class="p-3 text-center text-sm" style="color: var(--ds-text-subtle);">
              {t('common.searching')}
            </div>
          {:else if searchQuery.length >= 2 && searchResults.length === 0}
            <div data-testid="item-parent-no-results" class="p-3 text-center text-sm" style="color: var(--ds-text-subtle);">
              {#if validParentHierarchyLevel !== null}
                {t('items.noItemsAtLevel', { level: validParentHierarchyLevel })}
              {:else}
                {t('common.noItemsFound')}
              {/if}
            </div>
          {:else if searchQuery.length < 2}
            <div class="p-3 text-center text-sm" style="color: var(--ds-text-subtle);">
              {t('common.typeToSearch')}
            </div>
          {:else}
            {#each searchResults as result}
              {@const resultItemType = getItemTypeInfo(result.item_type_id)}
              <button
                data-testid={`item-parent-result-${result.id}`}
                onclick={() => selectParent(result)}
                disabled={saving}
                class="w-full px-3 py-2 text-left border-b last:border-b-0 disabled:opacity-50"
                style="border-color: var(--ds-border);"
              >
                <div class="flex items-center gap-2">
                  <!-- Item Type Icon -->
                  {#if resultItemType}
                    <ItemTypeIcon itemType={resultItemType} />
                  {/if}

                  <!-- Item Key -->
                  <div class="flex-shrink-0">
                    <ItemKey item={result} workspace={result.workspace_key ? { key: result.workspace_key } : workspace} />
                  </div>

                  <!-- Title -->
                  <div class="flex-1 min-w-0">
                    <div class="text-sm truncate" style="color: var(--ds-text);">{result.title}</div>
                  </div>
                </div>
              </button>
            {/each}
          {/if}
        </div>
      </div>
    {/if}
  </div>
  {/if}
  <ItemDetailBreadcrumbLevel
    itemType={currentItemType}
    iconTooltip={currentItemType ? `${currentItemType.name} (${currentHierarchyLevel?.name || 'Unknown level'}) — click to change` : undefined}
    iconTitle="Change item type"
    iconSlotTestId="item-type-icon-slot"
    iconTriggerTestId="item-type-change-trigger"
    iconTestId="item-type-change-icon"
    oniconclick={openItemTypeSelector}
    class="flex-1"
    style="color: var(--ds-text);"
  >
    {#snippet iconOverlay()}
      {#if currentItemType}
        {#if showItemTypeSelector}
          <div
            data-testid="item-type-selector"
            class="absolute left-0 top-6 w-96 rounded shadow-lg border z-50"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); backdrop-filter: blur(8px);"
          >
            <div class="flex items-center justify-between p-3 border-b" style="border-color: var(--ds-border);">
              <h3 class="font-medium" style="color: var(--ds-text);">Change item type</h3>
              <button
                type="button"
                onclick={closeItemTypeSelector}
                class="w-6 h-6 rounded transition-colors flex items-center justify-center"
                style="color: var(--ds-text-subtle);"
              >
                <X class="w-4 h-4" />
              </button>
            </div>
            {#if hasHierarchyMismatch}
              <div
                data-testid="item-type-compatibility-hint"
                data-compatible-level={compatibleHierarchyLevel}
                class="border-b px-3 py-2 text-xs"
                style="border-color: var(--ds-border); color: var(--ds-text-subtle); background: var(--ds-background-warning-subtle, var(--ds-surface));"
              >
                {t('items.compatibleTypeHint', {
                  parentLevel: parentHierarchyLevel,
                  compatibleLevel: compatibleHierarchyLevel
                })}
              </div>
            {/if}
            <div class="max-h-72 overflow-y-auto py-1">
              {#each displayedItemTypes as type (type.id)}
                {@const fitsCurrentParent = directParent?.itemType && canItemTypeBeChildOf(type, directParent.itemType)}
                <button
                  type="button"
                  data-testid={`item-type-option-${type.id}`}
                  data-parent-compatible={fitsCurrentParent}
                  onclick={() => selectItemType(type)}
                  disabled={type.id === item.item_type_id}
                  class="w-full px-3 py-2 text-left flex items-center gap-2 disabled:opacity-50"
                  style:background={hasHierarchyMismatch && fitsCurrentParent ? 'var(--ds-background-success-subtle, var(--ds-surface))' : undefined}
                >
                  <ItemTypeIcon itemType={type} />
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-sm" style="color: var(--ds-text);">{type.name}</span>
                    <span
                      data-testid={`item-type-level-${type.id}`}
                      class="block text-xs"
                      style="color: var(--ds-text-subtle);"
                    >
                      {isGenericSubtaskType(type)
                        ? t('items.genericSubtaskLevelLabel')
                        : t('items.hierarchyLevelLabel', { level: type.hierarchy_level })}
                    </span>
                  </span>
                  {#if hasHierarchyMismatch && fitsCurrentParent}
                    <span
                      data-testid={`item-type-compatible-${type.id}`}
                      class="flex flex-shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
                      style="background: var(--ds-background-success); color: var(--ds-text-success);"
                    >
                      <Check class="h-3 w-3" />
                      {t('items.fitsCurrentParent')}
                    </span>
                  {/if}
                </button>
              {/each}
            </div>
          </div>
        {/if}
      {/if}
    {/snippet}
    <Tooltip content={t('items.clickToCopyKey')}>
      {#snippet children()}
        <button
          onclick={() => oncopyKey?.()}
          data-testid="item-copy-key"
          class="text-xs px-2 py-1 rounded transition-colors cursor-pointer flex-shrink-0 whitespace-nowrap"
          style="background-color: var(--ds-surface); color: var(--ds-text);"
        >
          {item.workspace_key || workspace?.key || "WORK"}-{item.workspace_item_number}
        </button>
      {/snippet}
    </Tooltip>
    <span class="truncate">{item.title}</span>
  </ItemDetailBreadcrumbLevel>
</div>

{#if hasHierarchyMismatch}
  <div
    data-testid="item-hierarchy-mismatch"
    data-current-level={currentItemHierarchyLevel}
    data-parent-level={parentHierarchyLevel}
    class="mb-6 flex items-start gap-3 rounded-md border px-4 py-3"
    style="border-color: var(--ds-border-warning); background: var(--ds-background-warning-subtle, var(--ds-surface-raised));"
  >
    <AlertTriangle class="mt-0.5 h-5 w-5 flex-shrink-0" style="color: var(--ds-icon-warning);" />
    <div class="min-w-0 flex-1">
      <p class="text-sm font-medium" style="color: var(--ds-text);">
        {t('items.hierarchyMismatchTitle')}
      </p>
      <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
        {t('items.hierarchyMismatchDescription', {
          type: currentItemType?.name,
          currentLevel: currentItemHierarchyLevel,
          parentLevel: parentHierarchyLevel,
          requiredLevel: currentItemHierarchyLevel - 1,
          compatibleLevel: compatibleHierarchyLevel
        })}
      </p>
    </div>
    <button
      type="button"
      data-testid="item-hierarchy-change-type"
      onclick={openCompatibleItemTypes}
      class="flex-shrink-0 rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
      style="background: var(--ds-background-neutral); color: var(--ds-text);"
    >
      {t('items.chooseCompatibleType')}
    </button>
  </div>
{/if}
