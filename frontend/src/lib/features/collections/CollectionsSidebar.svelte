<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import { ChevronLeft, Filter, Search, Plus, X } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DynamicFieldFilter from '../items/DynamicFieldFilter.svelte';
  import Button from '../../components/Button.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import { api } from '../../api.js';

  let {
    collapsed = $bindable(false),
    workspaces = [],
    selectedWorkspaces = [],
    selectedStatuses = [],
    selectedPriorities = [],
    searchQuery = '',
    dynamicFilters = [],
    ontogglecollapse = null,
    onupdateworkspaces = null,
    onupdatestatuses = null,
    onupdatepriorities = null,
    onupdatesearch = null,
    onupdatedynamicfilters = null,
    onexecutesearch = null,
  } = $props();

  // Internal state
  let allStatuses = $state([]);
  let allPriorities = $state([]);
  let showSearchModal = $state(false);
  let tempSearchQuery = $state('');

  const SIDEBAR_STORAGE_KEY = 'collections-sidebar-collapsed';
  const SIDEBAR_WIDTH_KEY = 'collections-sidebar-width';

  let sidebarWidth = $state(256);
  let isResizing = $state(false);
  let resizeStartX = $state(0);
  let resizeStartWidth = $state(0);

  function startResize(event) {
    isResizing = true;
    resizeStartX = event.clientX;
    resizeStartWidth = sidebarWidth;
    event.preventDefault();
  }

  function handleResizeMove(event) {
    const deltaX = event.clientX - resizeStartX;
    const newWidth = Math.max(200, Math.min(480, resizeStartWidth + deltaX));
    sidebarWidth = newWidth;
  }

  function handleResizeUp() {
    isResizing = false;
    localStorage.setItem(SIDEBAR_WIDTH_KEY, String(sidebarWidth));
  }

  useEventListener(() => isResizing ? document : undefined, 'mousemove', handleResizeMove);
  useEventListener(() => isResizing ? document : undefined, 'mouseup', handleResizeUp);

  onMount(async () => {
    // Restore collapsed state from localStorage
    const savedState = localStorage.getItem(SIDEBAR_STORAGE_KEY);
    if (savedState !== null) {
      collapsed = savedState === 'true';
    }

    // Restore sidebar width from localStorage
    const savedWidth = localStorage.getItem(SIDEBAR_WIDTH_KEY);
    if (savedWidth) sidebarWidth = parseInt(savedWidth, 10) || 256;

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
    ontogglecollapse?.(collapsed);
  }

  function handleWorkspacesChange(newValue) {
    onupdateworkspaces?.(newValue);
    onexecutesearch?.();
  }

  function handleStatusesChange(newValue) {
    onupdatestatuses?.(newValue);
    onexecutesearch?.();
  }

  function handlePrioritiesChange(newValue) {
    onupdatepriorities?.(newValue);
    onexecutesearch?.();
  }

  function openSearchModal() {
    tempSearchQuery = searchQuery;
    showSearchModal = true;
  }

  function closeSearchModal() {
    showSearchModal = false;
  }

  function applySearch() {
    onupdatesearch?.(tempSearchQuery);
    onexecutesearch?.();
    showSearchModal = false;
  }

  function clearSearch() {
    tempSearchQuery = '';
    onupdatesearch?.('');
    onexecutesearch?.();
    showSearchModal = false;
  }

  function addDynamicFilter() {
    const newFilter = {
      field: null,
      operator: '=',
      value: '',
      values: []
    };
    onupdatedynamicfilters?.([...dynamicFilters, newFilter]);
  }

  function removeDynamicFilter(index) {
    onupdatedynamicfilters?.(dynamicFilters.filter((_, i) => i !== index));
    onexecutesearch?.();
  }

  function handleDynamicFilterChange(index, data) {
    const updated = [...dynamicFilters];
    updated[index] = data;
    onupdatedynamicfilters?.(updated);
    onexecutesearch?.();
  }

  function handleDynamicFilterExecute() {
    onexecutesearch?.();
  }
</script>

<div
  class="border-r flex flex-col transition-all duration-200 flex-shrink-0 relative"
  class:w-14={collapsed}
  style="
    {collapsed ? '' : `width: ${sidebarWidth}px; min-width: 200px; max-width: 480px;`}
    border-color: var(--ds-border);
    background-color: var(--ds-surface-raised);
  "
>
  {#if !collapsed}
    <div
      class="absolute right-0 top-0 bottom-0 w-1 cursor-ew-resize transition-colors opacity-0 hover:opacity-100 z-10"
      style="background-color: var(--ds-border);"
      onmouseenter={(e) => e.currentTarget.style.backgroundColor = '#3b82f6'}
      onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-border)'}
      onmousedown={startResize}
    ></div>
  {/if}

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
            onchange={(data) => handleDynamicFilterChange(index, data)}
            onremove={() => removeDynamicFilter(index)}
            onexecute={handleDynamicFilterExecute}
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
