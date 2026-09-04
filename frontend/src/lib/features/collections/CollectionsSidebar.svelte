<script>
  import { onMount } from 'svelte';
  import { ChevronLeft, Filter, Search } from '@lucide/svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import WorkItemFilterPanel from '../items/WorkItemFilterPanel.svelte';
  import SidebarResizeHandle from '../../layout/SidebarResizeHandle.svelte';

  let {
    collapsed = $bindable(false),
    workspaces = [],
    allStatuses = [],
    allPriorities = [],
    selectedWorkspaces = [],
    selectedStatuses = [],
    selectedPriorities = [],
    searchQuery = '',
    dynamicFilters = [],
    loading = false,
    disabled = false,
    ontogglecollapse = null,
    onupdateworkspaces = null,
    onupdatestatuses = null,
    onupdatepriorities = null,
    onupdatesearch = null,
    onupdatedynamicfilters = null,
    onexecutesearch = null,
  } = $props();

  const SIDEBAR_STORAGE_KEY = 'collections-sidebar-collapsed';
  const SIDEBAR_WIDTH_KEY = 'collections-sidebar-width';

  let sidebarWidth = $state(256);
  function persistSidebarWidth(width) {
    sidebarWidth = width;
    localStorage.setItem(SIDEBAR_WIDTH_KEY, String(sidebarWidth));
  }

  onMount(() => {
    const savedState = localStorage.getItem(SIDEBAR_STORAGE_KEY);
    if (savedState !== null) collapsed = savedState === 'true';

    const savedWidth = localStorage.getItem(SIDEBAR_WIDTH_KEY);
    if (savedWidth) sidebarWidth = parseInt(savedWidth, 10) || 256;
  });

  function toggleCollapse() {
    collapsed = !collapsed;
    localStorage.setItem(SIDEBAR_STORAGE_KEY, String(collapsed));
    ontogglecollapse?.(collapsed);
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
    <SidebarResizeHandle
      width={sidebarWidth}
      minWidth={200}
      maxWidth={480}
      defaultWidth={256}
      label="Resize collection filters"
      title="Drag to resize, double-click to reset"
      onresize={(width) => sidebarWidth = width}
      onresizeend={persistSidebarWidth}
    />
  {/if}

  <div
    class="flex items-center p-4 border-b"
    class:justify-center={collapsed}
    class:justify-between={!collapsed}
    style="border-color: var(--ds-border);"
  >
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
      <WorkItemFilterPanel
        testIdPrefix="collection"
        {workspaces}
        {allStatuses}
        {allPriorities}
        {selectedWorkspaces}
        {selectedStatuses}
        {selectedPriorities}
        {searchQuery}
        {dynamicFilters}
        interactionDisabled={disabled || loading}
        {disabled}
        {onupdateworkspaces}
        {onupdatestatuses}
        {onupdatepriorities}
        {onupdatesearch}
        {onupdatedynamicfilters}
        {onexecutesearch}
      />
    </div>
  {:else}
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
