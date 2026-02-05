<script>
  import { onMount } from 'svelte';
  import { Plus, CheckSquare, Calendar, Home, Inbox, SquareKanban, List, GitBranch, MapPin, Settings, BookOpen, Package, ChevronDown, FileCheck, FileStack, Play, BarChart3, ListTree, Milestone, Grip, Zap, Palette, Sparkles } from 'lucide-svelte';
  import { workspaceIconMap } from '../utils/icons.js';
  import { navigate, currentRoute } from '../router.js';
  import { currentWorkspace, workspacePermissions } from '../stores';
  import { api } from '../api.js';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import { workspaceGradientIndex, applyToAllViews, loadWorkspaceGradient, getGradientStyle } from '../stores/workspaceGradient.svelte.js';
    import Rows_3 from 'lucide-svelte/icons/rows-3';

  let { workspaceId = null } = $props();
  
  let collections = $state([]);
  let allCollections = $state([]);
  let currentCollectionId = $state(null); // Track by ID instead of name
  let currentCollectionName = $state('Default'); // For display
  let collectionDropdownItems = $state([]);
  let workspaceToolsExpanded = $state(true);
  let lastCollectionId = undefined; // Plain variable to prevent infinite loop in $effect

  const workspaceViewItems = [
    { id: 'backlog', label: 'Backlog', icon: Rows_3, tooltip: 'Backlog view for unfinished items' },
    { id: 'board', label: 'Board', icon: SquareKanban, tooltip: 'Kanban board view with columns' },
    { id: 'list', label: 'List', icon: List, tooltip: 'Detailed list view with all fields' },
    { id: 'tree', label: 'Tree', icon: ListTree, tooltip: 'Hierarchical tree view for nested items' },
    { id: 'map', label: 'Map', icon: MapPin, tooltip: 'Visual map view for spatial organization' }
  ];

  const workspaceOnlyViews = [
    { id: 'iterations', label: 'Iterations', icon: Calendar, tooltip: 'Manage sprints, PIs, and other iteration cycles' },
    { id: 'milestones', label: 'Milestones', icon: Milestone, tooltip: 'Manage workspace milestones and releases' },
    { id: 'actions', label: 'Actions', icon: Zap, tooltip: 'Automate workflows and triggers' }
  ];
  const workspaceOnlyViewIds = new Set(workspaceOnlyViews.map(view => view.id));
  const workspaceTestViewIds = new Set([
    'test-cases',
    'test-case-detail',
    'test-steps',
    'test-sets',
    'test-set-detail',
    'test-templates',
    'test-template-detail',
    'test-runs',
    'test-run-detail',
    'test-execution',
    'test-reports'
  ]);
  const testNavigationItems = [
    { id: 'test-cases', label: 'Test Cases', icon: FileCheck, tooltip: 'Manage test cases and steps', activeViews: ['test-cases', 'test-case-detail', 'test-steps'] },
    { id: 'test-sets', label: 'Test Plans', icon: Package, tooltip: 'Organize plans and suites', activeViews: ['test-sets', 'test-set-detail'] },
    { id: 'test-templates', label: 'Templates', icon: FileStack, tooltip: 'Template runs and shared steps', activeViews: ['test-templates', 'test-template-detail'] },
    { id: 'test-runs', label: 'Test Runs', icon: Play, tooltip: 'Schedule and execute runs', activeViews: ['test-runs', 'test-run-detail', 'test-execution'] },
    { id: 'test-reports', label: 'Reports', icon: BarChart3, tooltip: 'Review execution results', activeViews: ['test-reports'] }
  ];
  const activeTestNavId = $derived.by(() => getActiveTestNavId($currentRoute));
  const defaultCollectionView = workspaceViewItems[0]?.id || 'backlog';

  // Permission-based visibility
  const canViewTests = $derived.by(() => workspacePermissions.canViewTests(workspaceId));
  const canManageActions = $derived.by(() => workspacePermissions.canManageActions(workspaceId));
  const canAdmin = $derived.by(() => workspacePermissions.canAdminWorkspace(workspaceId));

  // Filter workspace-only views based on permissions
  const filteredWorkspaceOnlyViews = $derived.by(() => {
    return workspaceOnlyViews.filter(view => {
      if (view.id === 'actions') return canManageActions;
      return true;
    });
  });

  // Gradient detection
  const gradientStyle = $derived.by(() => ($applyToAllViews && $workspaceGradientIndex > 0) ? getGradientStyle($workspaceGradientIndex) : null);
  const hasGradient = $derived.by(() => gradientStyle !== null);
  const sidebarBgClass = $derived.by(() => hasGradient ? 'backdrop-blur-sm' : '');
  const sidebarBgStyle = $derived.by(() => hasGradient
    ? 'background-color: color-mix(in srgb, var(--ds-surface) 95%, transparent); border-color: color-mix(in srgb, var(--ds-border) 20%, transparent);'
    : 'background-color: var(--ds-surface); border-color: var(--ds-border);');
  const sidebarTextStyle = $derived.by(() => 'color: var(--ds-text);');
  const sidebarTextSubtleStyle = $derived.by(() => 'color: var(--ds-text-subtle);');

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
      await loadCollections();
    }
  });

  // Reactive statement to reload collections when workspaceId changes
  $effect(() => {
    if (workspaceId) {
      loadCollections();
    }
  });
  
  // Reactive statement to sync collection with route changes
  $effect(() => {
    syncCollectionWithRoute($currentRoute.params.collectionId);
  });
  $effect(() => {
    if (currentCollectionId !== lastCollectionId) {
      lastCollectionId = currentCollectionId;
      workspaceToolsExpanded = currentCollectionId === null;
    }
  });

  function syncCollectionWithRoute(routeCollectionId) {
    if (routeCollectionId) {
      currentCollectionId = routeCollectionId;
      // Update the display name based on ID
      const collection = collections.find(c => c.id == routeCollectionId);
      currentCollectionName = collection ? collection.name : 'Default';
    } else {
      currentCollectionId = null;
      currentCollectionName = 'Default';
    }
    buildCollectionDropdownItems();
  }

  async function loadCollections() {
    try {
      const result = await api.collections.getAll();
      allCollections = result || [];

      // Filter collections for this workspace
      collections = filterCollectionsForWorkspace(allCollections, workspaceId);

      // Sync with current route (reactive statement will handle this, but we need to rebuild dropdown)
      syncCollectionWithRoute($currentRoute.params.collectionId);
    } catch (error) {
      console.error('Failed to load collections:', error);
      collections = [];
      currentCollectionId = null;
      currentCollectionName = 'Default';
      buildCollectionDropdownItems();
    }
  }

  // Helper function to determine if a collection is associated with a workspace
  // Only checks direct workspace_id association - QL query content does not affect where a collection appears
  function isCollectionAssociatedWithWorkspace(collection, targetWorkspaceId) {
    const collectionWorkspaceId = collection.workspace_id ?? collection.workspaceId;
    return collectionWorkspaceId && Number(collectionWorkspaceId) === Number(targetWorkspaceId);
  }

  // Filter collections to show only those relevant to the current workspace
  function filterCollectionsForWorkspace(allCollections, targetWorkspaceId) {
    return allCollections.filter(collection => 
      isCollectionAssociatedWithWorkspace(collection, targetWorkspaceId)
    );
  }

  // Helper function to truncate long workspace names
  function truncateWorkspaceName(name) {
    if (!name) return 'Workspace';
    // Truncate to ~20 characters to fit in 2 lines max
    if (name.length <= 20) return name;
    return name.substring(0, 17) + '...';
  }

  function buildCollectionDropdownItems() {
    // Build the items array first, then assign once to avoid multiple state updates
    const items = [];

    // Always add Default collection first (shows all items)
    const workspaceName = truncateWorkspaceName($currentWorkspace?.name);
    items.push({
      id: 'default',
      type: 'regular',
      title: `${workspaceName} - Default`,
      subtitle: 'Show all items in workspace',
      badge: currentCollectionId === null ? '✓' : null,
      badgeClass: currentCollectionId === null ? '' : '',
      badgeStyle: currentCollectionId === null ? 'color: var(--ds-text-link);' : 'color: var(--ds-text-subtlest);',
      style: currentCollectionId === null ? 'background-color: var(--ds-background-selected); color: var(--ds-text); font-weight: 600;' : '',
      onClick: () => selectCollection(null)
    });

    // Add workspace-specific collections if any exist
    if (collections.length > 0) {
      const collectionItems = collections.map(collection => ({
        id: `collection-${collection.id}`,
        type: 'regular',
        title: collection.name,
        subtitle: collection.description || undefined,
        badge: currentCollectionId == collection.id ? '✓' : null,
        badgeClass: currentCollectionId == collection.id ? '' : '',
        badgeStyle: currentCollectionId == collection.id ? 'color: var(--ds-text-link);' : 'color: var(--ds-text-subtlest);',
        style: currentCollectionId == collection.id ? 'background-color: var(--ds-background-selected); color: var(--ds-text); font-weight: 600;' : '',
        onClick: () => selectCollection(collection)
      }));

      items.push(...collectionItems);
    }

    // Add divider and collections management link
    items.push(
      { id: 'divider-1', type: 'divider' },
      {
        id: 'add-collection',
        type: 'regular',
        icon: Plus,
        title: 'Add Collection',
        color: 'var(--ds-text-link)',
        onClick: () => {
          window.dispatchEvent(new CustomEvent('show-create-modal', {
            detail: {
              type: 'collection',
              workspaceId: workspaceId
            }
          }));
        }
      }
    );

    // Single assignment to state
    collectionDropdownItems = items;
  }

  function toggleWorkspaceToolsSection() {
    workspaceToolsExpanded = !workspaceToolsExpanded;
  }

  function selectCollection(collection) {
    if (collection === null) {
      currentCollectionId = null;
      currentCollectionName = 'Default';
    } else {
      currentCollectionId = collection.id;
      currentCollectionName = collection.name;
    }
    buildCollectionDropdownItems();
    
    // Navigate to the new URL with the selected collection
    // Determine current view from the route
    let currentView = $currentRoute.view;
    if (currentView === 'workspace-detail') {
      // If on overview, navigate to overview with/without collection
      const url = currentCollectionId 
        ? `/workspaces/${workspaceId}/collections/${currentCollectionId}`
        : `/workspaces/${workspaceId}`;
      navigate(url);
    } else if (currentView && currentView.startsWith('workspace-')) {
      // For other workspace views (board, list, etc.), extract the view name
      let viewName = currentView.replace('workspace-', '');

      // Workspace-only views cannot be scoped by collection, fallback to default collection view
      if (currentCollectionId !== null && workspaceOnlyViewIds.has(viewName)) {
        viewName = defaultCollectionView;
      }

      const url = getNavigationUrl(viewName);
      navigate(url);
    } else if (currentView && workspaceTestViewIds.has(currentView)) {
      const url = getTestNavigationUrl(getTestNavIdFromView(currentView));
      navigate(url);
    }
  }

  function getNavigationUrl(view) {
    if (workspaceOnlyViewIds.has(view) || !currentCollectionId) {
      return `/workspaces/${workspaceId}/${view}`;
    }
    return `/workspaces/${workspaceId}/collections/${currentCollectionId}/${view}`;
  }

  function getTestNavigationUrl(viewId) {
    switch (viewId) {
      case 'test-cases':
        return `/workspaces/${workspaceId}/tests`;
      case 'test-sets':
        return `/workspaces/${workspaceId}/tests/sets`;
      case 'test-templates':
        return `/workspaces/${workspaceId}/tests/templates`;
      case 'test-runs':
        return `/workspaces/${workspaceId}/tests/runs`;
      case 'test-reports':
        return `/workspaces/${workspaceId}/tests/reports`;
      default:
        return `/workspaces/${workspaceId}/tests`;
    }
  }

  function getTestNavIdFromView(view) {
    if (view === 'test-case-detail' || view === 'test-steps') return 'test-cases';
    if (view === 'test-set-detail') return 'test-sets';
    if (view === 'test-template-detail') return 'test-templates';
    if (view === 'test-run-detail' || view === 'test-execution') return 'test-runs';
    if (view === 'test-reports') return 'test-reports';
    return 'test-cases';
  }

  function getActiveTestNavId(route) {
    const view = route?.view;
    const path = route?.path || '';

    for (const item of testNavigationItems) {
      if (item.activeViews.includes(view)) return item.id;
    }

    if (path.includes('/tests/sets')) return 'test-sets';
    if (path.includes('/tests/templates')) return 'test-templates';
    if (path.includes('/tests/runs')) return 'test-runs';
    if (path.includes('/tests/reports')) return 'test-reports';
    if (path.includes('/tests')) return 'test-cases';
    return null;
  }
