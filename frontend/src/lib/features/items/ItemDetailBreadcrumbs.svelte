<script>
  import { FileText, Edit3, X, Check, Search } from 'lucide-svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import { createEventDispatcher } from 'svelte';
  import { api } from '../../api.js';

  const dispatch = createEventDispatcher();
  
  let { 
  workspace,
  parentHierarchy = [],
  currentItemType,
  currentHierarchyLevel,
  item,
  iconMap,
  workspaceId
} = $props();
  
  // We need access to item types to filter by hierarchy level
  let itemTypes = [];
  let validParentHierarchyLevel = null;
  
  // Parent editing state
  let showParentSelector = $state(false);
  let searchQuery = $state('');
  let searchResults = $state([]);
  let searching = $state(false);
  let saving = $state(false);
  let searchTimeout;
  
  function navigate(path) {
    dispatch('navigate', { path });
  }

  function getItemTypeInfo(itemTypeId) {
    if (!itemTypeId || !itemTypes.length) return null;
    return itemTypes.find(type => type.id === itemTypeId);
  }

  async function openParentSelector() {
    showParentSelector = true;
    searchQuery = '';
    searchResults = [];
    
    // Load item types and calculate valid parent hierarchy level
    try {
      itemTypes = await api.itemTypes.getAll();
      
      // Calculate the valid parent hierarchy level (current level - 1)
      if (currentItemType && currentHierarchyLevel) {
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
        
        // Further filter by hierarchy level if we have the information
       if (validParentHierarchyLevel !== null && itemTypes.length > 0) {
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
      dispatch('parent-changed');
      closeParentSelector();
    } catch (error) {
      console.error('Failed to update parent:', error);
      alert('Failed to update parent: ' + (error.message || error));
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
      
      // Dispatch event to parent component to reload data
      dispatch('parent-changed');
      closeParentSelector();
    } catch (error) {
      console.error('Failed to remove parent:', error);
      alert('Failed to remove parent: ' + (error.message || error));
    } finally {
      saving = false;
    }
  }
</script>

<!-- Breadcrumb Navigation -->
<div class="group flex items-center gap-2 text-sm mb-6 overflow-hidden" style="color: var(--ds-text-subtle);">
  <button
    onclick={() => navigate('/workspaces')}
    class="transition-colors hover:underline"
  >
    Workspaces
  </button>
  <span>/</span>
  <button
    onclick={() => navigate(`/workspaces/${workspaceId}`)}
    class="transition-colors hover:underline"
  >
    {workspace.name}
  </button>
  <span>/</span>
  <button
    onclick={() => navigate(`/workspaces/${workspaceId}/collections/default/list`)}
    class="transition-colors hover:underline"
  >
    Work Items
  </button>
  <span>/</span>
  <!-- Related Work Item link (for personal tasks) -->
  {#if workspace?.is_personal && item.related_work_item_id}
    <div class="flex items-center gap-1.5">
      <span class="text-xs italic" style="color: var(--ds-text-subtlest);">linked to</span>
      <button
        onclick={() => navigate(`/workspaces/${item.related_work_item_workspace_id}/items/${item.related_work_item_id}`)}
        class="transition-colors flex items-center gap-1.5 hover:underline"
        title="Go to linked work item"
      >
        <span class="text-xs px-1.5 py-0.5 rounded font-mono" style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);">
          {item.related_work_item_workspace_key}-{item.related_work_item_number}
        </span>
        <span class="truncate max-w-48">{item.related_work_item_title}</span>
      </button>
    </div>
    <span>/</span>
  {/if}
  <!-- Parent Hierarchy in breadcrumb -->
  {#if parentHierarchy.length > 0}
    {#each parentHierarchy as parent}
      <div class="flex items-center gap-2">
        {#if parent.itemType}
          <Tooltip content="{parent.itemType.name}">
            {#snippet children()}
              <div 
                class="w-4 h-4 rounded flex items-center justify-center text-white text-xs cursor-help"
                style="background-color: {parent.itemType.color};"
              >
                <svelte:component this={iconMap[parent.itemType.icon] || FileText} class="w-3 h-3" />
              </div>
            {/snippet}
          </Tooltip>
        {/if}
        <button
          onclick={() => navigate(`/workspaces/${parent.workspace_id}/items/${parent.id}`)}
          class="transition-colors hover:underline"
          title="Go to {parent.title}"
        >
          {parent.title}
        </button>
      </div>
      <span>/</span>
    {/each}
  {:else if !item.parent_id && !(workspace?.is_personal && item.related_work_item_id)}
    <!-- Show placeholder for "no parent" scenario (not shown for personal tasks with linked work items) -->
    <span class="italic" style="color: var(--ds-text-subtlest);">No parent</span>
    <span>/</span>
  {/if}

  <!-- Edit Parent Button (hidden for personal tasks with linked work items) -->
  {#if !(workspace?.is_personal && item.related_work_item_id)}
  <div class="relative">
    <div class="overflow-hidden transition-all duration-200 w-0 group-hover:w-4">
      <button
        onclick={openParentSelector}
        class="w-4 h-4 rounded transition-colors flex items-center justify-center"
        style="color: var(--ds-text-subtlest);"
        title={parentHierarchy.length > 0 ? "Change parent" : "Set parent"}
        disabled={saving}
      >
        <Edit3 class="w-3 h-3" />
      </button>
    </div>

    <!-- Parent Selector Popover -->
    {#if showParentSelector}
      <div
        class="absolute left-0 top-6 w-80 rounded shadow-lg border z-50"
        style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); backdrop-filter: blur(8px);"
      >
        <!-- Header -->
        <div class="flex items-center justify-between p-3 border-b" style="border-color: var(--ds-border);">
          <h3 class="font-medium" style="color: var(--ds-text);">
            {parentHierarchy.length > 0 ? 'Change Parent' : 'Set Parent'}
          </h3>
          <button
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
            <input
              type="text"
              bind:value={searchQuery}
              placeholder="Search for parent item..."
              class="w-full pl-9 pr-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            />
          </div>
          {#if validParentHierarchyLevel !== null}
            <div class="mt-2 text-xs" style="color: var(--ds-text-subtle);">
              Only showing items from hierarchy level {validParentHierarchyLevel}
              {#if currentHierarchyLevel}
                (one level above {currentHierarchyLevel.name})
              {/if}
            </div>
          {:else}
            <div class="mt-2 text-xs" style="color: var(--ds-text-subtle);">
              Search for parent item across workspaces
            </div>
          {/if}
        </div>

        <!-- Results -->
        <div class="max-h-60 overflow-y-auto">
          {#if parentHierarchy.length > 0}
            <!-- Remove parent option -->
            <button
              onclick={removeParent}
              disabled={saving}
              class="w-full px-3 py-2 text-left border-b text-red-600 hover:text-red-700 disabled:opacity-50"
              style="border-color: var(--ds-border);"
            >
              <div class="flex items-center gap-2">
                <X class="w-4 h-4" />
                <span class="text-sm">Remove parent</span>
              </div>
            </button>
          {/if}

          {#if searching}
            <div class="p-3 text-center text-sm" style="color: var(--ds-text-subtle);">
              Searching...
            </div>
          {:else if searchQuery.length >= 2 && searchResults.length === 0}
            <div class="p-3 text-center text-sm" style="color: var(--ds-text-subtle);">
              {#if validParentHierarchyLevel !== null}
                No items found at hierarchy level {validParentHierarchyLevel}
              {:else}
                No items found
              {/if}
            </div>
          {:else if searchQuery.length < 2}
            <div class="p-3 text-center text-sm" style="color: var(--ds-text-subtle);">
              Type at least 2 characters to search
            </div>
          {:else}
            {#each searchResults as result}
              {@const resultItemType = getItemTypeInfo(result.item_type_id)}
              <button
                onclick={() => selectParent(result)}
                disabled={saving}
                class="w-full px-3 py-2 text-left border-b last:border-b-0 disabled:opacity-50"
                style="border-color: var(--ds-border);"
              >
                <div class="flex items-center gap-2">
                  <!-- Item Type Icon -->
                  {#if resultItemType}
                    <div
                      class="w-4 h-4 rounded flex items-center justify-center text-white text-xs flex-shrink-0"
                      style="background-color: {resultItemType.color};"
                      title={resultItemType.name}
                    >
                      <svelte:component this={iconMap[resultItemType.icon] || FileText} class="w-3 h-3" />
                    </div>
                  {/if}

                  <!-- Item Key -->
                  <div class="flex-shrink-0">
                    <ItemKey item={result} workspace={result.workspace_key ? { key: result.workspace_key } : workspace} style="color: var(--ds-text-subtle);" />
                  </div>

                  <!-- Title -->
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium truncate" style="color: var(--ds-text);">{result.title}</div>
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
  <div class="flex items-center gap-2 min-w-0 flex-1" style="color: var(--ds-text);">
    {#if currentItemType}
      <Tooltip content="{currentItemType.name} ({currentHierarchyLevel?.name || 'Unknown level'})">
        {#snippet children()}
          <div
            class="w-4 h-4 rounded flex items-center justify-center text-white text-xs cursor-help"
            style="background-color: {currentItemType.color};"
          >
            <svelte:component this={iconMap[currentItemType.icon] || FileText} class="w-3 h-3" />
          </div>
        {/snippet}
      </Tooltip>
    {/if}
    <Tooltip content="Click to copy key to clipboard">
      {#snippet children()}
        <button
          onclick={() => dispatch('copy-key')}
          class="text-xs px-2 py-1 rounded transition-colors cursor-pointer flex-shrink-0 whitespace-nowrap"
          style="background-color: var(--ds-surface); color: var(--ds-text);"
        >
          {workspace?.key || "WORK"}-{item.workspace_item_number}
        </button>
      {/snippet}
    </Tooltip>
    <span class="truncate">{item.title}</span>
  </div>
</div>
