<script>
  import { createEventDispatcher } from 'svelte';
  import { createCombobox, melt } from '@melt-ui/svelte';
  import { fade, scale } from 'svelte/transition';
  import { backOut } from 'svelte/easing';
  import { navigate, currentRoute } from '../router.js';
  import { api } from '../api.js';
  import { contextCommands } from '../utils/contextCommands.js';
  import { isSystemAdmin } from '../stores';
  import {
    activeTimer,
    canStartTimer,
    canStopTimer,
    timerSyncing,
    stop as stopTimerAction,
    initialize as initializeTimer
  } from '../stores/timerStore.js';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();
  
  export let isOpen = false;
  export let additionalCommands = [];
  
  let workspaces = [];
  let workItems = [];
  let loadingData = false;
  let searchTimeout;
  
  // Load data for quick actions
  async function loadData() {
    try {
      loadingData = true;
      const workspaceData = await api.workspaces.getAll();
      workspaces = workspaceData || [];
    } catch (error) {
      console.error('Failed to load data for command palette:', error);
    } finally {
      loadingData = false;
    }
  }
  
  // Search work items dynamically
  async function searchWorkItems(query) {
    if (!query || query.length < 2) {
      workItems = [];
      return;
    }

    try {
      // Use the search API which handles both key-based search and text search
      const results = await api.search.items({
        query: query.trim(),
        limit: 6
      });
      workItems = results || [];
    } catch (error) {
      console.error('Failed to search work items:', error);
      workItems = [];
    }
  }
  
  // Debounced work item search
  function debouncedSearchWorkItems(query) {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => {
      searchWorkItems(query);
    }, 300);
  }
  
  function getWorkspaceTestBase() {
    const workspaceId = $currentRoute.params?.id;
    return workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
  }

  // Define navigation commands
  function getBaseCommands() {
    const workspaceTestBase = getWorkspaceTestBase();
    return [
      // Navigation
      { id: 'workspaces', label: 'Workspaces', description: 'Manage project workspaces', url: '/workspaces', keywords: ['workspace', 'projects', 'organize', 'w'] },
      { id: 'search', label: 'Search', description: 'Search work items and content', url: '/search', keywords: ['search', 'find', 'items', 's'] },
      { id: 'dashboard', label: 'Dashboard', description: 'View analytics and reports', url: '/dashboard', keywords: ['dashboard', 'analytics', 'reports', 'overview', 'd'] },
      { id: 'milestones', label: 'Milestones', description: 'Track project milestones', url: '/milestones', keywords: ['milestones', 'deadlines', 'targets', 'm'] },
      { id: 'channels', label: 'Channels', description: 'Communication channels and support', url: '/channels', keywords: ['channels', 'communication', 'support', 'help', 'ch'] },
      { id: 'collections', label: 'Collections', description: 'Manage work item collections and views', url: '/collections', keywords: ['collections', 'groups', 'organize', 'views', 'c'] },
      { id: 'admin', label: 'Admin Panel', description: 'System administration and settings', url: '/admin', keywords: ['admin', 'settings', 'configuration', 'system', 'a'], isAdmin: true },
      
      // Test Management Navigation
      { id: 'tests', label: 'Test Management', description: 'Manage test cases, plans, and execution', url: workspaceTestBase, keywords: ['test', 'testing', 'qa', 'quality', 'assurance', 't'] },
      { id: 'test-cases', label: 'Test Cases', description: 'View and manage test cases', url: workspaceTestBase, keywords: ['test', 'cases', 'testing', 'qa', 'tc'] },
      { id: 'test-plans', label: 'Test Plans', description: 'View and manage test plans', url: `${workspaceTestBase}/sets`, keywords: ['test', 'plans', 'suites', 'testing', 'tp'] },
      { id: 'test-runs', label: 'Test Runs', description: 'View and manage test executions', url: `${workspaceTestBase}/runs`, keywords: ['test', 'runs', 'execution', 'results', 'tr'] },
      { id: 'test-reports', label: 'Test Reports', description: 'View test execution reports', url: `${workspaceTestBase}/reports`, keywords: ['test', 'reports', 'results', 'analytics', 'trp'] },
      
      // Time Management Navigation
      { id: 'time-tracking', label: 'Time Tracking', description: 'Log and manage work time entries', url: '/time', keywords: ['time', 'tracking', 'hours', 'work', 'log', 'timesheet', 'tt'] },
      { id: 'time-reports', label: 'Time Reports', description: 'View time tracking reports and analytics', url: '/reports', keywords: ['time', 'reports', 'analytics', 'hours', 'timesheet', 'tr'] },
      { id: 'time-projects', label: 'Time Projects', description: 'Manage time tracking projects', url: '/projects', keywords: ['time', 'projects', 'clients', 'billing', 'tp'] },
      
      // Create Commands
      { id: 'create-work-item', label: 'Create Work Item', description: 'Create a new work item or task', type: 'create', createType: 'work-item', keywords: ['create', 'new', 'work', 'item', 'task', 'issue', 'cw', 'nw'] },
      { id: 'create-workspace', label: 'Create Workspace', description: 'Create a new project workspace', type: 'create', createType: 'workspace', keywords: ['create', 'new', 'workspace', 'project', 'space', 'cws', 'nws'] },
      { id: 'create-milestone', label: 'Create Milestone', description: 'Create a new project milestone', type: 'create', createType: 'milestone', keywords: ['create', 'new', 'milestone', 'target', 'deadline', 'cm', 'nm'] },
      { id: 'create-collection', label: 'Create Collection', description: 'Create a new work item collection', type: 'create', createType: 'collection', keywords: ['create', 'new', 'collection', 'group', 'cc', 'nc'] },
      
      // Test Management Commands
      { id: 'create-test-case', label: 'Create Test Case', description: 'Create a new test case for quality assurance', type: 'create', createType: 'test-case', keywords: ['create', 'new', 'test', 'case', 'testing', 'qa', 'quality', 'ctc', 'ntc'] },
      { id: 'create-test-plan', label: 'Create Test Plan', description: 'Create a new test plan with test cases', type: 'create', createType: 'test-plan', keywords: ['create', 'new', 'test', 'plan', 'testing', 'qa', 'suite', 'ctp', 'ntp'] },
      { id: 'create-test-run', label: 'Create Test Run', description: 'Create a new test run template for execution', type: 'create', createType: 'test-run', keywords: ['create', 'new', 'test', 'run', 'execution', 'template', 'ctr', 'ntr'] },
      
      // Time Management Commands
      { id: 'log-time', label: 'Log Time', description: 'Quickly add a new time entry', type: 'time-action', action: 'log-time', keywords: ['log', 'time', 'entry', 'hours', 'work', 'track', 'lt', 'add'] },
      { id: 'start-timer', label: 'Start Timer', description: 'Start tracking time for current work', type: 'time-action', action: 'start-timer', keywords: ['start', 'timer', 'track', 'time', 'begin', 'st'] },
      { id: 'stop-timer', label: 'Stop Timer', description: 'Stop current time tracking', type: 'time-action', action: 'stop-timer', keywords: ['stop', 'timer', 'end', 'finish', 'complete', 'stp'] },
      
      // Admin Commands (filtered by isSystemAdmin)
      { id: 'admin-custom-fields', label: 'Custom Fields', description: 'Manage custom field definitions', url: '/admin/custom-fields', action: () => navigateToAdminTab('custom-fields'), keywords: ['admin', 'custom', 'fields', 'forms', 'acf'], isAdmin: true },
      { id: 'admin-screens', label: 'Screen Management', description: 'Configure form screens and layouts', url: '/admin/screens', action: () => navigateToAdminTab('screens'), keywords: ['admin', 'screens', 'forms', 'layout', 'as'], isAdmin: true },
      { id: 'admin-hierarchy-levels', label: 'Hierarchy Levels', description: 'Configure work item hierarchy levels', url: '/admin/hierarchy-levels', action: () => navigateToAdminTab('hierarchy-levels'), keywords: ['admin', 'hierarchy', 'levels', 'structure', 'ahl'], isAdmin: true },
      { id: 'admin-item-types', label: 'Item Types', description: 'Manage work item types with icons and colors', url: '/admin/item-types', action: () => navigateToAdminTab('item-types'), keywords: ['admin', 'item', 'types', 'icons', 'colors', 'ait'], isAdmin: true },
      { id: 'admin-priorities', label: 'Priorities', description: 'Configure priority levels with icons and colors', url: '/admin/priorities', action: () => navigateToAdminTab('priorities'), keywords: ['admin', 'priorities', 'priority', 'levels', 'icons', 'colors', 'apr'], isAdmin: true },
      { id: 'admin-config-sets', label: 'Configuration Sets', description: 'Manage configuration sets with workflows and screens', url: '/admin/configuration-sets', action: () => navigateToAdminTab('configuration-sets'), keywords: ['admin', 'configuration', 'config', 'sets', 'acs'], isAdmin: true },
      { id: 'admin-statuses', label: 'Statuses', description: 'Manage individual work item statuses', url: '/admin/statuses?subtab=statuses', action: () => { navigate('/admin/statuses?subtab=statuses'); close(); }, keywords: ['admin', 'status', 'statuses', 'workflow', 'ast'], isAdmin: true },
      { id: 'admin-status-categories', label: 'Status Categories', description: 'Manage status categories and colors', url: '/admin/statuses?subtab=status-categories', action: () => { navigate('/admin/statuses?subtab=status-categories'); close(); }, keywords: ['admin', 'status', 'categories', 'colors', 'asc'], isAdmin: true },
      { id: 'admin-workflows', label: 'Workflow Builder', description: 'Design and manage workflow transitions', url: '/admin/workflows', action: () => navigateToAdminTab('workflows'), keywords: ['admin', 'workflow', 'transitions', 'flow', 'aw'], isAdmin: true },
      { id: 'admin-link-types', label: 'Link Types', description: 'Manage link types between work items', url: '/admin/link-types', action: () => navigateToAdminTab('link-types'), keywords: ['admin', 'link', 'types', 'relationships', 'alt'], isAdmin: true },
      { id: 'admin-scm-providers', label: 'SCM Providers', description: 'Configure GitHub, GitLab, Gitea, and Bitbucket integrations', url: '/admin/scm-providers', action: () => navigateToAdminTab('scm-providers'), keywords: ['admin', 'scm', 'git', 'github', 'gitlab', 'gitea', 'bitbucket', 'source', 'control', 'repository', 'asc'], isAdmin: true },
      { id: 'admin-attachments', label: 'Attachments', description: 'Manage attachment settings and configuration', url: '/admin/attachments', action: () => navigateToAdminTab('attachments'), keywords: ['admin', 'attachments', 'files', 'uploads', 'aa'], isAdmin: true },
      { id: 'admin-modules', label: 'Module Settings', description: 'Enable or disable time tracking and test management modules', url: '/admin/modules', action: () => navigateToAdminTab('modules'), keywords: ['admin', 'modules', 'settings', 'time', 'test', 'tracking', 'management', 'am'], isAdmin: true },
      { id: 'admin-themes', label: 'Theme Settings', description: 'Manage application themes and appearance', url: '/admin/themes', action: () => navigateToAdminTab('themes'), keywords: ['admin', 'themes', 'appearance', 'colors', 'styling', 'at'], isAdmin: true },
      { id: 'admin-users', label: 'User Management', description: 'Manage users, roles, and permissions', url: '/admin/users', action: () => navigateToAdminTab('users'), keywords: ['admin', 'users', 'roles', 'permissions', 'au'], isAdmin: true },
      { id: 'admin-groups', label: 'Group Management', description: 'Manage user groups and memberships', url: '/admin/groups', action: () => navigateToAdminTab('groups'), keywords: ['admin', 'groups', 'teams', 'memberships', 'ag'], isAdmin: true },
      { id: 'admin-permissions', label: 'Permissions', description: 'Manage user permissions and access control', url: '/admin/permissions?subtab=permissions', action: () => { navigate('/admin/permissions?subtab=permissions'); close(); }, keywords: ['admin', 'permissions', 'access', 'control', 'ap'], isAdmin: true },
      { id: 'admin-permission-sets', label: 'Permission Sets', description: 'Manage permission bundles for configuration sets', url: '/admin/permissions?subtab=permission-sets', action: () => { navigate('/admin/permissions?subtab=permission-sets'); close(); }, keywords: ['admin', 'permission', 'sets', 'bundles', 'aps'], isAdmin: true },
      { id: 'admin-workspace-roles', label: 'Workspace Roles', description: 'View workspace roles and their permissions', url: '/admin/workspace-roles', action: () => navigateToAdminTab('workspace-roles'), keywords: ['admin', 'workspace', 'roles', 'permissions', 'awr'], isAdmin: true },
      { id: 'admin-sso', label: 'Single Sign-On', description: 'Configure OIDC identity providers for SSO', url: '/admin/sso', action: () => navigateToAdminTab('sso'), keywords: ['admin', 'sso', 'single', 'sign', 'on', 'oidc', 'identity', 'provider', 'login', 'oauth', 'asso'], isAdmin: true },
      { id: 'admin-workspaces', label: 'Workspaces Admin', description: 'Manage workspaces and settings', url: '/admin/workspaces', action: () => navigateToAdminTab('workspaces'), keywords: ['admin', 'workspaces', 'spaces', 'aws'], isAdmin: true },
      { id: 'admin-notifications', label: 'Notification Settings', description: 'Manage notification configurations', url: '/admin/notification-settings', action: () => navigateToAdminTab('notification-settings'), keywords: ['admin', 'notifications', 'alerts', 'settings', 'an'], isAdmin: true },

      // System Commands
      { id: 'quit-app', label: 'Quit Application', description: 'Gracefully shut down the application server', type: 'system-action', action: 'quit', keywords: ['quit', 'exit', 'shutdown', 'close', 'stop', 'q'] },
    ];
  }
  
  // Quick workspace navigation commands
  function createWorkspaceCommands() {
    const workspaceCommands = [];

    // Add workspace navigation commands
    workspaces.slice(0, 8).forEach(workspace => {
      const url = workspace.is_personal ? '/personal' : `/workspaces/${workspace.id}`;
      workspaceCommands.push({
        id: `goto-workspace-${workspace.id}`,
        label: `Go to ${workspace.name}`,
        description: `Navigate to ${workspace.name} workspace`,
        keywords: ['goto', 'workspace', 'navigate', workspace.name.toLowerCase(), 'gw'],
        type: 'navigation',
        url: url
      });
    });

    return workspaceCommands;
  }

  // Workspace context commands (only show when in a workspace)
  function createWorkspaceContextCommands() {
    const currentWorkspaceId = $currentRoute.params?.id;
    const contextCommands = [];
    
    if (currentWorkspaceId && ['workspace-detail', 'workspace-settings', 'workspace-board', 'workspace-list', 'workspace-tree', 'workspace-map', 'item-detail'].includes($currentRoute.view)) {
      // Find current workspace name
      const currentWorkspace = workspaces.find(w => w.id === parseInt(currentWorkspaceId));
      const workspaceName = currentWorkspace ? currentWorkspace.name : 'Current Workspace';
      
      contextCommands.push({
        id: 'workspace-overview',
        label: `${workspaceName} Overview`,
        description: 'View workspace dashboard with stats and charts',
        keywords: ['overview', 'dashboard', 'workspace', 'stats', 'charts', workspaceName.toLowerCase(), 'o'],
        type: 'workspace-context',
        url: `/workspaces/${currentWorkspaceId}`
      });
    }
    
    return contextCommands;
  }
  
  // Work item commands from search results
  function createWorkItemCommands() {
    return workItems.map(item => {
      const itemKey = `${item.workspace_key || 'WORK'}-${item.workspace_item_number || item.id}`;
      return {
        id: `goto-item-${item.id}`,
        label: `${itemKey}: ${item.title}`,
        description: `${item.workspace_name} • ${item.status}${item.priority ? ` • ${item.priority}` : ''}`,
        keywords: [
          itemKey.toLowerCase(),
          item.title.toLowerCase(),
          item.workspace_name?.toLowerCase(),
          item.workspace_key?.toLowerCase(),
          item.workspace_item_number?.toString(),
          item.id.toString(),
          'item',
          'work',
          'gi'
        ].filter(Boolean),
        type: 'work-item',
        url: `/workspaces/${item.workspace_id}/items/${item.id}`,
        item: item
      };
    });
  }
  
  // Combine all commands with proper priority ordering
  $: commands = [
    // Priority 1: Context-sensitive commands (highest priority)
    ...$contextCommands,

    // Priority 2: Additional commands passed as props
    ...additionalCommands,

    // Priority 3: Workspace context commands
    ...createWorkspaceContextCommands(),

    // Priority 4: Base application commands (filter admin commands based on user role)
    ...getBaseCommands().filter(cmd => !cmd.isAdmin || $isSystemAdmin),

    // Priority 5: Dynamic workspace navigation commands
    ...createWorkspaceCommands(),

    // Priority 6: Work item search results (lowest priority)
    ...(workItems.length > 0 ? createWorkItemCommands() : [])
  ];
  
  
  
  // Fuzzy search function
  function fuzzyMatch(text, search) {
    const searchLower = search.toLowerCase();
    const textLower = text.toLowerCase();
    
    // Exact match gets highest score
    if (textLower.includes(searchLower)) {
      return 100;
    }
    
    // Fuzzy match - check if all search characters appear in order
    let searchIndex = 0;
    let score = 0;
    
    for (let i = 0; i < textLower.length && searchIndex < searchLower.length; i++) {
      if (textLower[i] === searchLower[searchIndex]) {
        score += 10;
        searchIndex++;
      }
    }
    
    // Return score if all characters were found
    return searchIndex === searchLower.length ? score : 0;
  }
  
  function searchCommands(query, commandsList = commands) {
    if (!query.trim()) return commandsList;
    
    const results = commandsList.map(cmd => {
      // Search in label, keywords, and description
      const labelScore = fuzzyMatch(cmd.label, query);
      const keywordScore = cmd.keywords.length > 0 ? Math.max(...cmd.keywords.map(k => fuzzyMatch(k, query))) : 0;
      const descScore = fuzzyMatch(cmd.description, query);
      
      const maxScore = Math.max(labelScore, keywordScore, descScore);
      
      return {
        ...cmd,
        score: maxScore
      };
    }).filter(cmd => cmd.score > 0)
      .sort((a, b) => b.score - a.score);
    
    return results;
  }
  
  // Create combobox without portal
  const {
    elements: { menu, input, option },
    states: { open, inputValue, selected },
    helpers: { isSelected }
  } = createCombobox({
    forceVisible: true,
    portal: null
  });
  
  // Sync with parent component
  $: if (isOpen !== $open) {
    open.set(isOpen);
  }
  
  // Trigger work item search when input changes
  $: if ($inputValue && $inputValue.length >= 2) {
    debouncedSearchWorkItems($inputValue);
  } else if ($inputValue.length < 2) {
    workItems = [];
  }

  // Update filtered commands based on input and commands array, limit to 4 items
  // Make explicit dependency on commands to ensure reactivity when workItems change
  $: filteredCommands = searchCommands($inputValue, commands).slice(0, 4);
  
  let userInteracted = false;
  
  // Auto-select first result when typing (but don't execute)
  $: if (filteredCommands.length > 0 && $inputValue.trim() && !userInteracted) {
    const firstCommand = filteredCommands[0];
    selected.set({ value: firstCommand.id, label: firstCommand.label });
  }
  
  // Handle selection only when user explicitly selects (Enter key or click)
  // Don't auto-execute on selection change
  
  // Navigate to admin tab function
  function navigateToAdminTab(tab) {
    navigate(`/admin/${tab}`);
  }
  
  async function executeCommand(command) {
    if (command.type === 'context-action') {
      // Handle context-sensitive commands
      if (command.action && typeof command.action === 'function') {
        try {
          await command.action();
          close();
        } catch (error) {
          console.error('Failed to execute context command:', error);
          // Don't close on error so user can try again
        }
      } else {
        console.error('Context command missing action function:', command);
      }
      return;
    } else if (command.type === 'create') {
      const workspaceTestBase = getWorkspaceTestBase();
      // Handle test management create commands by navigating and triggering forms
      if (command.createType === 'test-case') {
        navigate(workspaceTestBase);
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('trigger-test-case-form'));
        }, 100);
        return;
      }
      
      if (command.createType === 'test-plan') {
        navigate(`${workspaceTestBase}/sets`);
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('trigger-test-plan-form'));
        }, 100);
        return;
      }
      
      if (command.createType === 'test-run') {
        navigate(`${workspaceTestBase}/runs`);
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('trigger-test-run-form'));
        }, 100);
        return;
      }
      
      // Show create modal with specific type for other create commands
      dispatch('show-create-modal', { type: command.createType });
      
      // Pre-select current workspace if we're creating a work item and we're in a workspace context
      if (command.createType === 'work-item') {
        const currentWorkspaceId = $currentRoute.params?.id;
        if (currentWorkspaceId && ['workspace-detail', 'workspace-settings', 'workspace-board', 'workspace-list', 'workspace-tree', 'workspace-map', 'item-detail'].includes($currentRoute.view)) {
          setTimeout(() => {
            window.dispatchEvent(new CustomEvent('set-create-workspace', { 
              detail: { workspaceId: parseInt(currentWorkspaceId) } 
            }));
          }, 50);
        }
      }
      
      close();
    } else if (command.type === 'time-action') {
      // Handle time management actions
      if (command.action === 'log-time') {
        // Navigate to time tracking page and focus the form
        navigate('/time');
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('focus-time-entry-form'));
        }, 100);
      } else if (command.action === 'start-timer') {
        // Start a timer directly using the timer composable
        if ($canStartTimer) {
          // Show error - need a workspace and project context
          alert(t('dialogs.alerts.startTimerFromItem'));
        } else if ($activeTimer) {
          alert(t('dialogs.alerts.timerAlreadyRunning'));
        }
      } else if (command.action === 'stop-timer') {
        // Stop the timer directly using the timer store
        if ($canStopTimer) {
          try {
            await stopTimerAction();
          } catch (error) {
            console.error('Failed to stop timer:', error);
            alert(t('dialogs.alerts.stopTimerFailed', { error: error.message }));
          }
        } else if (!$activeTimer) {
          alert(t('dialogs.alerts.noTimerRunning'));
        } else if ($timerSyncing) {
          alert(t('dialogs.alerts.timerSyncing'));
        }
      }
      close();
    } else if (command.type === 'system-action') {
      // Handle system commands
      if (command.action === 'quit') {
        // Confirm before quitting
        if (confirm(t('dialogs.confirmations.quitApplication'))) {
          try {
            await api.system.shutdown();
            // Show a message that the server is shutting down
            alert(t('dialogs.alerts.applicationShuttingDown'));
          } catch (error) {
            console.error('Failed to shutdown:', error);
            alert(t('dialogs.alerts.shutdownFailed'));
          }
        }
      }
      close();
    } else if (command.action) {
      // Custom action command
      if (typeof command.action === 'function') {
        command.action();
      }
      close();
    } else {
      // Navigation command
      navigate(command.url);
      close();
    }
  }
  
  function close() {
    isOpen = false;
    open.set(false);
    inputValue.set('');
    dispatch('close');
  }
  
  // Handle keyboard interactions
  function handleKeydown(e) {
    if (e.key === 'Escape') {
      close();
    } else if (e.key === 'Enter' && $selected) {
      e.preventDefault();
      const command = commands.find(cmd => cmd.id === $selected.value);
      if (command) {
        executeCommand(command);
      }
    } else if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
      userInteracted = true;
    }
  }
  
  // Handle click outside
  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) {
      close();
    }
  }
  
  let searchInputRef;
  
  // Load data and focus input when opened
  $: if (isOpen) {
    loadData();
    // Initialize timer to get latest state
    initializeTimer();
  }
  
  // Focus the search input when opened
  $: if (isOpen && searchInputRef) {
    setTimeout(() => {
      searchInputRef.focus();
      searchInputRef.select();
    }, 50);
  }
