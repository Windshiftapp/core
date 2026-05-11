<script>
  import { IconLayoutKanban as SquareKanban, IconList as List, IconMapPin as MapPin, IconPencil as Pencil, IconLayoutRows as Rows_3, IconListTree as ListTree, IconFolderOpen as FolderOpen, IconChevronRight } from '@tabler/icons-svelte-runes';
  import { GanttChart } from '@lucide/svelte';
  import { navigate, currentRoute } from '../../router.js';
  import Tooltip from '../../components/Tooltip.svelte';
  import Button from '../../components/Button.svelte';
  import { useEventListener } from 'runed';
  import { uiStore } from '../../stores/ui.svelte.js';
  import { collectionStore } from '../../stores/collectionContext.js';


  const MIN_WIDTH = 180;
  const MAX_WIDTH = 320;
  const COLLAPSE_THRESHOLD = 100;

  let sidebarWidth = $derived($uiStore.wsSidebarWidth);
  let isCollapsed = $derived($uiStore.wsSidebarCollapsed);
  let isResizing = $state(false);
  let resizeStartX = $state(0);
  let resizeStartWidth = $state(0);

  function onResizeStart(e) {
    e.preventDefault();
    resizeStartX = e.clientX;
    resizeStartWidth = isCollapsed ? 48 : sidebarWidth;
    isResizing = true;
  }

  function handleResizeMove(e) {
    const rawWidth = resizeStartWidth + (e.clientX - resizeStartX);
    if (rawWidth < COLLAPSE_THRESHOLD) {
      if (!isCollapsed) {
        uiStore.wsSidebarCollapsed = true;
      }
    } else {
      if (isCollapsed) {
        uiStore.wsSidebarCollapsed = false;
      }
      const newWidth = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, rawWidth));
      uiStore.wsSidebarWidth = newWidth;
    }
  }

  function handleResizeUp() {
    isResizing = false;
  }

  useEventListener(() => isResizing ? window : undefined, 'mousemove', handleResizeMove);
  useEventListener(() => isResizing ? window : undefined, 'mouseup', handleResizeUp);

  function onResizeHandleDblClick() {
    uiStore.wsSidebarCollapsed = false;
    uiStore.resetWsSidebarWidth();
  }

  let { collectionId = null } = $props();

  const collectionViewItems = [
    { id: 'backlog', label: 'Backlog', icon: Rows_3, routeView: 'collection-backlog', tooltip: 'Backlog view for unfinished items' },
    { id: 'board', label: 'Board', icon: SquareKanban, routeView: 'collection-board', tooltip: 'Kanban board view with columns' },
    { id: 'list', label: 'List', icon: List, routeView: 'collection-list', tooltip: 'Detailed list view with all fields' },
    { id: 'tree', label: 'Tree', icon: ListTree, routeView: 'collection-tree', tooltip: 'Hierarchical tree view for nested items' },
    { id: 'map', label: 'Map', icon: MapPin, routeView: 'collection-map', tooltip: 'Visual map view for spatial organization' },
    { id: 'roadmap', label: 'Roadmap', icon: GanttChart, routeView: 'collection-roadmap', tooltip: 'Timeline view with date ranges and dependencies' },
  ];

  function getNavUrl(viewId) {
    return `/collections/${collectionId}/${viewId}`;
  }

  let collectionName = $derived(collectionStore.collectionName);
  let itemCount = $derived(collectionStore.itemsPagination?.total ?? 0);

  const sidebarBgStyle = 'background-color: var(--ds-surface); border-color: var(--ds-border);';
</script>

