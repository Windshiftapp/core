<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate, isWorkspaceRoute } from '../router.js';
  import { testingStore, authStore, permissionStore, uiStore, currentWorkspace, workspacesStore, workspacePermissions, ssoStore } from '../stores';
  import EmailVerificationBanner from '../features/notifications/EmailVerificationBanner.svelte';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import { t } from '../stores/i18n.svelte.js';
  import NotFound from './NotFound.svelte';
  import Workspaces from '../workspaces/Workspaces.svelte';
  import WorkspaceSettings from '../workspaces/WorkspaceSettings.svelte';
  import Collections from '../features/collections/Collections.svelte';
  import CollectionsList from '../features/collections/CollectionsList.svelte';
  import NotificationsPage from './NotificationsPage.svelte';
  import UserProfile from './UserProfile.svelte';
  import Security from './Security.svelte';
  import SearchPage from './SearchPage.svelte';
  import About from './About.svelte';
  import Channels from '../features/channels/Channels.svelte';
  import Customers from '../workspaces/Customers.svelte';
  import Footer from '../layout/Footer.svelte';
  import {
    Layers3, Search, BarChart3, Settings, Plus, Sheet, Target, User, Notebook, Grip, GitBranch, MapPin, Shield, Home, Clock, CheckSquare, Calendar, LifeBuoy, MoreHorizontal, Inbox, SquareKanban, FolderOpen, Milestone, Library, Package, Users
  } from 'lucide-svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import GlobalConfirmDialog from '../dialogs/GlobalConfirmDialog.svelte';
  import FloatingTimer from '../features/time/FloatingTimer.svelte';
  import NotificationTray from '../features/notifications/NotificationTray.svelte';
  import ToastContainer from '../features/notifications/ToastContainer.svelte';
  import UserAvatar from '../components/UserAvatar.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Button from '../components/Button.svelte';
  import PermissionGuard from '../layout/PermissionGuard.svelte';
  import UnauthorizedAccess from './UnauthorizedAccess.svelte';
  import WorkspaceNavigation from '../workspaces/WorkspaceNavigation.svelte';
  import { getShortcut, matchesShortcut, getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import { workspaceIconMap } from '../utils/icons.js';

  // Get shortcut configurations
  const commandPaletteShortcut = getShortcut('global', 'commandPalette');

  let showCommandPalette = $state(false);
  let showCreateModal = $state(false);
  let showEmailVerificationBanner = $state(false);

  // Lazy loaded components registry
  let componentRegistry = $state(new Map());
  let loadingRoutes = $state(new Set());
  let lastSpaceTime = 0;
  const DOUBLE_SPACE_THRESHOLD = 300; // milliseconds
let workspaceSearchQuery = $state('');

  // Component loaders with literal import paths for Vite's static analysis
  const componentLoaders = {
    'admin': () => import('./Admin.svelte'),
    'time': () => import('../features/time/Time.svelte'),
    'test-cases': () => import('../features/testing/TestCases.svelte'),
    'test-sets': () => import('../features/testing/TestSets.svelte'),
    'test-templates': () => import('../features/testing/TestTemplates.svelte'),
    'test-runs': () => import('../features/testing/TestRuns.svelte'),
    'test-reports': () => import('../features/testing/TestReports.svelte'),
    'test-steps': () => import('../features/testing/TestSteps.svelte'),
    'test-execution': () => import('../features/testing/TestExecution.svelte'),
    'test-run-detail': () => import('../features/testing/TestRunDetail.svelte'),
    'test-template-detail': () => import('../features/testing/TestTemplateDetail.svelte'),
    'milestones': () => import('../features/milestones/Milestones.svelte'),
    'milestone-detail': () => import('../features/milestones/MilestoneDetail.svelte'),
    'iterations': () => import('../features/iterations/Iterations.svelte'),
    'iteration-detail': () => import('../features/iterations/IterationDetail.svelte'),
    'assets': () => import('../features/assets/AssetBrowser.svelte'),
    'asset-detail': () => import('../features/assets/AssetBrowser.svelte'),
    'workspace-board': () => import('../features/collections/CollectionBoard.svelte'),
    'workspace-board-config': () => import('../settings/BoardConfigurationPage.svelte'),
    'workspace-backlog': () => import('../features/collections/CollectionBacklog.svelte'),
    'workspace-list': () => import('../features/collections/CollectionList.svelte'),
    'workspace-tree': () => import('../features/collections/CollectionTree.svelte'),
    'workspace-map': () => import('../features/collections/CollectionMap.svelte'),
    'workspace-iterations': () => import('../features/iterations/Iterations.svelte'),
    'workspace-milestones': () => import('../features/milestones/Milestones.svelte'),
    'command-palette': () => import('../layout/CommandPalette.svelte'),
    'create-modal': () => import('../dialogs/CreateModal.svelte'),
    'homepage': () => import('./Homepage.svelte'),
    'item-detail': () => import('../features/items/ItemDetail.svelte'),
    'personal-task-detail': () => import('../features/personal/PersonalTaskDetail.svelte'),
    'workspace-detail': () => import('../workspaces/WorkspaceWelcome.svelte'),
    'workspace-overview': () => import('../workspaces/WorkspaceWelcome.svelte'),
    'personal-workspace': () => import('../workspaces/WorkspaceDetail.svelte'),
    'workspace-calendar': () => import('../features/time/WeeklyCalendar.svelte'),
    'workspace-reviews': () => import('../features/personal/PersonalReview.svelte'),
    'workflow-designer': () => import('../features/workflows/WorkflowDesigner.svelte'),
    'configuration-set-detail': () => import('../settings/ConfigurationSetDetail.svelte')
  };

  // Preload all chunks after initial load for faster navigation
  function preloadChunks() {
    // Use requestIdleCallback for non-blocking preload, fallback to setTimeout
    const schedulePreload = window.requestIdleCallback || ((cb) => setTimeout(cb, 1000));

    schedulePreload(() => {
      Object.values(componentLoaders).forEach(loader => {
        loader().catch(() => {}); // Silently preload, ignore errors
      });
    });
  }

  // Route configuration for lazy-loaded components (metadata only)
  const routeConfig = {
    'admin': {
      loadingMsg: 'Loading Admin Panel...',
      errorMsg: 'Failed to load Admin Panel',
      requirePermission: 'systemAdmin'
    },
    'time': {
      loadingMsg: 'Loading Time & Projects...',
      errorMsg: 'Failed to load Time & Projects'
    },
    'test-cases': {
      loadingMsg: 'Loading Test Cases...',
      errorMsg: 'Failed to load Test Cases',
      wrapper: 'none',
      getProps: (route) => ({ workspaceId: route.params.id })
    },
    'test-sets': {
      loadingMsg: 'Loading Test Plans...',
      errorMsg: 'Failed to load Test Plans',
      wrapper: 'none',
      getProps: (route) => ({ workspaceId: route.params.id })
    },
    'test-templates': {
      loadingMsg: 'Loading Test Templates...',
      errorMsg: 'Failed to load Test Templates',
      wrapper: 'none',
      getProps: (route) => ({ workspaceId: route.params.id })
    },
    'test-runs': {
      loadingMsg: 'Loading Test Runs...',
      errorMsg: 'Failed to load Test Runs',
      wrapper: 'none',
      getProps: (route) => ({ workspaceId: route.params.id })
    },
    'test-reports': {
      loadingMsg: 'Loading Test Reports...',
      errorMsg: 'Failed to load Test Reports',
      wrapper: 'none',
      getProps: (route) => ({ workspaceId: route.params.id })
    },
    'test-steps': {
      loadingMsg: 'Loading Test Steps...',
      errorMsg: 'Failed to load Test Steps',
      wrapper: 'none',
      getProps: (route) => ({ workspaceId: route.params.id })
    },
    'test-execution': {
      loadingMsg: 'Loading Test Execution...',
      errorMsg: 'Failed to load Test Execution',
      wrapper: 'none'
    },
    'test-run-detail': {
      loadingMsg: 'Loading Test Run Details...',
      errorMsg: 'Failed to load Test Run Details',
      wrapper: 'none'
    },
    'test-template-detail': {
      loadingMsg: 'Loading Template Details...',
      errorMsg: 'Failed to load Template Details',
      wrapper: 'none'
    },
    'milestones': {
      loadingMsg: 'Loading Milestones...',
      errorMsg: 'Failed to load Milestones',
      wrapper: 'surface-full'
    },
    'milestone-detail': {
      loadingMsg: 'Loading Milestone...',
      errorMsg: 'Failed to load Milestone',
      wrapper: 'surface-full',
      getProps: (route) => ({ milestoneId: route.params.id })
    },
    'iterations': {
      loadingMsg: 'Loading Iterations...',
      errorMsg: 'Failed to load Iterations',
      wrapper: 'surface-full',
      getProps: (route) => ({ typeId: route.params.typeId })
    },
    'iteration-detail': {
      loadingMsg: 'Loading Iteration...',
      errorMsg: 'Failed to load Iteration',
      wrapper: 'surface-full',
      getProps: (route) => ({ iterationId: route.params.id })
    },
    'assets': {
      loadingMsg: 'Loading Assets...',
      errorMsg: 'Failed to load Assets',
      wrapper: 'surface-full'
    },
    'asset-detail': {
      loadingMsg: 'Loading Asset...',
      errorMsg: 'Failed to load Asset',
      wrapper: 'surface-full',
      getProps: (route) => ({ assetId: route.params.id })
    },
    'workspace-board': {
      loadingMsg: 'Loading Board View...',
      errorMsg: 'Failed to load Board View',
      getProps: (route) => ({ workspaceId: route.params.id, collectionId: route.params.collectionId })
    },
    'workspace-board-config': {
      loadingMsg: 'Loading Board Configuration...',
      errorMsg: 'Failed to load Board Configuration',
      getProps: (route) => ({ workspaceId: route.params.id, collectionId: route.params.collectionId })
    },
    'workspace-backlog': {
      loadingMsg: 'Loading Backlog View...',
      errorMsg: 'Failed to load Backlog View',
      getProps: (route) => ({ workspaceId: route.params.id, collectionId: route.params.collectionId })
    },
    'workspace-list': {
      loadingMsg: 'Loading List View...',
      errorMsg: 'Failed to load List View',
      getProps: (route) => ({ workspaceId: route.params.id, collectionId: route.params.collectionId })
    },
    'workspace-tree': {
      loadingMsg: 'Loading Tree View...',
      errorMsg: 'Failed to load Tree View',
      getProps: (route) => ({ workspaceId: route.params.id, collectionId: route.params.collectionId })
    },
    'workspace-map': {
      loadingMsg: 'Loading Map View...',
      errorMsg: 'Failed to load Map View',
      getProps: (route) => ({ workspaceId: route.params.id, collectionId: route.params.collectionId })
    },
    'workspace-iterations': {
      loadingMsg: 'Loading Iterations...',
      errorMsg: 'Failed to load Iterations',
      wrapper: 'surface-full',
      getProps: (route) => ({ workspaceId: route.params.id })
    },
    'workspace-milestones': {
      loadingMsg: 'Loading Milestones...',
      errorMsg: 'Failed to load Milestones',
      wrapper: 'surface-full',
      getProps: (route) => ({ workspaceId: route.params.id })
    },
    'command-palette': {
      trigger: 'showCommandPalette'
    },
    'create-modal': {
      trigger: 'showCreateModal'
    },
    'homepage': {
      loadingMsg: 'Loading Homepage...',
      errorMsg: 'Failed to load Homepage',
      wrapper: 'surface-full'
    },
    'item-detail': {
      loadingMsg: 'Loading Item Details...',
      errorMsg: 'Failed to load Item Details',
      getProps: (route) => ({
        workspaceId: route.path.startsWith('/personal') ? $workspacesStore.personalWorkspace?.id : route.params.id,
        itemId: route.params.itemId,
        tab: route.query.tab || 'comments',
        moduleSettings: $moduleSettings
      })
    },
    'personal-task-detail': {
      loadingMsg: 'Loading Task...',
      errorMsg: 'Failed to load Task',
      getProps: (route) => ({
        workspaceId: route.path.startsWith('/personal') ? $workspacesStore.personalWorkspace?.id : route.params.id,
        itemId: route.params.itemId,
        isModal: false
      })
    },
    'workspace-detail': {
      loadingMsg: 'Loading Workspace...',
      errorMsg: 'Failed to load Workspace',
      getProps: (route) => ({ workspaceId: route.params.id, collectionId: route.params.collectionId })
    },
    'workspace-overview': {
      loadingMsg: 'Loading Workspace...',
      errorMsg: 'Failed to load Workspace',
      getProps: (route) => ({ workspaceId: route.params.id, collectionId: route.params.collectionId })
    },
    'personal-workspace': {
      loadingMsg: 'Loading Personal Workspace...',
      errorMsg: 'Failed to load Personal Workspace',
      getProps: () => ({ workspaceId: $workspacesStore.personalWorkspace?.id })
    },
    'workspace-calendar': {
      loadingMsg: 'Loading Calendar...',
      errorMsg: 'Failed to load Calendar',
      wrapper: 'surface-full',
      getProps: (route) => ({
        workspaceId: route.path.startsWith('/personal') ? $workspacesStore.personalWorkspace?.id : route.params.id
      })
    },
    'workspace-reviews': {
      loadingMsg: 'Loading Reviews...',
      errorMsg: 'Failed to load Reviews',
      wrapper: 'surface-full',
      getProps: (route) => ({
        currentUser: authStore.currentUser,
        workspaceId: route.path.startsWith('/personal') ? $workspacesStore.personalWorkspace?.id : route.params.id
      })
    },
    'workflow-designer': {
      loadingMsg: 'Loading workflow designer...',
      errorMsg: 'Failed to load workflow designer'
    },
    'configuration-set-detail': {
      loadingMsg: 'Loading configuration set...',
      errorMsg: 'Failed to load configuration set'
    }
  };

  const testViews = new Set([
    'test-cases',
    'test-sets',
    'test-templates',
    'test-runs',
    'test-reports',
    'test-steps',
    'test-run-detail',
    'test-template-detail',
    'test-execution',
    'test-case-detail',
    'test-set-detail'
  ]);

  function resolveRouteConfig(view) {
    if (!view) {
      return { key: null, config: null };
    }

    if (routeConfig[view]) {
      return { key: view, config: routeConfig[view] };
    }

    for (const [key, config] of Object.entries(routeConfig)) {
      if (config.matchViews?.includes(view)) {
        return { key, config };
      }
    }

    return { key: null, config: null };
  }

  // Compute the effective view, replacing item-detail with personal-task-detail for personal workspaces
  let effectiveView = $derived.by(() => {
    const view = $currentRoute.view;

    // Check if this is an item-detail view and if the workspace is personal
    if (view === 'item-detail') {
      const workspaceId = $currentRoute.path?.startsWith('/personal')
        ? $workspacesStore.personalWorkspace?.id
        : parseInt($currentRoute.params?.id);

      // Check if this workspace is the personal workspace
      if ($workspacesStore.personalWorkspace?.id && workspaceId === $workspacesStore.personalWorkspace?.id) {
        return 'personal-task-detail';
      }
    }

    return view;
  });
  
  onMount(async () => {
    // Load full app data for authenticated users
    // (MainApp only renders when user is authenticated, App.svelte handles auth/setup)
    await workspacesStore.load();
    // Also load personal workspace so it's available immediately
    workspacesStore.loadPersonalWorkspace();
    moduleSettings.load();
    // Load all permissions for permission checking (admin only)
    await permissionStore.loadAllPermissions(authStore.currentUser);
    // Load workspace permissions for current user
    const userId = authStore.currentUser?.id;
    if (userId) {
      workspacePermissions.loadPermissions(userId);
    }

    // Check for email verification pending (after SSO callback redirect)
    if (ssoStore.checkForEmailVerificationPending()) {
      showEmailVerificationBanner = true;
    } else {
      // Also check current verification status from API
      try {
        const status = await ssoStore.getVerificationStatus();
        if (status.configured && !status.email_verified) {
          showEmailVerificationBanner = true;
        }
      } catch (err) {
        console.warn('Failed to check email verification status:', err);
      }
    }

    // Preload chunks after app is ready for faster navigation
    preloadChunks();
  });

  // Global keydown listener for command palette and shortcuts
  onMount(() => {
    function handleGlobalKeydown(e) {
      // Check if we're in an input, textarea, or content-editable element
      const isInInputField = e.target.tagName === 'INPUT' ||
                            e.target.tagName === 'TEXTAREA' ||
                            e.target.contentEditable === 'true' ||
                            e.target.closest('[contenteditable="true"]');

      // Command palette shortcut (Ctrl/Cmd+K)
      if (matchesShortcut(e, commandPaletteShortcut)) {
        e.preventDefault();
        showCommandPalette = true;
        return;
      }

      if (e.code === 'Space') {
        const now = Date.now();

        // Only trigger command palette if not in an input field
        if (!isInInputField) {
          if (now - lastSpaceTime < DOUBLE_SPACE_THRESHOLD) {
            e.preventDefault();
            showCommandPalette = true;
          } else {
            // Prevent the first space from being inserted anywhere
            e.preventDefault();
          }
          lastSpaceTime = now;
        }
      } else if (e.key === 'c' && !e.ctrlKey && !e.metaKey && !e.altKey && !isInInputField) {
        // "C" key for create (only when not in input fields and no modifiers)
        e.preventDefault();
        showCreateDropdown();
      }
    }
    
    document.addEventListener('keydown', handleGlobalKeydown);
    
    // Listen for create modal events from other components
    function handleShowCreateModal(event) {
      showCreateModal = true;
      const detail = event.detail || {};

      if (detail.type) {
        // Increased delay to allow CreateModal to lazy load and mount before dispatching type
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('set-create-type', { detail: { type: detail.type } }));
        }, 200);
      }

      if (detail.workspaceId) {
        // Dispatch workspace selection after type so resetForm doesn't clear it
        const workspaceId = typeof detail.workspaceId === 'string'
          ? parseInt(detail.workspaceId, 10)
          : detail.workspaceId;

        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('set-create-workspace', {
            detail: { workspaceId }
          }));
        }, detail.type ? 250 : 200);
      }
    }
    
    window.addEventListener('show-create-modal', handleShowCreateModal);
    
    // Listen for workspace refresh events from other components
    function handleRefreshWorkspaces() {
      workspacesStore.reload();
    }

    window.addEventListener('refresh-workspaces', handleRefreshWorkspaces);
    
    return () => {
      document.removeEventListener('keydown', handleGlobalKeydown);
      window.removeEventListener('show-create-modal', handleShowCreateModal);
      window.removeEventListener('refresh-workspaces', handleRefreshWorkspaces);
    };
  });



  // Load current workspace when route changes (only for workspace routes)
  $effect(() => {
    // Handle personal workspace routes
    if ($currentRoute.path?.startsWith('/personal') && ($currentRoute.view?.startsWith('workspace-') || $currentRoute.view === 'personal-workspace' || $currentRoute.view === 'item-detail')) {
      const personalWorkspaceId = $workspacesStore.personalWorkspace?.id;
      if (personalWorkspaceId) {
        currentWorkspace.load(personalWorkspaceId);
      } else {
        // Personal workspace not loaded yet - trigger loading
        // The effect will re-run when personalWorkspace is set in the store
        workspacesStore.loadPersonalWorkspace();
      }
    }
    // Handle regular workspace routes
    else if ($currentRoute.params?.id && ($currentRoute.view?.startsWith('workspace-') || $currentRoute.view === 'workspace' || $currentRoute.view === 'item-detail' || $currentRoute.view === 'item' || testViews.has($currentRoute.view))) {
      currentWorkspace.load($currentRoute.params.id);
    } else {
      currentWorkspace.clear();
    }
  });

  // Redirect from workspace-detail to the configured default view
  $effect(() => {
    if ($currentRoute.view === 'workspace-detail' && $currentWorkspace) {
      const defaultView = $currentWorkspace.default_view || 'board';
      const wsId = $currentRoute.params?.id;
      // Redirect to configured default view (defaults to 'board')
      if (wsId) {
        if (defaultView === 'overview') {
          navigate(`/workspaces/${wsId}/overview`);
        } else {
          navigate(`/workspaces/${wsId}/${defaultView}`);
        }
      }
    }
  });

  // Single effect to load components based on current route
  $effect(() => {
    const view = effectiveView;

    // Load component for current route view
    if (view) {
      const { key } = resolveRouteConfig(view);
      if (key) {
        loadComponentForRoute(key);
      }
    }

    // Load command palette when opened
    if (showCommandPalette) {
      loadComponentForRoute('command-palette');
    }

    // Load create modal when opened
    if (showCreateModal) {
      loadComponentForRoute('create-modal');
    }
  });



  // Derived workspace dropdown items that automatically updates when store or search changes
  const workspacesDropdownItems = $derived.by(() => {
    const items = [];

    // Add search input at the top
    items.push({
      type: 'search',
      id: 'search',
      placeholder: t('nav.searchWorkspaces'),
      value: workspaceSearchQuery,
      onInput: (value) => {
        workspaceSearchQuery = value;
      }
    });

    // Filter workspaces based on search query
    const search = workspaceSearchQuery?.trim().toLowerCase();
    const filteredWorkspaces = !search
      ? $workspacesStore.regularWorkspaces
      : $workspacesStore.regularWorkspaces.filter(workspace => {
          const nameMatch = workspace.name?.toLowerCase().includes(search);
          const keyMatch = workspace.key?.toLowerCase().includes(search);
          const descriptionMatch = workspace.description?.toLowerCase().includes(search);
          return nameMatch || keyMatch || descriptionMatch;
        });

    // Add workspace items
    if (filteredWorkspaces.length > 0) {
      const workspaceItems = filteredWorkspaces.map(workspace => {
        const hasAvatar = workspace.avatar_url;
        const workspaceIcon = workspaceIconMap[workspace.icon] || workspaceIconMap.Package;

        return {
          id: workspace.id,
          type: 'regular',
          icon: hasAvatar ? null : workspaceIcon,
          iconColor: hasAvatar ? null : workspace.color,
          avatarUrl: hasAvatar ? workspace.avatar_url : null,
          title: workspace.name,
          subtitle: workspace.description,
          onClick: () => navigateToWorkspace(workspace.id)
        };
      });

      items.push(
        { type: 'group', items: workspaceItems },
        { type: 'divider' }
      );
    } else if ($workspacesStore.regularWorkspaces.length > 0 && workspaceSearchQuery) {
      // Show "no results" only if there are workspaces but search didn't match
      items.push(
        { type: 'text', text: t('nav.noWorkspacesMatch') },
        { type: 'divider' }
      );
    } else if ($workspacesStore.regularWorkspaces.length === 0) {
      items.push(
        { type: 'text', text: t('nav.noWorkspacesFound') },
        { type: 'divider' }
      );
    }

    // Add combined manage workspaces action
    items.push({
      id: 'manage',
      type: 'regular',
      icon: Settings,
      title: t('nav.manageWorkspaces'),
      subtitle: t('nav.manageWorkspacesSubtitle'),
      color: 'var(--ds-text-link)',
      class: 'font-medium',
      onClick: () => navigate('/workspaces')
    });

    return items;
  });


  function showCreateDropdown() {
    showCreateModal = true;
    
    // Pre-select current workspace if we're in a workspace context
    const currentWorkspaceId = $currentRoute.params?.id;
    if (currentWorkspaceId && ['workspace-detail', 'workspace-calendar', 'workspace-reviews', 'workspace-settings', 'workspace-settings-general', 'workspace-settings-appearance', 'workspace-settings-categories', 'workspace-settings-members', 'workspace-settings-configuration', 'workspace-settings-danger', 'workspace-board', 'workspace-backlog', 'workspace-list', 'workspace-tree', 'workspace-map', 'item-detail'].includes($currentRoute.view)) {
      // Dispatch event to pre-select the workspace
      setTimeout(() => {
        window.dispatchEvent(new CustomEvent('set-create-workspace', { 
          detail: { workspaceId: parseInt(currentWorkspaceId) } 
        }));
      }, 50);
    }
  }
  
  function navigateToWorkspace(workspaceId) {
    navigate(`/workspaces/${workspaceId}`);
  }

  // Generic lazy loader function for all routes
  async function loadComponentForRoute(view) {
    const loader = componentLoaders[view];
    if (!loader) return;

    // Skip if already loading or loaded
    if (loadingRoutes.has(view) || componentRegistry.has(view)) return;

    // Create new Set with added view (triggers Svelte 5 reactivity)
    loadingRoutes = new Set(loadingRoutes).add(view);

    try {
      const module = await loader();
      // Create new Map with added component (triggers Svelte 5 reactivity)
      componentRegistry = new Map(componentRegistry).set(view, module.default);
    } catch (error) {
      console.error(`Failed to load component for ${view}:`, error);
    } finally {
      // Create new Set without the view (triggers Svelte 5 reactivity)
      const newLoadingRoutes = new Set(loadingRoutes);
      newLoadingRoutes.delete(view);
      loadingRoutes = newLoadingRoutes;
    }
  }

  // Helper to get component for current view (supports matchViews)
  function getComponentForView(view) {
    // Direct match
    if (componentRegistry.has(view)) {
      return componentRegistry.get(view);
    }

    const { key } = resolveRouteConfig(view);
    if (key && componentRegistry.has(key)) {
      return componentRegistry.get(key);
    }

    return null;
  }

  // Helper to check if component is loading
  function isComponentLoading(view) {
    if (loadingRoutes.has(view)) return true;

    const { key } = resolveRouteConfig(view);
    if (key && loadingRoutes.has(key)) {
      return true;
    }

    return false;
  }

  // Get props for current route's component
  function getPropsForRoute(view) {
    const { config } = resolveRouteConfig(view);
    if (!config || !config.getProps) return {};
    return config.getProps($currentRoute);
  }

  // Check if a route requires wrapper styling
  function getWrapperClass(view) {
    const { config } = resolveRouteConfig(view);
    return config?.wrapper || null;
  }
