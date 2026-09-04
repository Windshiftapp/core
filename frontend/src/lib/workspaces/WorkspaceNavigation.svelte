<script>
  import { onMount } from 'svelte';
  import {
    IconPlus as Plus,
    IconArrowLeft as ArrowLeft,
    IconSquareCheck as CheckSquare,
    IconCalendar as Calendar,
    IconHome as Home,
    IconSettings as Settings,
    IconBook as BookOpen,
    IconChevronDown as ChevronDown,
    IconChevronRight,
    IconGripVertical as Grip,
    IconPalette as Palette,
    IconSparkles as Sparkles,
    IconPencil as Pencil,
  } from '@tabler/icons-svelte-runes';
  import { workspaceIconMap } from '../utils/icons.js';
  import { workspaceViewItems, workspaceOnlyViews, testNavigationItems, workspaceSettingsItems, workspaceSettingsViews, workspaceSettingsRoute } from '../navigation/workspaceNavigation.js';
  import { navItemStyle, onNavMouseEnter, onNavMouseLeave } from '../navigation/navItemStyle.js';
  import { navigate, currentRoute } from '../router.js';
  import { currentWorkspace, workspacePermissions } from '../stores';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import { api } from '../api.js';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import ScrollableSidebar from '../layout/ScrollableSidebar.svelte';
  import SidebarResizeHandle from '../layout/SidebarResizeHandle.svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import PagesNavSidebar from '../features/pages/PagesNavSidebar.svelte';
  import WorkspaceAdminNav from './WorkspaceAdminNav.svelte';
  import { i18n, t } from '../stores/i18n.svelte.js';
  import { workspaceGradientIndex, applyToAllViews, loadWorkspaceGradient, getGradientStyle } from '../stores/workspaceGradient.svelte.js';
  import {
    uiStore,
    WS_SIDEBAR_DEFAULT_WIDTH,
    WS_SIDEBAR_MAX_WIDTH,
    WS_SIDEBAR_MIN_WIDTH,
  } from '../stores/ui.svelte.js';

  const MIN_WIDTH = WS_SIDEBAR_MIN_WIDTH;
  const MAX_WIDTH = WS_SIDEBAR_MAX_WIDTH;
  const COLLAPSE_THRESHOLD = 100;
  const COLLAPSED_WIDTH = 48;

  const PERSONAL_NAV_ITEMS = [
    { icon: CheckSquare, labelKey: 'personal.myTasks', route: '/personal', view: 'personal-workspace' },
    { icon: BookOpen, labelKey: 'personal.reviews', route: '/personal/reviews', view: 'workspace-reviews' },
    { icon: Calendar, labelKey: 'personal.weeklyCalendar', route: '/personal/calendar', view: 'workspace-calendar' },
    { icon: Sparkles, labelKey: 'settings.aiFeatures.features.plan_my_day.name', route: '/personal/plan', view: 'personal-plan' },
  ];

  // Full list of workspace admin route views (registry-driven).
  const SETTINGS_VIEWS = workspaceSettingsViews;

  let sidebarWidth = $derived($uiStore.wsSidebarWidth);
  let isCollapsed = $derived($uiStore.wsSidebarCollapsed);

  let { workspaceId = null } = $props();

  let collections = $state([]);
  let allCollections = $state([]);
  let currentCollectionId = $state(null); // Track by ID instead of name
  let currentCollectionName = $state(''); // For display
  let collectionDisplayName = $derived(currentCollectionId ? currentCollectionName : t('collections.allItems'));
  let collectionDropdownItems = $state([]);
  let testsExpanded = $state(true);
  let workspaceToolsExpanded = $state(true);
  let lastCollectionId = undefined; // Plain variable to prevent infinite loop in $effect

  // Workspace view registries live in navigation/workspaceNavigation.js
  const workspaceOnlyViewIds = new Set(
    workspaceOnlyViews.map(view => view.id)
  );
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
  const activeTestNavId = $derived.by(() => getActiveTestNavId($currentRoute));
  const isSettingsView = $derived(SETTINGS_VIEWS.includes($currentRoute.view));
  const defaultCollectionView = workspaceViewItems[0]?.id || 'backlog';

  // Permission-based visibility
  const canViewTests = $derived.by(() => workspacePermissions.canViewTests(workspaceId));
  const canManageActions = $derived.by(() => workspacePermissions.canManageActions(workspaceId));
  const canAdmin = $derived.by(() => workspacePermissions.canAdminWorkspace(workspaceId));

  // Filter workspace-only views based on permissions
  const filteredWorkspaceOnlyViews = $derived.by(() => {
    return workspaceOnlyViews.filter(view => {
      if (view.id === 'agents') return canAdmin;
      if (view.id === 'actions') return canManageActions;
      return true;
    });
  });

  // Gradient detection
  const gradientStyle = $derived.by(() => ($applyToAllViews && $workspaceGradientIndex > 0) ? getGradientStyle($workspaceGradientIndex) : null);
  const hasGradient = $derived.by(() => gradientStyle !== null);
  const sidebarBgClass = $derived.by(() => hasGradient ? 'backdrop-blur-sm' : '');
  const sidebarBgStyle = $derived.by(() => hasGradient
    ? 'background-color: color-mix(in srgb, var(--ds-surface) 95%, transparent); border-color: var(--ds-border);'
    : 'background-color: var(--ds-surface); border-color: var(--ds-border);');

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
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
    i18n.locale;
    buildCollectionDropdownItems();
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
      currentCollectionName = collection?.name || '';
    } else {
      currentCollectionId = null;
      currentCollectionName = '';
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
      if (error?.name === 'AbortError') return;
      console.error('Failed to load collections:', error);
      collections = [];
      currentCollectionId = null;
      currentCollectionName = '';
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
    if (!name) return t('common.workspace');
    // Truncate to ~20 characters to fit in 2 lines max
    if (name.length <= 20) return name;
    return name.substring(0, 17) + '...';
  }

  function buildCollectionDropdownItems() {
    // Build the items array first, then assign once to avoid multiple state updates
    const items = [];

    // Always add the unfiltered workspace view first (shows all items)
    const workspaceName = truncateWorkspaceName($currentWorkspace?.name);
    items.push({
      id: 'default',
      type: 'regular',
      title: `${workspaceName} — ${t('collections.allItems')}`,
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
        title: t('collections.createCollection'),
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

  function toggleTestsSection() {
    testsExpanded = !testsExpanded;
  }

  function viewLabel(view) {
    return t(view.labelKey);
  }

  function viewTooltip(view) {
    return t(view.tooltipKey || view.labelKey);
  }

  function selectCollection(collection) {
    if (collection === null) {
      currentCollectionId = null;
      currentCollectionName = '';
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
    } else if (currentView === 'item-detail') {
      const currentItemId = $currentRoute.params.itemId;
      if (currentItemId) {
        const url = currentCollectionId
          ? `/workspaces/${workspaceId}/collections/${currentCollectionId}/items/${currentItemId}`
          : `/workspaces/${workspaceId}/items/${currentItemId}`;
        navigate(url);
      }
    } else if (currentView && workspaceTestViewIds.has(currentView)) {
      // Test views are not collection-sensitive, redirect to default collection view
      if (currentCollectionId !== null) {
        navigate(getNavigationUrl(defaultCollectionView));
      } else {
        const url = getTestNavigationUrl(getTestNavIdFromView(currentView));
        navigate(url);
      }
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

  function isSettingsActive() {
    return SETTINGS_VIEWS.includes($currentRoute.view);
  }

  function isWorkspaceViewActive(view) {
    return view.activeViews?.includes($currentRoute.view)
      || $currentRoute.view === `workspace-${view.id}`;
  }
</script>

{#snippet resizeHandle()}
  <SidebarResizeHandle
    width={sidebarWidth}
    minWidth={MIN_WIDTH}
    maxWidth={MAX_WIDTH}
    defaultWidth={WS_SIDEBAR_DEFAULT_WIDTH}
    collapsed={isCollapsed}
    collapsedWidth={COLLAPSED_WIDTH}
    collapseThreshold={COLLAPSE_THRESHOLD}
    label={t('aria.resizeNavigation')}
    title={t('aria.sidebarResizeHint')}
    testId="workspace-sidebar-resize"
    onresize={(width) => uiStore.wsSidebarWidth = width}
    oncollapsechange={(collapsed) => uiStore.wsSidebarCollapsed = collapsed}
  />
{/snippet}

{#snippet workspaceAvatar(collapsed = false)}
  {#if $currentWorkspace?.avatar_url}
    <div class={collapsed ? 'w-8 h-8 flex-shrink-0' : 'flex items-center justify-center w-10 h-10 flex-shrink-0'}>
      <img
        src={$currentWorkspace.avatar_url}
        alt={$currentWorkspace.name}
        class="w-8 h-8 rounded-md object-cover"
      />
    </div>
  {:else}
    {@const WorkspaceIcon = workspaceIconMap[$currentWorkspace?.icon] || Grip}
    <div class={collapsed ? 'w-8 h-8 rounded-md flex items-center justify-center flex-shrink-0' : 'flex items-center justify-center w-10 h-10 flex-shrink-0'}>
      <div
        class="w-8 h-8 rounded-md flex items-center justify-center"
        style="background-color: {$currentWorkspace?.color || ($currentWorkspace?.is_personal ? '#f97316' : '#3b82f6')};"
      >
        <WorkspaceIcon size={collapsed ? 16 : 18} color="white" />
      </div>
    </div>
  {/if}
{/snippet}

{#snippet workspaceHeader({ backLink = false } = {})}
  <div class="px-4 pb-4 border-b" style="border-color: var(--ds-border);">
    <div class="flex items-center gap-3">
      {@render workspaceAvatar(false)}
      <div class="flex-1 min-w-0">
        <Tooltip content={$currentWorkspace?.name || t('common.workspace')}>
          <div class="font-medium text-sm truncate" style="color: var(--ds-text);">
            {$currentWorkspace?.name || t('common.workspace')}
          </div>
        </Tooltip>
        {#if backLink}
          <a class="workspace-header-back-link" href={`/workspaces/${workspaceId}`} data-testid="workspace-back-link">
            <ArrowLeft size={13} />
            <span>{t('workspaceSettings.backToWorkspace')}</span>
          </a>
        {:else if $currentWorkspace?.is_personal}
          <div class="text-xs text-orange-600">{t('components.userAvatar.myWorkspace')}</div>
        {:else if $currentWorkspace?.description}
          <Tooltip content={$currentWorkspace.description}>
            <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
              {$currentWorkspace.description}
            </div>
          </Tooltip>
        {/if}
      </div>
    </div>
  </div>
{/snippet}

{#snippet navLink(item)}
  {@const ItemIcon = item.icon}
  <Tooltip content={item.tooltip || item.label} placement="right">
    <a
      href={item.href}
      data-testid={item.testId}
      class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 workspace-nav-item no-underline"
      style={navItemStyle(item.isActive)}
      onmouseenter={(e) => onNavMouseEnter(e, item.isActive)}
      onmouseleave={(e) => onNavMouseLeave(e, item.isActive)}
    >
      <ItemIcon class="w-4 h-4" />
      {item.label}
    </a>
  </Tooltip>
{/snippet}

{#snippet collapsedNavIcon(item)}
  {@const ItemIcon = item.icon}
  <Tooltip content={item.label} placement="right">
    <a
      href={item.href}
      data-testid={item.testId}
      class="w-10 h-10 rounded flex items-center justify-center transition-colors no-underline"
      style={navItemStyle(item.isActive)}
      onmouseenter={(e) => onNavMouseEnter(e, item.isActive)}
      onmouseleave={(e) => onNavMouseLeave(e, item.isActive)}
    >
      <ItemIcon size={20} />
    </a>
  </Tooltip>
{/snippet}

{#snippet sectionDivider()}
  <div class="w-8 border-t my-1" style="border-color: var(--ds-border);"></div>
{/snippet}

{#snippet regularSidebarHeader()}
  {@render workspaceHeader()}

  <!-- Keep workspace context visible while its navigation scrolls. -->
  <div class="px-4 pt-2 mb-6">
    <Tooltip content={t('collections.collection')} placement="right">
      <DropdownMenu
        triggerText={collectionDisplayName}
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
{/snippet}

{#snippet sidebarResizeHandle()}
  {@render resizeHandle()}
{/snippet}

{#snippet drilldownSidebarHeader()}
  {@render workspaceHeader({ backLink: true })}
{/snippet}

{#snippet collapsedSidebarHeader()}
  <div class="h-10 mb-2 w-full flex items-center justify-center">
    {@render workspaceAvatar(true)}
  </div>
{/snippet}

{#snippet collapsedSidebarContent()}
  {#if isSettingsView}
    <!-- Collapsed admin rail: back arrow + a module icon per settings page. -->
    <div class="flex flex-col items-center space-y-1 mt-6">
      {@render collapsedNavIcon({ href: `/workspaces/${workspaceId}`, label: t('workspaceSettings.backToWorkspace'), icon: ArrowLeft, isActive: false })}
      {@render sectionDivider()}
      {#each workspaceSettingsItems as item (item.id)}
        {@render collapsedNavIcon({ href: workspaceSettingsRoute(workspaceId, item.id), label: t(item.labelKey), icon: item.icon, isActive: $currentRoute.view === item.view })}
      {/each}
    </div>
  {:else if $currentWorkspace?.is_personal}
    <div class="flex flex-col items-center space-y-1 mt-6">
      {#each PERSONAL_NAV_ITEMS as item (item.route)}
        {@render collapsedNavIcon({ href: item.route, label: t(item.labelKey), icon: item.icon, isActive: $currentRoute.view === item.view })}
      {/each}
    </div>
  {:else}
    <div class="flex flex-col items-center space-y-1 mt-6">
      {@render collapsedNavIcon({ href: getNavigationUrl('overview'), label: t('workspaceSettings.views.overview'), icon: Home, isActive: $currentRoute.view === 'workspace-overview' })}

      {#each workspaceViewItems as view (view.id)}
        {@render collapsedNavIcon({ href: getNavigationUrl(view.id), label: viewLabel(view), icon: view.icon, isActive: $currentRoute.view === `workspace-${view.id}` })}
      {/each}

      {#if $moduleSettings.test_management_enabled && canViewTests && !currentCollectionId}
        {@render sectionDivider()}
        {#each testNavigationItems as view (view.id)}
          {@render collapsedNavIcon({ href: getTestNavigationUrl(view.id), label: viewLabel(view), icon: view.icon, isActive: activeTestNavId === view.id })}
        {/each}
      {/if}

      {@render sectionDivider()}
      {#each filteredWorkspaceOnlyViews as view (view.id)}
        {@render collapsedNavIcon({ href: getNavigationUrl(view.id), label: viewLabel(view), icon: view.icon, testId: view.testId, isActive: isWorkspaceViewActive(view) })}
      {/each}

      {#if canAdmin}
        {@render collapsedNavIcon({ href: `/workspaces/${workspaceId}/settings/general`, label: t('workspaceSettings.title'), icon: Settings, isActive: isSettingsActive() })}
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet collapsedSidebarFooter()}
  <Tooltip content={t('collections.expandSidebar')} placement="right">
    <button
      type="button"
      onclick={() => uiStore.wsSidebarCollapsed = false}
      class="w-10 h-10 rounded flex items-center justify-center transition-colors"
      style="color: var(--ds-text-subtle);"
      onmouseenter={(e) => e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'}
      onmouseleave={(e) => e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'}
    >
      <IconChevronRight size={20} />
    </button>
  </Tooltip>

  {@render resizeHandle()}
{/snippet}

{#if isCollapsed}
  <!-- Collapsed icon-only sidebar -->
  <ScrollableSidebar
    class="workspace-resizable-sidebar relative h-full flex-shrink-0 border-r items-center py-4 {sidebarBgClass}"
    style="width: {COLLAPSED_WIDTH}px; {sidebarBgStyle}"
    aria-label={t('aria.mainNavigation')}
    header={collapsedSidebarHeader}
    footer={collapsedSidebarFooter}
    reserveScrollbarSpace={false}
    scrollClass="w-full"
    scrollTestid="workspace-navigation-scroll"
  >
    {@render collapsedSidebarContent()}
  </ScrollableSidebar>
{:else if isSettingsView}
  <!-- Workspace admin drilldown: keep the workspace identity header (with a
       back link) and swap the body for the folded admin module nav. -->
  <ScrollableSidebar
    class="workspace-resizable-sidebar sidebar-mode-panel relative h-full flex-shrink-0 {sidebarBgClass} border-r flex flex-col py-4"
    style="width: {sidebarWidth}px; min-width: {MIN_WIDTH}px; max-width: {MAX_WIDTH}px; {sidebarBgStyle}"
    data-testid="workspace-admin-sidebar"
    aria-label={t('workspaceSettings.title')}
    header={drilldownSidebarHeader}
    footer={sidebarResizeHandle}
    scrollTestid="workspace-admin-navigation-scroll"
  >
    <WorkspaceAdminNav {workspaceId} />
  </ScrollableSidebar>
{:else if $currentRoute.view === 'workspace-pages' || $currentRoute.view === 'workspace-pages-archived'}
  <!-- Pages drilldown keeps the common workspace identity header and swaps the body for the page tree. -->
  <ScrollableSidebar
    class="workspace-resizable-sidebar sidebar-mode-panel relative h-full flex-shrink-0 {sidebarBgClass} border-r flex flex-col py-4"
    style="width: {sidebarWidth}px; min-width: {MIN_WIDTH}px; max-width: {MAX_WIDTH}px; {sidebarBgStyle}"
    data-testid="workspace-pages-sidebar"
    aria-label={t('pages.treeHeading')}
    header={drilldownSidebarHeader}
    footer={sidebarResizeHandle}
    scrollContent={false}
    reserveScrollbarSpace={false}
  >
    <div class="flex flex-1 min-h-0">
      <PagesNavSidebar {workspaceId} embedded />
    </div>
  </ScrollableSidebar>
{:else if $currentWorkspace?.is_personal}
  <!-- Simplified Personal Workspace Sidebar -->
  <ScrollableSidebar
    class="workspace-resizable-sidebar sidebar-mode-panel relative h-full flex-shrink-0 {sidebarBgClass} border-r py-4"
    style="width: {sidebarWidth}px; min-width: {MIN_WIDTH}px; max-width: {MAX_WIDTH}px; {sidebarBgStyle}"
    aria-label={t('aria.mainNavigation')}
    header={workspaceHeader}
    footer={sidebarResizeHandle}
    scrollTestid="workspace-navigation-scroll"
  >
    <nav class="px-4 pt-2 space-y-1 pb-2">
      {#each PERSONAL_NAV_ITEMS as item (item.route)}
        {@render navLink({ href: item.route, label: t(item.labelKey), icon: item.icon, isActive: $currentRoute.view === item.view })}
      {/each}
    </nav>
  </ScrollableSidebar>
{:else}
  <!-- Regular Workspace Navigation Sidebar -->
  <ScrollableSidebar
    class="workspace-resizable-sidebar sidebar-mode-panel relative h-full flex-shrink-0 {sidebarBgClass} border-r py-4"
    style="width: {sidebarWidth}px; min-width: {MIN_WIDTH}px; max-width: {MAX_WIDTH}px; {sidebarBgStyle}"
    aria-label={t('aria.mainNavigation')}
    header={regularSidebarHeader}
    footer={sidebarResizeHandle}
    scrollTestid="workspace-navigation-scroll"
  >
    <nav class="px-4 space-y-1 pb-2">
      {@render navLink({ href: getNavigationUrl('overview'), label: t('workspaceSettings.views.overview'), tooltip: t('commandPalette.commands.workspaceOverview.description'), icon: Home, isActive: $currentRoute.view === 'workspace-overview' })}

      {#each workspaceViewItems as view (view.id)}
        {@render navLink({ href: getNavigationUrl(view.id), label: viewLabel(view), tooltip: viewTooltip(view), icon: view.icon, testId: view.testId, isActive: $currentRoute.view === `workspace-${view.id}` })}
      {/each}

      {#if currentCollectionId}
        <div class="mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
          <div class="text-xs font-semibold uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">
            {t('collections.collection')}
          </div>
          {@render navLink({ href: `/collections/${currentCollectionId}?workspace=${workspaceId}`, label: t('collections.editCollection'), icon: Pencil, isActive: false })}
        </div>
      {/if}

      {#if $moduleSettings.test_management_enabled && canViewTests && !currentCollectionId}
        <div class="mt-4 pt-4 border-t space-y-1" style="border-color: var(--ds-border);">
          <button
            type="button"
            class="w-full flex items-center justify-between text-xs font-semibold uppercase tracking-wide transition-colors"
            style="color: var(--ds-text-subtle);"
            aria-controls="workspace-tests-navigation"
            aria-expanded={testsExpanded}
            data-testid="workspace-tests-toggle"
            onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text)'}
            onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
            onclick={toggleTestsSection}
          >
            <span>{t('commandPalette.commands.tests.label')}</span>
            <ChevronDown class={`w-4 h-4 transition-transform ${testsExpanded ? 'rotate-180' : ''}`} />
          </button>

          {#if testsExpanded}
            <div id="workspace-tests-navigation" class="space-y-1" data-testid="workspace-tests-navigation">
              {#each testNavigationItems as view (view.id)}
                {@render navLink({ href: getTestNavigationUrl(view.id), label: viewLabel(view), tooltip: viewTooltip(view), icon: view.icon, isActive: activeTestNavId === view.id })}
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <div class="mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
        <button
          type="button"
          class="w-full flex items-center justify-between text-xs font-semibold uppercase tracking-wide mb-2 transition-colors"
          style="color: var(--ds-text-subtle);"
          onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text)'}
          onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
          onclick={toggleWorkspaceToolsSection}
        >
          <span>{t('actions.config.tools')}</span>
          <ChevronDown class={`w-4 h-4 transition-transform ${workspaceToolsExpanded ? 'rotate-180' : ''}`} />
        </button>

        {#if workspaceToolsExpanded}
          <div class="space-y-1" data-testid="workspace-tools-navigation">
            {#each filteredWorkspaceOnlyViews as view (view.id)}
              {@render navLink({ href: getNavigationUrl(view.id), label: viewLabel(view), tooltip: viewTooltip(view), icon: view.icon, testId: view.testId, isActive: isWorkspaceViewActive(view) })}
            {/each}

            {#if canAdmin}
              {@render navLink({ href: `/workspaces/${workspaceId}/look-and-feel`, label: t('lookAndFeel.title'), tooltip: t('lookAndFeel.subtitle'), icon: Palette, isActive: $currentRoute.view === 'workspace-look-and-feel' })}
              {@render navLink({ href: `/workspaces/${workspaceId}/settings/general`, label: t('workspaceSettings.title'), tooltip: t('workspaceSettings.subtitle', { name: $currentWorkspace?.name || t('common.workspace') }), icon: Settings, isActive: isSettingsActive() })}
            {/if}
          </div>
        {/if}
      </div>
    </nav>
  </ScrollableSidebar>
{/if}

<style>
  :global(.sidebar-mode-panel) {
    animation: sidebar-mode-enter 180ms var(--ease-smooth, ease) both;
  }

  @keyframes sidebar-mode-enter {
    from {
      opacity: 0;
      transform: translateX(10px);
    }

    to {
      opacity: 1;
      transform: translateX(0);
    }
  }

  .workspace-header-back-link {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    margin-top: 0.125rem;
    color: var(--ds-text-subtle);
    font-size: 0.75rem;
    line-height: 1rem;
    text-decoration: none;
    transition: color 120ms ease;
  }

  .workspace-header-back-link:hover {
    color: var(--ds-text);
    text-decoration: underline;
  }

  /* Keep the shared handle's outer half clickable. */
  :global(.workspace-resizable-sidebar.scrollable-sidebar) {
    overflow: visible;
  }

  /* Enhanced navigation item transitions */
  :global(.workspace-nav-item) {
    transition:
      background-color var(--duration-normal, 200ms) var(--ease-smooth, ease),
      color var(--duration-fast, 100ms) var(--ease-smooth, ease),
      transform var(--duration-fast, 100ms) var(--ease-smooth, cubic-bezier(0.16, 1, 0.3, 1));
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

    :global(.sidebar-mode-panel),
    nav,
    nav .border-t {
      animation: none;
    }
  }
</style>
