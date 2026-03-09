<script>
  import { onMount } from 'svelte';
  import { ChevronLeft, Filter, Search, Plus, X } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DynamicFieldFilter from '../items/DynamicFieldFilter.svelte';
  import Button from '../../components/Button.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import { api } from '../../api.js';
  import { createEventDispatcher } from 'svelte';

  const dispatch = createEventDispatcher();

  // Props
  export let collapsed = false;
  export let workspaces = [];
  export let selectedWorkspaces = [];
  export let selectedStatuses = [];
  export let selectedPriorities = [];
  export let searchQuery = '';
  export let dynamicFilters = [];

  // Internal state
  let allStatuses = [];
  let allPriorities = [];
  let showSearchModal = false;
  let tempSearchQuery = '';

  const SIDEBAR_STORAGE_KEY = 'collections-sidebar-collapsed';

  onMount(async () => {
    // Restore collapsed state from localStorage
    const savedState = localStorage.getItem(SIDEBAR_STORAGE_KEY);
    if (savedState !== null) {
      collapsed = savedState === 'true';
    }

    // Load statuses and priorities
    try {
      const statuses = await api.statuses.getAll();
      allStatuses = (statuses || []).map(status => ({
        id: status.id,
        name: status.name || status.key || ''
      }));

      const priorities = await api.priorities.getAll();
      allPriorities = (priorities || [])
        .sort((a, b) => a.sort_order - b.sort_order)
        .map(priority => ({
          id: priority.id,
          name: priority.name,
          color: priority.color || null
        }));
    } catch (err) {
      console.error('Failed to load statuses and priorities:', err);
    }
  });

  function toggleCollapse() {
    collapsed = !collapsed;
    localStorage.setItem(SIDEBAR_STORAGE_KEY, String(collapsed));
    dispatch('toggle-collapse', collapsed);
  }

  function handleWorkspacesChange(newValue) {
    dispatch('update-workspaces', newValue);
    dispatch('execute-search');
  }

  function handleStatusesChange(newValue) {
    dispatch('update-statuses', newValue);
    dispatch('execute-search');
  }

  function handlePrioritiesChange(newValue) {
    dispatch('update-priorities', newValue);
    dispatch('execute-search');
  }

  function openSearchModal() {
    tempSearchQuery = searchQuery;
    showSearchModal = true;
  }

  function closeSearchModal() {
    showSearchModal = false;
  }

  function applySearch() {
    dispatch('update-search', tempSearchQuery);
    dispatch('execute-search');
    showSearchModal = false;
  }

  function clearSearch() {
    tempSearchQuery = '';
    dispatch('update-search', '');
    dispatch('execute-search');
    showSearchModal = false;
  }

  function addDynamicFilter() {
    const newFilter = {
      field: null,
      operator: '=',
      value: '',
      values: []
    };
    dispatch('update-dynamic-filters', [...dynamicFilters, newFilter]);
  }

  function removeDynamicFilter(index) {
    dispatch('update-dynamic-filters', dynamicFilters.filter((_, i) => i !== index));
    dispatch('execute-search');
  }

  function handleDynamicFilterChange(index, event) {
    const updated = [...dynamicFilters];
    updated[index] = event.detail;
    dispatch('update-dynamic-filters', updated);
    dispatch('execute-search');
  }

  function handleDynamicFilterExecute() {
    dispatch('execute-search');
  }
</script>

<div
  class="border-r flex flex-col transition-all duration-200 flex-shrink-0"
  class:w-14={collapsed}
  class:w-64={!collapsed}
  style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