</script>

{#snippet loadingState(message)}
  <div class="flex items-center justify-center h-full">
    <div class="text-center">
      <Spinner class="mx-auto mb-4" />
      <p class="text-gray-600">{message}</p>
    </div>
  </div>
{/snippet}

{#snippet errorState(message, retryFn)}
  <div class="flex items-center justify-center h-full">
    <div class="text-center">
      <p class="text-red-600">{message}</p>
      {#if retryFn}
        <Button variant="primary" onclick={retryFn} class="mt-4">
          {t('nav.retry')}
        </Button>
      {/if}
    </div>
  </div>
{/snippet}

{#snippet lazyLoadedComponent(view)}
  {@const component = getComponentForView(view)}
  {@const loading = isComponentLoading(view)}
  {@const routeEntry = resolveRouteConfig(view)}
  {@const config = routeEntry.config}
  {@const loaderKey = routeEntry.key || view}
  {@const props = getPropsForRoute(view)}

  {#if loading}
    {@render loadingState(config?.loadingMsg || 'Loading...')}
  {:else if component}
    <svelte:component this={component} {...props} />
  {:else}
    {@render errorState(config?.errorMsg || 'Failed to load component', () => loadComponentForRoute(loaderKey))}
  {/if}
{/snippet}

<!-- Main Internal App - Rendered only when user is authenticated -->
<div class="min-h-screen flex flex-col" style="background-color: var(--ds-surface);">
  <!-- Email Verification Banner -->
  <EmailVerificationBanner
    show={showEmailVerificationBanner}
    ondismiss={() => showEmailVerificationBanner = false}
  />

  <!-- Vertical Left Sidebar Navigation -->
  {#if !$uiStore.reviewFullscreen}
    <nav class="w-16 shadow-lg border-r flex flex-col items-center py-4 fixed h-full z-40 themed-nav" style="border-color: var(--ds-border);" aria-label="Main navigation">
      <!-- Logo -->
      <Tooltip content="Windshift" placement="right">
        <a
          href="/"
          class="flex items-center justify-center w-10 h-10 mb-6 hover:opacity-80 transition-opacity cursor-pointer"
        >
          <img src="/cmicon-2.svg" alt="Windshift" class="w-8 h-8 flex-shrink-0" />
        </a>
      </Tooltip>

      <!-- Main Navigation -->
      <div class="flex flex-col items-center space-y-1 flex-1">
        <!-- Workspaces -->
        <Tooltip content={t('nav.workspaces')} placement="right">
          <div>
            <DropdownMenu
              triggerIcon={Grip}
              triggerClass="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button {isWorkspaceRoute($currentRoute.view) ? 'nav-button-selected' : ''} {!$workspacesStore.loaded ? 'opacity-50 cursor-wait' : ''}"
              triggerStyle=""
              items={workspacesDropdownItems}
              maxWidth="max-w-xs"
              showChevron={false}
              placement="right-start"
              iconOnly={true}
            />
          </div>
        </Tooltip>

        <!-- Collections -->
        <Tooltip content={t('nav.collections')} placement="right">
          <a
            href="/collections"
            class="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button {$currentRoute.view === 'collections-list' ? 'nav-button-selected' : ''}"
            aria-current={$currentRoute.view === 'collections-list' ? 'page' : undefined}
          >
            <Library class="w-5 h-5" />
          </a>
        </Tooltip>

        <!-- Time & Projects -->
        <Tooltip content={t('nav.timeAndProjects')} placement="right">
          <a
            href="/time"
            class="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button {$currentRoute.view === 'time' ? 'nav-button-selected' : ''}"
            aria-current={$currentRoute.view === 'time' ? 'page' : undefined}
          >
            <Clock class="w-5 h-5" />
          </a>
        </Tooltip>

        <!-- Milestones -->
        <Tooltip content={t('nav.milestones')} placement="right">
          <a
            href="/milestones"
            class="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button {$currentRoute.view === 'milestones' || $currentRoute.view === 'milestone-detail' ? 'nav-button-selected' : ''}"
            aria-current={$currentRoute.view === 'milestones' ? 'page' : undefined}
          >
            <Milestone class="w-5 h-5" />
          </a>
        </Tooltip>

        <!-- Iterations -->
        <Tooltip content={t('nav.iterations')} placement="right">
          <a
            href="/iterations"
            class="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button {$currentRoute.view === 'iterations' || $currentRoute.view === 'iteration-detail' ? 'nav-button-selected' : ''}"
            aria-current={$currentRoute.view === 'iterations' ? 'page' : undefined}
          >
            <Calendar class="w-5 h-5" />
          </a>
        </Tooltip>

        <!-- Assets -->
        <Tooltip content={t('nav.assets')} placement="right">
          <a
            href="/assets"
            class="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button {$currentRoute.view === 'assets' || $currentRoute.view === 'asset-detail' ? 'nav-button-selected' : ''}"
            aria-current={$currentRoute.view === 'assets' ? 'page' : undefined}
          >
            <Package class="w-5 h-5" />
          </a>
        </Tooltip>

        <!-- Channels -->
        <Tooltip content={t('nav.channels')} placement="right">
          <a
            href="/channels"
            class="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button {$currentRoute.view === 'channels' ? 'nav-button-selected' : ''}"
            aria-current={$currentRoute.view === 'channels' ? 'page' : undefined}
          >
            <LifeBuoy class="w-5 h-5" />
          </a>
        </Tooltip>

        <!-- Customers (conditional based on permission) -->
        {#if $permissionStore.canAccessCustomers}
          <Tooltip content={t('nav.customers')} placement="right">
            <a
              href="/customers"
              class="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button {$currentRoute.view === 'customers' ? 'nav-button-selected' : ''}"
              aria-current={$currentRoute.view === 'customers' ? 'page' : undefined}
            >
              <Users class="w-5 h-5" />
            </a>
          </Tooltip>
        {/if}

        <!-- Top Actions Section - "Notch" style centered positioning -->
        <div class="flex flex-col items-center space-y-2 my-6 py-4">
          <!-- Create button -->
          <Tooltip content="{t('nav.create')} (C)" placement="right">
            <button
              onclick={showCreateDropdown}
              class="w-10 h-10 bg-[var(--ds-interactive)] bg-primary text-white rounded items-center justify-center text-sm font-medium transition cursor-pointer flex gap-2"
            >
              <Plus class="w-5 h-5" />
            </button>
          </Tooltip>

          <!-- Search button -->
          <Tooltip content="{t('nav.search')} ({getShortcutDisplay('global', 'commandPalette')} or Space Space)" placement="right">
            <button
              onclick={() => showCommandPalette = true}
              class="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button"
            >
              <Search class="w-5 h-5" />
            </button>
          </Tooltip>
        </div>
      </div>

      <!-- Bottom Section -->
      <div class="flex flex-col items-center space-y-1 mt-auto">
        <!-- Admin (conditional) -->
        {#if $permissionStore.canAccessAdmin}
          <Tooltip content={t('nav.admin')} placement="right">
            <a
              href="/admin"
              class="w-10 h-10 rounded flex items-center justify-center cursor-pointer nav-button {$currentRoute.view === 'admin' ? 'nav-button-selected' : ''}"
              aria-current={$currentRoute.view === 'admin' ? 'page' : undefined}
            >
              <Settings class="w-5 h-5" />
            </a>
          </Tooltip>
        {/if}

        <!-- Notification Tray -->
        <Tooltip content={t('nav.notifications')} placement="right">
          <div class="w-10 h-10 rounded flex items-center justify-center">
            <NotificationTray />
          </div>
        </Tooltip>

        <!-- User Profile Avatar -->
        <Tooltip content={t('nav.profile')} placement="right">
          <div class="w-10 h-10 rounded flex items-center justify-center">
            <UserAvatar />
          </div>
        </Tooltip>
      </div>
    </nav>
    {/if}

    <!-- Main Content Area with Sidebar Layout -->
    <div class="flex flex-1 {!$uiStore.reviewFullscreen ? 'ml-16' : ''}">
      <!-- Left Sidebar for Workspace/Admin Navigation -->
      {#if !$uiStore.reviewFullscreen && $currentRoute.view !== 'workspaces' && (isWorkspaceRoute($currentRoute.view) || effectiveView === 'personal-task-detail' || testViews.has($currentRoute.view))}
        <WorkspaceNavigation workspaceId={$currentRoute.path?.startsWith('/personal') ? $workspacesStore.personalWorkspace?.id : $currentRoute.params.id} />
      {/if}

      <!-- Main Content -->
      <main class="flex-1">
    {#if true}
      {@const view = effectiveView}
      {@const wrapper = getWrapperClass(view)}
      {@const routeEntry = resolveRouteConfig(view)}
      {@const hasLazyRoute = !!routeEntry.config}

      {#if view === 'workspaces'}
      <Workspaces showAdminHeader={false} />

    {:else if ['workspace-settings', 'workspace-settings-general', 'workspace-settings-appearance', 'workspace-settings-categories', 'workspace-settings-members', 'workspace-settings-configuration', 'workspace-settings-source-control', 'workspace-settings-danger'].includes(view)}
      <div class="p-6" style="background-color: var(--ds-surface);">
        <WorkspaceSettings
          workspaceId={$currentRoute.params.id}
          activeTab={
            view === 'workspace-settings-appearance' ? 'appearance' :
            view === 'workspace-settings-categories' ? 'categories' :
            view === 'workspace-settings-members' ? 'members' :
            view === 'workspace-settings-configuration' ? 'configuration' :
            view === 'workspace-settings-source-control' ? 'source-control' :
            view === 'workspace-settings-danger' ? 'danger' :
            'general'
          }
        />
      </div>

    {:else if view === 'collections-list'}
      <CollectionsList />
    {:else if view === 'collections-edit'}
      <Collections collectionId={$currentRoute.params.id} />

    {:else if view === 'channels'}
      <div style="background-color: var(--ds-surface);">
        <Channels />
      </div>
    {:else if view === 'customers'}
      <Customers />

    {:else if view === 'notifications'}
      <NotificationsPage />
    {:else if view === 'search'}
      <SearchPage />

    {:else if view === 'profile'}
      <div class="p-6" style="background-color: var(--ds-surface);">
        <UserProfile />
      </div>
    {:else if view === 'security'}
      <div class="p-6" style="background-color: var(--ds-surface);">
        <Security />
      </div>

    {:else if view === 'about'}
      <About />
    {:else if view === '404'}
      <div class="p-6" style="background-color: var(--ds-surface);">
        <NotFound />
      </div>

    {:else if view === 'admin'}
      <PermissionGuard requireSystemAdmin={true}>
        {@render lazyLoadedComponent(view)}
        <svelte:fragment slot="fallback" let:requiredPermissionDisplay>
          <UnauthorizedAccess
            message="You need system administrator privileges to access the administration panel."
            requiredPermission={requiredPermissionDisplay}
          />
        </svelte:fragment>
      </PermissionGuard>

    {:else if hasLazyRoute}
      {#if wrapper === 'surface-full'}
        <div style="background-color: var(--ds-surface);">
          {@render lazyLoadedComponent(view)}
        </div>
      {:else if wrapper === 'surface-padded'}
        <div class="p-6" style="background-color: var(--ds-surface);">
          {@render lazyLoadedComponent(view)}
        </div>
      {:else}
        {@render lazyLoadedComponent(view)}
      {/if}

      {:else}
        <Workspaces showAdminHeader={false} />
      {/if}
    {/if}
      </main>
    </div>
    
    <!-- Footer with proper sidebar margin -->
    <footer class="{!$uiStore.reviewFullscreen ? 'ml-16' : ''}">
      <Footer />
    </footer>
</div>


<!-- Command Palette -->
{#if true}
  {@const commandPaletteComponent = getComponentForView('command-palette')}
  {@const commandPaletteLoading = isComponentLoading('command-palette')}

  {#if commandPaletteLoading}
    <div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div class="bg-white rounded p-6">
        <Spinner class="mx-auto mb-4" />
        <p class="text-gray-600">{t('nav.loadingSearch')}</p>
      </div>
    </div>
  {:else if commandPaletteComponent && showCommandPalette}
    <svelte:component
      this={commandPaletteComponent}
      bind:isOpen={showCommandPalette}
      onclose={() => showCommandPalette = false}
      onshow-create-modal={(event) => {
        showCreateModal = true;
        if (event.detail?.type) {
          setTimeout(() => {
            window.dispatchEvent(new CustomEvent('set-create-type', { detail: { type: event.detail.type } }));
          }, 50);
        }
      }}
    />
  {/if}
{/if}

<!-- Create Modal -->
{#if true}
  {@const createModalComponent = getComponentForView('create-modal')}
  {@const createModalLoading = isComponentLoading('create-modal')}

  {#if createModalLoading}
    <div class="fixed inset-0 flex items-center justify-center z-50" style="background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(2px);">
      <div class="bg-white rounded p-6">
        <Spinner class="mx-auto mb-4" />
        <p class="text-gray-600">{t('nav.loadingCreateForm')}</p>
      </div>
    </div>
  {:else if createModalComponent && showCreateModal}
    <svelte:component
      this={createModalComponent}
      bind:isOpen={showCreateModal}
      onclose={() => showCreateModal = false}
    />
  {/if}
{/if}

<!-- Global Confirmation Dialog -->
<GlobalConfirmDialog />

<!-- Floating Timer -->
<FloatingTimer />

<!-- Toast Container -->
<ToastContainer />

<style>
  /* Global CSS custom properties for theming - uses design tokens */
  :global(html) {
    --nav-bg-color: var(--ds-surface-raised);
    --nav-text-color: var(--ds-text);
  }

  /* Themed navigation styles */
  :global(.themed-nav) {
    background-color: var(--nav-bg-color);
    color: var(--nav-text-color);
  }

  /* Ensure child elements inherit the theme colors */
  :global(.themed-nav *) {
    color: inherit;
  }

  /* Override any specific text colors for navigation elements */
  :global(.themed-nav a),
  :global(.themed-nav button) {
    color: var(--nav-text-color);
  }

  /* Theme-aware navigation button classes with enhanced animations */
  :global(.themed-nav .nav-button) {
    color: var(--nav-text-color);
    position: relative;
    transition:
      background-color var(--duration-normal, 200ms) var(--ease-smooth, ease),
      transform var(--duration-fast, 100ms) var(--ease-spring, cubic-bezier(0.34, 1.56, 0.64, 1)),
      box-shadow var(--duration-normal, 200ms) var(--ease-smooth, ease);
  }

  /* Subtle glow effect on hover */
  :global(.themed-nav .nav-button::before) {
    content: '';
    position: absolute;
    inset: -2px;
    border-radius: 8px;
    background: radial-gradient(
      circle at center,
      var(--ds-interactive) 0%,
      transparent 70%
    );
    opacity: 0;
    transition: opacity var(--duration-normal, 200ms) var(--ease-smooth, ease);
    pointer-events: none;
    z-index: -1;
  }

  :global(.themed-nav .nav-button:hover) {
    background-color: var(--ds-background-neutral-hovered);
    transform: scale(1.05);
  }

  :global(.themed-nav .nav-button:hover::before) {
    opacity: 0.12;
  }

  :global(.themed-nav .nav-button:active) {
    transform: scale(0.95);
  }

  :global(.themed-nav .nav-button.nav-button-selected) {
    background-color: var(--ds-surface-pressed);
    box-shadow: var(--ds-glow-nav);
  }

  :global(.themed-nav .nav-button.nav-button-selected:hover) {
    background-color: var(--ds-surface-pressed);
    transform: scale(1.02);
  }

  :global(.themed-nav .nav-button.nav-button-selected::before) {
    opacity: 0.15;
  }

  /* Exception: Primary buttons should keep their original colors and hover behavior */
  :global(.themed-nav .bg-primary) {
    color: var(--ds-text-inverse) !important;
    background-color: var(--ds-interactive) !important;
    transition:
      background-color var(--duration-normal, 200ms) var(--ease-smooth, ease),
      transform var(--duration-fast, 100ms) var(--ease-spring, cubic-bezier(0.34, 1.56, 0.64, 1)),
      box-shadow var(--duration-normal, 200ms) var(--ease-smooth, ease);
  }

  :global(.themed-nav .bg-primary:hover) {
    background-color: var(--ds-interactive-hovered) !important;
    transform: scale(1.05);
    box-shadow: var(--ds-glow-primary);
  }

  :global(.themed-nav .bg-primary:active) {
    transform: scale(0.95);
  }

  /* Reduced motion support */
  @media (prefers-reduced-motion: reduce) {
    :global(.themed-nav .nav-button),
    :global(.themed-nav .bg-primary) {
      transition: none;
    }
    :global(.themed-nav .nav-button:hover),
    :global(.themed-nav .nav-button:active),
    :global(.themed-nav .nav-button.nav-button-selected:hover),
    :global(.themed-nav .bg-primary:hover),
    :global(.themed-nav .bg-primary:active) {
      transform: none;
    }
  }
</style>