{#if isCollapsed}
  <!-- Collapsed icon-only sidebar -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="relative h-full flex-shrink-0 border-r flex flex-col items-center py-4"
       style="width: 48px; {sidebarBgStyle}">
    <div class="flex flex-col items-center space-y-1 mt-2">
      {#each collectionViewItems as view}
        {@const isActive = $currentRoute.view === view.routeView}
        {@const ViewIcon = view.icon}
        <Tooltip content={view.label} placement="right">
          <a
            href={getNavUrl(view.id)}
            class="w-10 h-10 rounded flex items-center justify-center transition-colors no-underline"
            style={isActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
            onmouseenter={(e) => { if (!isActive) e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
            onmouseleave={(e) => { e.currentTarget.style.cssText = isActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'; }}
          >
            <ViewIcon size={20} />
          </a>
        </Tooltip>
      {/each}

      <!-- Divider -->
      <div class="w-8 border-t my-1" style="border-color: var(--ds-border);"></div>

      <!-- Edit Collection -->
      <Tooltip content="Edit Collection" placement="right">
        <a
          href={`/collections/${collectionId}`}
          class="w-10 h-10 rounded flex items-center justify-center transition-colors no-underline"
          style={$currentRoute.view === 'collections-edit' ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
          onmouseenter={(e) => { if ($currentRoute.view !== 'collections-edit') e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
          onmouseleave={(e) => { e.currentTarget.style.cssText = $currentRoute.view === 'collections-edit' ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'; }}
        >
          <Pencil size={20} />
        </a>
      </Tooltip>
    </div>

    <!-- Spacer -->
    <div class="flex-1"></div>

    <!-- Back to Collections -->
    <div class="w-8 border-t mb-2" style="border-color: var(--ds-border);"></div>
    <Tooltip content="Back to Collections" placement="right">
      <a
        href="/collections"
        class="w-10 h-10 rounded flex items-center justify-center cursor-pointer transition-colors mb-1 no-underline"
        style="color: var(--ds-text-subtle);"
        onmouseenter={(e) => e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'}
        onmouseleave={(e) => e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'}
      >
        <FolderOpen size={20} />
      </a>
    </Tooltip>

    <!-- Expand button -->
    <Tooltip content="Expand sidebar" placement="right">
      <button onclick={() => uiStore.wsSidebarCollapsed = false}
              class="w-10 h-10 rounded flex items-center justify-center transition-colors"
              style="color: var(--ds-text-subtle);"
              onmouseenter={(e) => e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'}
              onmouseleave={(e) => e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'}>
        <IconChevronRight size={20} />
      </button>
    </Tooltip>

    <!-- Resize handle -->
    <div class="ws-resize-handle" onmousedown={onResizeStart} ondblclick={onResizeHandleDblClick}></div>
  </div>
{:else}
  <!-- Expanded Collection Navigation Sidebar -->
  <div class="relative h-full flex-shrink-0 border-r flex flex-col py-4" style="width: {sidebarWidth}px; min-width: {MIN_WIDTH}px; max-width: {MAX_WIDTH}px; {sidebarBgStyle}">

    <!-- Collection Header -->
    <div class="px-4 mb-4 pb-4 border-b" style="border-color: var(--ds-border);">
      <!-- Collection info (static) -->
      <div class="flex items-center gap-3 w-full p-1">
        <div class="flex items-center justify-center w-10 h-10 flex-shrink-0">
          <div class="w-8 h-8 rounded-md flex items-center justify-center" style="background-color: var(--ds-accent-blue-subtle);">
            <FolderOpen size={18} style="color: var(--ds-text-info);" />
          </div>
        </div>
        <div class="flex-1 min-w-0">
          <Tooltip content={collectionName}>
            <div class="font-medium text-sm truncate" style="color: var(--ds-text);">{collectionName}</div>
          </Tooltip>
          <div class="text-xs" style="color: var(--ds-text-subtle);">Collection{#if itemCount > 0} · {itemCount} items{/if}</div>
        </div>
      </div>
    </div>

    <nav class="flex-1 px-4 space-y-2">
      <!-- View Items -->
      {#each collectionViewItems as view}
        {@const isActive = $currentRoute.view === view.routeView}
        {@const ViewIcon = view.icon}
        <Tooltip content={view.tooltip} placement="right">
          <a
            href={getNavUrl(view.id)}
            class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item no-underline"
            style={isActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
            onmouseenter={(e) => { if (!isActive) e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
            onmouseleave={(e) => { if (!isActive) e.currentTarget.style.cssText = isActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'; }}
          >
            <ViewIcon class="w-4 h-4" />
            {view.label}
          </a>
        </Tooltip>
      {/each}

      <!-- Divider + Collection section -->
      <div class="mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
        <div class="text-xs font-semibold uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">
          Collection
        </div>

        <!-- Edit Collection -->
        <Tooltip content="Edit collection query and settings" placement="right">
          <a
            href={`/collections/${collectionId}`}
            class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item mt-2 no-underline"
            style="color: var(--ds-text-subtle);"
            onmouseenter={(e) => e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'}
            onmouseleave={(e) => e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'}
          >
            <Pencil class="w-4 h-4" />
            Edit Collection
          </a>
        </Tooltip>
      </div>
    </nav>

    <!-- Footer - Back to Collections -->
    <div class="px-4 pt-4 border-t" style="border-color: var(--ds-border);">
      <Button
        variant="default"
        icon={FolderOpen}
        href="/collections"
        class="w-full justify-center"
      >
        Back to Collections
      </Button>
    </div>

    <!-- Resize handle -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="ws-resize-handle"
      onmousedown={onResizeStart}
      ondblclick={onResizeHandleDblClick}
    ></div>
  </div>
{/if}

<style>
  .ws-resize-handle {
    position: absolute;
    top: 0;
    right: 0;
    width: 4px;
    height: 100%;
    cursor: col-resize;
    z-index: 10;
    transition: background-color 150ms ease;
  }

  .ws-resize-handle:hover,
  .ws-resize-handle:active {
    background-color: var(--ds-border-focused, #3b82f6);
  }

  @media (prefers-reduced-motion: reduce) {
    nav,
    nav .border-t {
      animation: none;
    }
  }
</style>