>
  <!-- Header with collapse toggle -->
  <div class="flex items-center p-4 border-b" class:justify-center={collapsed} class:justify-between={!collapsed} style="border-color: var(--ds-border);">
    {#if !collapsed}
      <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('collections.filters')}</h3>
    {/if}
    <button
      onclick={toggleCollapse}
      class="p-1.5 rounded transition-colors hover:bg-opacity-10"
      style="color: var(--ds-text-subtle);"
      title={collapsed ? t('collections.expandSidebar') : t('collections.collapseSidebar')}
    >
      <ChevronLeft class="w-4 h-4 transition-transform duration-200 {collapsed ? 'rotate-180' : ''}" />
    </button>
  </div>

  {#if !collapsed}
    <div class="flex-1 overflow-y-auto p-4">
      <!-- Search button -->
      <div class="mb-4">
        <button
          onclick={openSearchModal}
          class="w-full flex items-center gap-2 px-2.5 py-1.5 text-sm border rounded transition-colors"
          style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text-subtle);"
          onmouseenter={(e) => e.currentTarget.style.borderColor = 'var(--ds-border-bold)'}
          onmouseleave={(e) => e.currentTarget.style.borderColor = 'var(--ds-border)'}
        >
          <Search class="w-4 h-4 flex-shrink-0" style="color: var(--ds-icon-subtle);" />
          {#if searchQuery}
            <span class="truncate text-left flex-1" style="color: var(--ds-text);">{searchQuery}</span>
            <button
              onclick={(e) => { e.stopPropagation(); clearSearch(); }}
              class="p-0.5 rounded transition-colors flex-shrink-0"
              style="color: var(--ds-text-subtle);"
              title={t('collections.clearSearch')}
            >
              <X class="w-3 h-3" />
            </button>
          {:else}
            <span class="text-left flex-1">{t('collections.searchItems')}</span>
          {/if}
        </button>
      </div>

      <!-- Filter pickers with chips -->
      <div class="space-y-4">
        <!-- Workspace Picker -->
        <div>
          <span class="block text-xs font-medium mb-1.5" style="color: var(--ds-text-subtle);">
            {t('collections.workspaces')}
          </span>
          <BasePicker
            items={workspaces}
            value={selectedWorkspaces}
            multiple={true}
            placeholder={t('collections.selectWorkspaces')}
            getValue={(item) => item?.id}
            getLabel={(item) => item?.name ?? ''}
            onChange={handleWorkspacesChange}
          />
        </div>

        <!-- Status Picker -->
        <div>
          <span class="block text-xs font-medium mb-1.5" style="color: var(--ds-text-subtle);">
            {t('collections.status')}
          </span>
          <BasePicker
            items={allStatuses}
            value={selectedStatuses}
            multiple={true}
            placeholder={t('collections.selectStatuses')}
            getValue={(item) => item?.id}
            getLabel={(item) => item?.name ?? ''}
            onChange={handleStatusesChange}
          />
        </div>

        <!-- Priority Picker -->
        <div>
          <span class="block text-xs font-medium mb-1.5" style="color: var(--ds-text-subtle);">
            {t('collections.priority')}
          </span>
          <BasePicker
            items={allPriorities}
            value={selectedPriorities}
            multiple={true}
            placeholder={t('collections.selectPriorities')}
            getValue={(item) => item?.id}
            getLabel={(item) => item?.name ?? ''}
            onChange={handlePrioritiesChange}
          />
        </div>

        <!-- Dynamic filters -->
        {#each dynamicFilters as filter, index}
          <DynamicFieldFilter
            {filter}
            compact={true}
            on:change={(event) => handleDynamicFilterChange(index, event)}
            on:remove={() => removeDynamicFilter(index)}
            on:execute={handleDynamicFilterExecute}
          />
        {/each}

        <!-- Add Field Filter button -->
        <Button
          variant="ghost"
          size="sm"
          icon={Plus}
          onclick={addDynamicFilter}
          class="w-full justify-start"
        >
          {t('collections.addFieldFilter')}
        </Button>
      </div>
    </div>
  {:else}
    <!-- Collapsed state: show icons only -->
    <div class="flex flex-col items-center gap-4 p-4 pt-6">
      <button
        onclick={toggleCollapse}
        class="p-2 rounded transition-colors"
        style="color: var(--ds-icon-subtle);"
        title={t('collections.filters')}
      >
        <Filter class="w-5 h-5" />
      </button>
      <button
        onclick={toggleCollapse}
        class="p-2 rounded transition-colors"
        style="color: var(--ds-icon-subtle);"
        title={t('common.search')}
      >
        <Search class="w-5 h-5" />
      </button>
    </div>
  {/if}
</div>

<!-- Search Modal -->
<Modal bind:isOpen={showSearchModal} maxWidth="max-w-md" onclose={closeSearchModal} onSubmit={applySearch}>
  <div class="p-4">
    <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">{t('collections.searchItemsTitle')}</h3>
    <input
      type="text"
      bind:value={tempSearchQuery}
      placeholder={t('collections.enterSearchText')}
      class="w-full px-3 py-2 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
      style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
    />
    <div class="flex justify-end gap-2 mt-4">
      <Button variant="ghost" size="sm" onclick={clearSearch}>{t('collections.clear')}</Button>
      <Button variant="ghost" size="sm" onclick={closeSearchModal}>{t('common.cancel')}</Button>
      <Button variant="primary" size="sm" onclick={applySearch}>{t('collections.apply')}</Button>
    </div>
  </div>
</Modal>