</script>

{#if $currentWorkspace?.is_personal}
  <!-- Simplified Personal Workspace Sidebar -->
  <div class="w-48 min-w-48 h-full flex-shrink-0 {sidebarBgClass} border-r flex flex-col py-4" style={sidebarBgStyle}>
    <!-- Workspace Header -->
    <div class="px-4 mb-4 pb-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center gap-3">
        {#if $currentWorkspace.avatar_url}
          <div class="flex items-center justify-center w-10 h-10 flex-shrink-0">
            <img src={$currentWorkspace.avatar_url} alt="{$currentWorkspace.name} avatar" class="w-8 h-8 rounded-md object-cover" />
          </div>
        {:else}
          <div class="flex items-center justify-center w-10 h-10 flex-shrink-0">
            <div class="w-8 h-8 rounded-md flex items-center justify-center" style="background-color: {$currentWorkspace.color || '#f97316'};">
              <svelte:component this={workspaceIconMap[$currentWorkspace.icon] || Grip} size={18} color="white" />
            </div>
          </div>
        {/if}
        <div class="flex-1 min-w-0">
          <div class="font-medium text-sm truncate" style="color: var(--ds-text);">{$currentWorkspace.name}</div>
          <div class="text-xs text-orange-600">Personal</div>
        </div>
      </div>
    </div>
    
    <nav class="flex-1 px-4 space-y-2">
      <button
        onclick={() => navigate('/personal')}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
        style={$currentRoute.view === 'personal-workspace' ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if ($currentRoute.view !== 'personal-workspace') e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if ($currentRoute.view !== 'personal-workspace') e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
      >
        <CheckSquare class="w-4 h-4" />
        My Tasks
      </button>
      <button
        onclick={() => navigate('/personal/reviews')}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
        style={$currentRoute.view === 'workspace-reviews' ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if ($currentRoute.view !== 'workspace-reviews') e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if ($currentRoute.view !== 'workspace-reviews') e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
      >
        <BookOpen class="w-4 h-4" />
        Reviews
      </button>
      <button
        onclick={() => navigate('/personal/calendar')}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
        style={$currentRoute.view === 'workspace-calendar' ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if ($currentRoute.view !== 'workspace-calendar') e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if ($currentRoute.view !== 'workspace-calendar') e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
      >
        <Calendar class="w-4 h-4" />
        Weekly Calendar
      </button>
      <button
        onclick={() => navigate('/personal/plan')}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
        style={$currentRoute.view === 'personal-plan' ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if ($currentRoute.view !== 'personal-plan') e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if ($currentRoute.view !== 'personal-plan') e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
      >
        <Sparkles class="w-4 h-4" />
        Plan My Day
      </button>
    </nav>
  </div>
{:else}
  <!-- Regular Workspace Navigation Sidebar -->
  <div class="w-48 min-w-48 h-full flex-shrink-0 {sidebarBgClass} border-r flex flex-col py-4" style={sidebarBgStyle}>

  <!-- Workspace Header -->
  <div class="px-4 mb-4 pb-4 border-b" style="border-color: var(--ds-border);">
    <div class="flex items-center gap-3">
      {#if $currentWorkspace?.avatar_url}
        <div class="flex items-center justify-center w-10 h-10 flex-shrink-0">
          <img src={$currentWorkspace.avatar_url} alt="{$currentWorkspace.name} avatar" class="w-8 h-8 rounded-md object-cover" />
        </div>
      {:else}
        <div class="flex items-center justify-center w-10 h-10 flex-shrink-0">
          <div class="w-8 h-8 rounded-md flex items-center justify-center" style="background-color: {$currentWorkspace?.color || '#3b82f6'};">
            <svelte:component this={workspaceIconMap[$currentWorkspace?.icon] || Grip} size={18} color="white" />
          </div>
        </div>
      {/if}
      <div class="flex-1 min-w-0">
        <div class="font-medium text-sm truncate" style="color: var(--ds-text);">{$currentWorkspace?.name || 'Workspace'}</div>
        {#if $currentWorkspace?.description}
          <div class="text-xs truncate" style="color: var(--ds-text-subtle);">{$currentWorkspace.description}</div>
        {/if}
      </div>
    </div>
  </div>
  
  <!-- Collection Selector -->
  <div class="px-4 mb-6">
    <Tooltip content="Collection" placement="right">
      <DropdownMenu
        triggerText={currentCollectionName}
        items={collectionDropdownItems}
        maxWidth="max-w-full"
        showChevron={true}
        placement="bottom-start"
        triggerClass="w-full text-left font-medium rounded !px-3 !py-2.5 !text-sm transition-colors"
        triggerStyle="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text);"
        triggerAlignment="between"
      />
    </Tooltip>
  </div>
  
  <nav class="flex-1 px-4 space-y-2">
    
    <!-- Overview Link -->
    <Tooltip content="Workspace overview and dashboard" placement="right">
      <button
        onclick={() => navigate(`/workspaces/${workspaceId}/overview`)}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
        style={$currentRoute.view === 'workspace-overview' ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if ($currentRoute.view !== 'workspace-overview') e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if ($currentRoute.view !== 'workspace-overview') e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
      >
        <Home class="w-4 h-4" />
        Overview
      </button>
    </Tooltip>
    
    <!-- Workspace Views -->
    {#each workspaceViewItems as view}
      {@const isViewActive = $currentRoute.view === `workspace-${view.id}`}
      <Tooltip content={view.tooltip} placement="right">
        <button
          onclick={() => navigate(getNavigationUrl(view.id))}
          class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
          style={isViewActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
          onmouseenter={(e) => { if (!isViewActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
          onmouseleave={(e) => { if (!isViewActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
        >
          <svelte:component this={view.icon} class="w-4 h-4" />
          {view.label}
        </button>
      </Tooltip>
    {/each}

    {#if currentCollectionId}
      <div class="mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
        <div class="text-xs font-semibold uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">
          Collection
        </div>
        <button
          onclick={() => navigate(`/collections/${currentCollectionId}`)}
          class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
          style="color: var(--ds-text-subtle);"
          onmouseenter={(e) => e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'}
          onmouseleave={(e) => e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'}
        >
          <Pencil class="w-4 h-4" />
          Edit Collection
        </button>
      </div>
    {/if}

    {#if canViewTests}
    <div class="mt-4 pt-4 border-t space-y-2" style="border-color: var(--ds-border);">
      <div class="text-xs font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtle);">
        Tests
      </div>
      {#each testNavigationItems as view}
        {@const isTestActive = activeTestNavId === view.id}
        <Tooltip content={view.tooltip} placement="right">
          <button
            onclick={() => navigate(getTestNavigationUrl(view.id))}
            class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
            style={isTestActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
            onmouseenter={(e) => { if (!isTestActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
            onmouseleave={(e) => { if (!isTestActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
          >
            <svelte:component this={view.icon} class="w-4 h-4" />
            {view.label}
          </button>
        </Tooltip>
      {/each}
    </div>
    {/if}

    <div class="mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
      <button
        class="w-full flex items-center justify-between text-xs font-semibold uppercase tracking-wide mb-2 transition-colors"
        style="color: var(--ds-text-subtle);"
        onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text)'}
        onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
        onclick={toggleWorkspaceToolsSection}
      >
        <span>Workspace tools</span>
        <ChevronDown class={`w-4 h-4 transition-transform ${workspaceToolsExpanded ? 'rotate-180' : ''}`} />
      </button>

      {#if workspaceToolsExpanded}
        <div class="space-y-2">
          {#each filteredWorkspaceOnlyViews as view}
            {@const isToolActive = $currentRoute.view === `workspace-${view.id}`}
            <Tooltip content={view.tooltip} placement="right">
              <button
                onclick={() => navigate(getNavigationUrl(view.id))}
                class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
                style={isToolActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
                onmouseenter={(e) => { if (!isToolActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
                onmouseleave={(e) => { if (!isToolActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
              >
                <svelte:component this={view.icon} class="w-4 h-4" />
                {view.label}
              </button>
            </Tooltip>
          {/each}

          {#if canAdmin}
            <Tooltip content="Customize appearance and layout" placement="right">
              <button
                onclick={() => navigate(`/workspaces/${workspaceId}/look-and-feel`)}
                class="w-full text-left px-3 py-2 cursor-pointer rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
                style={$currentRoute.view === 'workspace-look-and-feel' ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
                onmouseenter={(e) => { if ($currentRoute.view !== 'workspace-look-and-feel') e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
                onmouseleave={(e) => { if ($currentRoute.view !== 'workspace-look-and-feel') e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
              >
                <Palette class="w-4 h-4" />
                Look and Feel
              </button>
            </Tooltip>
          {/if}

          <Tooltip content="Configure workspace settings and preferences" placement="right">
            <button
              onclick={() => navigate(`/workspaces/${workspaceId}/settings/general`)}
              class="w-full text-left px-3 py-2 cursor-pointer rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item"
              style={['workspace-settings', 'workspace-settings-general', 'workspace-settings-categories', 'workspace-settings-members', 'workspace-settings-configuration', 'workspace-settings-source-control', 'workspace-settings-danger'].includes($currentRoute.view) ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
              onmouseenter={(e) => { if (!['workspace-settings', 'workspace-settings-general', 'workspace-settings-categories', 'workspace-settings-members', 'workspace-settings-configuration', 'workspace-settings-source-control', 'workspace-settings-danger'].includes($currentRoute.view)) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
              onmouseleave={(e) => { if (!['workspace-settings', 'workspace-settings-general', 'workspace-settings-categories', 'workspace-settings-members', 'workspace-settings-configuration', 'workspace-settings-source-control', 'workspace-settings-danger'].includes($currentRoute.view)) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
            >
              <Settings class="w-4 h-4" />
              Settings
            </button>
          </Tooltip>
        </div>
      {/if}
    </div>
  </nav>
  </div>
{/if}

<style>
  /* Enhanced navigation item transitions */
  :global(.workspace-nav-item) {
    transition:
      background-color var(--duration-normal, 200ms) var(--ease-smooth, ease),
      color var(--duration-fast, 100ms) var(--ease-smooth, ease),
      transform var(--duration-fast, 100ms) var(--ease-spring, cubic-bezier(0.34, 1.56, 0.64, 1));
  }

  :global(.workspace-nav-item:hover) {
    transform: translateX(4px);
  }

  :global(.workspace-nav-item:active) {
    transform: translateX(2px) scale(0.98);
  }

  /* Staggered entrance animation for nav sections */
  nav {
    animation: fade-up var(--duration-normal, 200ms) var(--ease-smooth, ease) forwards;
  }

  /* Section header animation */
  nav .border-t {
    animation: fade-up var(--duration-slow, 300ms) var(--ease-smooth, ease) forwards;
    animation-delay: 100ms;
  }

  /* Reduced motion support */
  @media (prefers-reduced-motion: reduce) {
    :global(.workspace-nav-item:hover),
    :global(.workspace-nav-item:active) {
      transform: none;
    }

    nav,
    nav .border-t {
      animation: none;
    }
  }
</style>
