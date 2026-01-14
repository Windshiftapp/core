<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Search, Calendar, User, Eye, Edit, Trash2, MoreHorizontal, Building, AlertCircle } from 'lucide-svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import WorkItemFilter from '../features/items/WorkItemFilter.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { formatDate } from '../utils/dateFormatter.js';
  import { searchStore } from '../stores/searchStore.svelte.js';
  import Spinner from '../components/Spinner.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import { getStatusStyle } from '../utils/statusColors.js';

  // Subscribe to the entire store state
  let state = {};
  const unsubscribe = searchStore.subscribe(value => state = value);

  // Reactive shorthand for commonly used values
  $: ({ searchResults, loading, workspaces, hasFilters, error: cqlError } = state);

  onMount(async () => {
    // Load reference data (workspaces, statuses, priorities)
    await searchStore.loadReferenceData();

    // Restore filter state from URL
    searchStore.restoreFromURL();

    // Execute search if we have filters from URL
    if (state.hasFilters || state.manualQlQuery) {
      await searchStore.executeSearch();
    }

    // Cleanup on unmount
    return () => {
      unsubscribe();
      searchStore.destroy();
    };
  });

  function getPriorityColor(priority) {
    const colors = {
      low: 'text-gray-500',
      medium: 'text-blue-500',
      high: 'text-orange-500',
      critical: 'text-red-500'
    };
    return colors[priority] || 'text-gray-500';
  }

  function viewItem(item) {
    // Navigate to item detail page
    window.location.href = `/workspaces/${item.workspace_id}/items/${item.id}`;
  }

  function editItem(item) {
    // Navigate to item edit page
    window.location.href = `/workspaces/${item.workspace_id}/items/${item.id}`;
  }

  async function deleteItem(item) {
    if (!confirm(`Are you sure you want to delete "${item.title}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await api.items.delete(item.id);
      // Refresh search results
      await searchStore.executeSearch();
    } catch (error) {
      console.error('Failed to delete item:', error);
      alert('Failed to delete item: ' + (error.message || error));
    }
  }

  function buildItemActions(item) {
    return [
      {
        id: 'view',
        type: 'regular',
        icon: Eye,
        title: 'View Details',
        onClick: () => viewItem(item)
      },
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        onClick: () => editItem(item)
      },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50 hover:text-red-700',
        onClick: () => deleteItem(item)
      }
    ];
  }

</script>

<div class="min-h-screen" style="background-color: var(--ds-surface);">
  <div class="p-6">
    <!-- Header -->
    <PageHeader
      icon={Search}
      title="Search Work Items"
      subtitle="Search across all workspaces with advanced filtering options"
    />

    <!-- Work Item Filter Component -->
    <div class="mb-6">
      <WorkItemFilter />

      <!-- Results Count -->
      {#if searchResults.length > 0}
        <div class="text-sm mb-4" style="color: var(--ds-text-subtle);">
          {searchResults.length} results found
        </div>
      {/if}
    </div>

    <!-- Search Results -->
    {#if loading}
      <div class="rounded-xl border shadow-sm p-8 text-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <Spinner class="mx-auto mb-4" />
        <div style="color: var(--ds-text-subtle);">Searching...</div>
      </div>
    {:else if searchResults.length === 0 && hasFilters}
      <div class="rounded-xl border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <EmptyState
          icon={Search}
          title="No results found"
          description="Try adjusting your search terms or filters."
        />
      </div>
    {:else if searchResults.length === 0}
      <div class="rounded-xl border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <EmptyState
          icon={Search}
          title="Start your search"
          description="Enter keywords or use filters to find work items."
        />
      </div>
    {:else}
      <div class="rounded-xl border shadow-sm overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <!-- Table Header -->
        <div class="px-6 py-4 border-b" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
          <div class="grid grid-cols-12 gap-4 font-medium text-sm" style="color: var(--ds-text-subtle);">
            <div class="col-span-5">Work Item</div>
            <div class="col-span-2">Workspace</div>
            <div class="col-span-1">Status</div>
            <div class="col-span-1">Priority</div>
            <div class="col-span-2">Updated</div>
            <div class="col-span-1">Actions</div>
          </div>
        </div>

        <!-- Table Body -->
        <div class="divide-y" style="border-color: var(--ds-border);">
          {#each searchResults as item}
            <div class="px-6 py-4 transition-colors cursor-pointer table-row" role="button" tabindex="0" onclick={() => viewItem(item)} onkeydown={(e) => e.key === 'Enter' && viewItem(item)}>
              <div class="grid grid-cols-12 gap-4 items-center">
                <!-- Work Item -->
                <div class="col-span-5">
                  <div class="flex items-center gap-2 mb-1">
                    <span class="text-xs font-mono px-2 py-1 rounded" style="color: var(--ds-text-subtle); background-color: var(--ds-surface);">
                      {item.workspace_key || 'WORK'}-{item.id}
                    </span>
                    <h4 class="font-medium transition-colors item-title" style="color: var(--ds-text);">
                      {item.title}
                    </h4>
                  </div>
                  {#if item.description}
                    <p class="text-sm line-clamp-2" style="color: var(--ds-text-subtle);">{item.description}</p>
                  {/if}
                </div>

                <!-- Workspace -->
                <div class="col-span-2">
                  <div class="flex items-center gap-1 text-sm">
                    <Building class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                    <span style="color: var(--ds-text);">{item.workspace_name}</span>
                  </div>
                </div>

                <!-- Status -->
                <div class="col-span-1">
                  <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium" style={getStatusStyle(item.status)}>
                    {item.status.replace('_', ' ')}
                  </span>
                </div>

                <!-- Priority -->
                <div class="col-span-1">
                  <div class="flex items-center gap-1">
                    <AlertCircle class="w-4 h-4 {getPriorityColor(item.priority)}" />
                    <span class="text-sm font-medium capitalize {getPriorityColor(item.priority)}">
                      {item.priority || 'medium'}
                    </span>
                  </div>
                </div>

                <!-- Updated Date -->
                <div class="col-span-2">
                  <div class="flex items-center gap-1 text-sm" style="color: var(--ds-text-subtle);">
                    <Calendar class="w-4 h-4" />
                    {formatDate(item.updated_at) || '-'}
                  </div>
                </div>

                <!-- Actions -->
                <div class="col-span-1" role="button" tabindex="0" onclick={e => e.stopPropagation()} onkeydown={e => (e.key === 'Enter' || e.key === ' ') && e.stopPropagation()}>
                  <DropdownMenu
                    triggerText=""
                    triggerIcon={MoreHorizontal}
                    triggerClass="p-2 rounded transition-colors action-btn"
                    items={buildItemActions(item)}
                    align="right"
                  />
                </div>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>
</div>


<style>
  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .table-row:hover {
    background-color: var(--ds-surface);
  }

  .item-title:hover {
    color: var(--ds-interactive) !important;
  }

  .action-btn:hover {
    background-color: var(--ds-surface);
  }

  .divide-y > :not([hidden]) ~ :not([hidden]) {
    border-color: var(--ds-border);
  }
</style>