</script>

<style>
  /* Command palette container animation */
  .command-palette-container {
    animation: scale-in var(--duration-normal, 200ms) var(--ease-spring, cubic-bezier(0.34, 1.56, 0.64, 1)) forwards;
  }

  /* Melt UI combobox styling - enhanced */
  [data-highlighted] {
    background-color: var(--ds-background-selected) !important;
    border-right: 4px solid var(--ds-interactive) !important;
    box-shadow: inset 0 0 0 1px var(--ds-glow-nav, rgba(40, 116, 187, 0.2));
  }

  /* Override Melt UI positioning to ensure proper width */
  [data-melt-combobox-menu] {
    position: static !important;
    width: 100% !important;
    transform: none !important;
    top: auto !important;
    left: auto !important;
  }

  /* Command option transitions - subtle and clean */
  .command-option {
    transition: background-color var(--duration-fast, 100ms) ease;
  }

  .command-option:hover {
    background-color: var(--ds-surface-hovered, var(--ds-surface));
  }


  .kbd {
    background-color: var(--ds-surface);
    color: var(--ds-text-subtle);
    transition: background-color var(--duration-fast, 100ms) ease;
  }

  .kbd:hover {
    background-color: var(--ds-surface-hovered, var(--ds-background-neutral-hovered));
  }

  /* Reduced motion support */
  @media (prefers-reduced-motion: reduce) {
    .command-palette-container {
      animation: none;
    }
  }
</style>

{#if isOpen}
  <!-- Backdrop with enhanced blur -->
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 flex items-start justify-center pt-[20vh] z-[60]"
    style="background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(8px) saturate(120%);"
    tabindex="-1"
    onclick={handleBackdropClick}
    onkeydown={handleKeydown}
    role="dialog"
    aria-modal="true"
  >
    <!-- Input Container with scale entrance -->
    <div
      class="relative w-full max-w-2xl mx-4"
      transition:scale={{ duration: 200, start: 0.95, easing: backOut }}
    >
      <div class="command-palette-container rounded-xl overflow-hidden" style="background-color: var(--ds-glass-bg, var(--ds-surface-raised)); backdrop-filter: blur(12px) saturate(180%); -webkit-backdrop-filter: blur(12px) saturate(180%); border: 1px solid var(--ds-glass-border, var(--ds-border)); box-shadow: var(--shadow-float, 0 20px 50px rgba(0, 0, 0, 0.18));">
        <!-- Search Input -->
        <div class="p-4 border-b" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
          <input
            bind:this={searchInputRef}
            use:melt={$input}
            type="text"
            placeholder={t('commandPalette.searchPlaceholder')}
            class="w-full text-lg border-none outline-none bg-transparent"
            style="color: var(--ds-text);"
          />
        </div>
        
        <!-- Menu positioned directly after input -->
        {#if $open}
          <div
            use:melt={$menu}
            class="w-full"
            style="background-color: var(--ds-surface-raised);"
          >
            <!-- Menu Items -->
            {#if filteredCommands.length === 0}
              <div class="p-4 text-center" style="color: var(--ds-text-subtle);">
                {t('commandPalette.noCommandsFound')}
              </div>
            {:else}
              <div class="max-h-80 overflow-y-auto">
                {#each filteredCommands as command}
                  <div
                    use:melt={$option({ value: command.id, label: command.label })}
                    onclick={() => executeCommand(command)}
                    class="w-full text-left p-4 transition-colors cursor-pointer command-option"
                  >
                    <div class="flex items-center justify-between">
                      <div class="flex-1">
                        <div class="flex items-center gap-2">
                          <div class="font-medium" style="color: var(--ds-text);">{command.label}</div>
                          {#if command._isContextCommand}
                            <span class="px-1.5 py-0.5 text-xs rounded font-medium" style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);">
                              {t('commandPalette.context')}
                            </span>
                          {/if}
                        </div>
                        <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{command.description}</div>
                      </div>
                      <div class="text-xs ml-4 flex-shrink-0" style="color: var(--ds-text-subtlest);">
                        {#if command.keywords && command.keywords.length > 0}
                          {command.keywords.slice(0, 3).join(', ')}{#if command.keywords.length > 3}...{/if}
                        {/if}
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
            
            <!-- Footer - Help section moved to bottom of dropdown -->
            <div class="p-3 border-t" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
              <div class="flex justify-between text-xs mb-2" style="color: var(--ds-text-subtle);">
                <div>
                  <kbd class="kbd px-1 py-0.5 rounded text-xs">↵</kbd> {t('commandPalette.toSelect')}
                  <kbd class="kbd px-1 py-0.5 rounded text-xs ml-2">↑↓</kbd> {t('commandPalette.toNavigate')}
                </div>
                <div>
                  <kbd class="kbd px-1 py-0.5 rounded text-xs">ESC</kbd> {t('commandPalette.toClose')}
                </div>
              </div>
              <div class="flex justify-between items-center">
                <button
                  onclick={() => executeCommand({ url: '/search' })}
                  class="text-xs underline"
                  style="color: var(--ds-interactive);"
                >
                  {t('commandPalette.advancedSearch')}
                </button>
                <div class="text-xs" style="color: var(--ds-text-subtlest);">
                  {t('commandPalette.pressToOpen', { shortcut: '⎵⎵' })}
                </div>
              </div>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}
