<script>
  import { createEventDispatcher } from 'svelte';
  import { createCombobox, melt } from '@melt-ui/svelte';
  import { fade, scale } from 'svelte/transition';
  import { backOut } from 'svelte/easing';
  import { navigate, currentRoute } from '../router.js';
  import { api } from '../api.js';
  import { contextCommands } from '../utils/contextCommands.js';
  import { isSystemAdmin } from '../stores';
  import { timerStore } from '../stores/timerStore.svelte.js';
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
    const workspaceId = $currentRoute.params?.id;
    const workspaceTestBase = getWorkspaceTestBase();

    const commands = [
      // Navigation
      { id: 'workspaces', label: t('commandPalette.commands.workspaces.label'), description: t('commandPalette.commands.workspaces.description'), url: '/workspaces', keywords: ['workspace', 'projects', 'organize', 'w'] },
      { id: 'search', label: t('commandPalette.commands.search.label'), description: t('commandPalette.commands.search.description'), url: '/search', keywords: ['search', 'find', 'items', 's'] },
      { id: 'dashboard', label: t('commandPalette.commands.dashboard.label'), description: t('commandPalette.commands.dashboard.description'), url: '/dashboard', keywords: ['dashboard', 'analytics', 'reports', 'overview', 'd'] },
      { id: 'milestones', label: t('commandPalette.commands.milestones.label'), description: t('commandPalette.commands.milestones.description'), url: '/milestones', keywords: ['milestones', 'deadlines', 'targets', 'm'] },
      { id: 'iterations', label: t('commandPalette.commands.iterations.label'), description: t('commandPalette.commands.iterations.description'), url: '/iterations', keywords: ['iterations', 'sprints', 'cycles', 'planning', 'i'] },
      { id: 'channels', label: t('commandPalette.commands.channels.label'), description: t('commandPalette.commands.channels.description'), url: '/channels', keywords: ['channels', 'communication', 'support', 'help', 'ch'] },
      { id: 'collections', label: t('commandPalette.commands.collections.label'), description: t('commandPalette.commands.collections.description'), url: '/collections', keywords: ['collections', 'groups', 'organize', 'views', 'c'] },
      { id: 'admin', label: t('commandPalette.commands.adminPanel.label'), description: t('commandPalette.commands.adminPanel.description'), url: '/admin', keywords: ['admin', 'settings', 'configuration', 'system', 'a'], isAdmin: true },

      // Time Management Navigation
      { id: 'time-tracking', label: t('commandPalette.commands.timeTracking.label'), description: t('commandPalette.commands.timeTracking.description'), url: '/time', keywords: ['time', 'tracking', 'hours', 'work', 'log', 'timesheet', 'tt'] },
      { id: 'time-reports', label: t('commandPalette.commands.timeReports.label'), description: t('commandPalette.commands.timeReports.description'), url: '/reports', keywords: ['time', 'reports', 'analytics', 'hours', 'timesheet', 'tr'] },
      { id: 'time-projects', label: t('commandPalette.commands.timeProjects.label'), description: t('commandPalette.commands.timeProjects.description'), url: '/projects', keywords: ['time', 'projects', 'clients', 'billing', 'tp'] },

      // Create Commands
      { id: 'create-work-item', label: t('commandPalette.commands.createWorkItem.label'), description: t('commandPalette.commands.createWorkItem.description'), type: 'create', createType: 'work-item', keywords: ['create', 'new', 'work', 'item', 'task', 'issue', 'cw', 'nw'] },
      { id: 'create-workspace', label: t('commandPalette.commands.createWorkspace.label'), description: t('commandPalette.commands.createWorkspace.description'), type: 'create', createType: 'workspace', keywords: ['create', 'new', 'workspace', 'project', 'space', 'cws', 'nws'] },
      { id: 'create-milestone', label: t('commandPalette.commands.createMilestone.label'), description: t('commandPalette.commands.createMilestone.description'), type: 'create', createType: 'milestone', keywords: ['create', 'new', 'milestone', 'target', 'deadline', 'cm', 'nm'] },
      { id: 'create-collection', label: t('commandPalette.commands.createCollection.label'), description: t('commandPalette.commands.createCollection.description'), type: 'create', createType: 'collection', keywords: ['create', 'new', 'collection', 'group', 'cc', 'nc'] },

      // Time Management Commands
      { id: 'log-time', label: t('commandPalette.commands.logTime.label'), description: t('commandPalette.commands.logTime.description'), type: 'time-action', action: 'log-time', keywords: ['log', 'time', 'entry', 'hours', 'work', 'track', 'lt', 'add'] },
      { id: 'start-timer', label: t('commandPalette.commands.startTimer.label'), description: t('commandPalette.commands.startTimer.description'), type: 'time-action', action: 'start-timer', keywords: ['start', 'timer', 'track', 'time', 'begin', 'st'] },
      { id: 'stop-timer', label: t('commandPalette.commands.stopTimer.label'), description: t('commandPalette.commands.stopTimer.description'), type: 'time-action', action: 'stop-timer', keywords: ['stop', 'timer', 'end', 'finish', 'complete', 'stp'] },

      // Admin Commands (filtered by isSystemAdmin)
      { id: 'admin-custom-fields', label: t('commandPalette.commands.adminCustomFields.label'), description: t('commandPalette.commands.adminCustomFields.description'), url: '/admin/custom-fields', action: () => navigateToAdminTab('custom-fields'), keywords: ['admin', 'custom', 'fields', 'forms', 'acf'], isAdmin: true },
      { id: 'admin-screens', label: t('commandPalette.commands.adminScreens.label'), description: t('commandPalette.commands.adminScreens.description'), url: '/admin/screens', action: () => navigateToAdminTab('screens'), keywords: ['admin', 'screens', 'forms', 'layout', 'as'], isAdmin: true },
      { id: 'admin-hierarchy-levels', label: t('commandPalette.commands.adminHierarchyLevels.label'), description: t('commandPalette.commands.adminHierarchyLevels.description'), url: '/admin/hierarchy-levels', action: () => navigateToAdminTab('hierarchy-levels'), keywords: ['admin', 'hierarchy', 'levels', 'structure', 'ahl'], isAdmin: true },
      { id: 'admin-item-types', label: t('commandPalette.commands.adminItemTypes.label'), description: t('commandPalette.commands.adminItemTypes.description'), url: '/admin/item-types', action: () => navigateToAdminTab('item-types'), keywords: ['admin', 'item', 'types', 'icons', 'colors', 'ait'], isAdmin: true },
      { id: 'admin-priorities', label: t('commandPalette.commands.adminPriorities.label'), description: t('commandPalette.commands.adminPriorities.description'), url: '/admin/priorities', action: () => navigateToAdminTab('priorities'), keywords: ['admin', 'priorities', 'priority', 'levels', 'icons', 'colors', 'apr'], isAdmin: true },
      { id: 'admin-config-sets', label: t('commandPalette.commands.adminConfigSets.label'), description: t('commandPalette.commands.adminConfigSets.description'), url: '/admin/configuration-sets', action: () => navigateToAdminTab('configuration-sets'), keywords: ['admin', 'configuration', 'config', 'sets', 'acs'], isAdmin: true },
      { id: 'admin-statuses', label: t('commandPalette.commands.adminStatuses.label'), description: t('commandPalette.commands.adminStatuses.description'), url: '/admin/statuses?subtab=statuses', action: () => { navigate('/admin/statuses?subtab=statuses'); close(); }, keywords: ['admin', 'status', 'statuses', 'workflow', 'ast'], isAdmin: true },
      { id: 'admin-status-categories', label: t('commandPalette.commands.adminStatusCategories.label'), description: t('commandPalette.commands.adminStatusCategories.description'), url: '/admin/statuses?subtab=status-categories', action: () => { navigate('/admin/statuses?subtab=status-categories'); close(); }, keywords: ['admin', 'status', 'categories', 'colors', 'asc'], isAdmin: true },
      { id: 'admin-workflows', label: t('commandPalette.commands.adminWorkflows.label'), description: t('commandPalette.commands.adminWorkflows.description'), url: '/admin/workflows', action: () => navigateToAdminTab('workflows'), keywords: ['admin', 'workflow', 'transitions', 'flow', 'aw'], isAdmin: true },
      { id: 'admin-link-types', label: t('commandPalette.commands.adminLinkTypes.label'), description: t('commandPalette.commands.adminLinkTypes.description'), url: '/admin/link-types', action: () => navigateToAdminTab('link-types'), keywords: ['admin', 'link', 'types', 'relationships', 'alt'], isAdmin: true },
      { id: 'admin-scm-providers', label: t('commandPalette.commands.adminScmProviders.label'), description: t('commandPalette.commands.adminScmProviders.description'), url: '/admin/scm-providers', action: () => navigateToAdminTab('scm-providers'), keywords: ['admin', 'scm', 'git', 'github', 'gitlab', 'gitea', 'bitbucket', 'source', 'control', 'repository', 'asc'], isAdmin: true },
      { id: 'admin-attachments', label: t('commandPalette.commands.adminAttachments.label'), description: t('commandPalette.commands.adminAttachments.description'), url: '/admin/attachments', action: () => navigateToAdminTab('attachments'), keywords: ['admin', 'attachments', 'files', 'uploads', 'aa'], isAdmin: true },
      { id: 'admin-modules', label: t('commandPalette.commands.adminModules.label'), description: t('commandPalette.commands.adminModules.description'), url: '/admin/modules', action: () => navigateToAdminTab('modules'), keywords: ['admin', 'modules', 'settings', 'time', 'test', 'tracking', 'management', 'am'], isAdmin: true },
      { id: 'admin-themes', label: t('commandPalette.commands.adminThemes.label'), description: t('commandPalette.commands.adminThemes.description'), url: '/admin/themes', action: () => navigateToAdminTab('themes'), keywords: ['admin', 'themes', 'appearance', 'colors', 'styling', 'at'], isAdmin: true },
      { id: 'admin-users', label: t('commandPalette.commands.adminUsers.label'), description: t('commandPalette.commands.adminUsers.description'), url: '/admin/users', action: () => navigateToAdminTab('users'), keywords: ['admin', 'users', 'roles', 'permissions', 'au'], isAdmin: true },
      { id: 'admin-groups', label: t('commandPalette.commands.adminGroups.label'), description: t('commandPalette.commands.adminGroups.description'), url: '/admin/groups', action: () => navigateToAdminTab('groups'), keywords: ['admin', 'groups', 'teams', 'memberships', 'ag'], isAdmin: true },
      { id: 'admin-permissions', label: t('commandPalette.commands.adminPermissions.label'), description: t('commandPalette.commands.adminPermissions.description'), url: '/admin/permissions?subtab=permissions', action: () => { navigate('/admin/permissions?subtab=permissions'); close(); }, keywords: ['admin', 'permissions', 'access', 'control', 'ap'], isAdmin: true },
      { id: 'admin-permission-sets', label: t('commandPalette.commands.adminPermissionSets.label'), description: t('commandPalette.commands.adminPermissionSets.description'), url: '/admin/permissions?subtab=permission-sets', action: () => { navigate('/admin/permissions?subtab=permission-sets'); close(); }, keywords: ['admin', 'permission', 'sets', 'bundles', 'aps'], isAdmin: true },
      { id: 'admin-workspace-roles', label: t('commandPalette.commands.adminWorkspaceRoles.label'), description: t('commandPalette.commands.adminWorkspaceRoles.description'), url: '/admin/workspace-roles', action: () => navigateToAdminTab('workspace-roles'), keywords: ['admin', 'workspace', 'roles', 'permissions', 'awr'], isAdmin: true },
      { id: 'admin-sso', label: t('commandPalette.commands.adminSso.label'), description: t('commandPalette.commands.adminSso.description'), url: '/admin/sso', action: () => navigateToAdminTab('sso'), keywords: ['admin', 'sso', 'single', 'sign', 'on', 'oidc', 'identity', 'provider', 'login', 'oauth', 'asso'], isAdmin: true },
      { id: 'admin-security', label: t('commandPalette.commands.adminSecurity.label'), description: t('commandPalette.commands.adminSecurity.description'), url: '/admin/security', action: () => navigateToAdminTab('security'), keywords: ['admin', 'security', 'calendar', 'feeds', 'plugins', 'asec'], isAdmin: true },
      { id: 'admin-system-import', label: t('commandPalette.commands.adminSystemImport.label'), description: t('commandPalette.commands.adminSystemImport.description'), url: '/admin/system-import', action: () => navigateToAdminTab('system-import'), keywords: ['admin', 'import', 'system', 'migration', 'data', 'asi'], isAdmin: true },
      { id: 'admin-assets', label: t('commandPalette.commands.adminAssets.label'), description: t('commandPalette.commands.adminAssets.description'), url: '/admin/assets', action: () => navigateToAdminTab('assets'), keywords: ['admin', 'assets', 'inventory', 'items', 'aas'], isAdmin: true },
      { id: 'admin-workspaces', label: t('commandPalette.commands.adminWorkspaces.label'), description: t('commandPalette.commands.adminWorkspaces.description'), url: '/admin/workspaces', action: () => navigateToAdminTab('workspaces'), keywords: ['admin', 'workspaces', 'spaces', 'aws'], isAdmin: true },
      { id: 'admin-notifications', label: t('commandPalette.commands.adminNotifications.label'), description: t('commandPalette.commands.adminNotifications.description'), url: '/admin/notification-settings', action: () => navigateToAdminTab('notification-settings'), keywords: ['admin', 'notifications', 'alerts', 'settings', 'an'], isAdmin: true },

      // System Commands
      { id: 'quit-app', label: t('commandPalette.commands.quitApp.label'), description: t('commandPalette.commands.quitApp.description'), type: 'system-action', action: 'quit', keywords: ['quit', 'exit', 'shutdown', 'close', 'stop', 'q'] },
    ];

    // Only include test management when in workspace context
    if (workspaceId) {
      commands.push(
        // Test Management Navigation (labels aligned with workspace navigation sidebar)
        { id: 'tests', label: t('commandPalette.commands.tests.label'), description: t('commandPalette.commands.tests.description'), url: workspaceTestBase, keywords: ['test', 'testing', 'qa', 'quality', 'assurance', 't'] },
        { id: 'test-cases', label: t('commandPalette.commands.testCases.label'), description: t('commandPalette.commands.testCases.description'), url: workspaceTestBase, keywords: ['test', 'cases', 'testing', 'qa', 'tc'] },
        { id: 'test-plans', label: t('commandPalette.commands.testPlans.label'), description: t('commandPalette.commands.testPlans.description'), url: `${workspaceTestBase}/sets`, keywords: ['test', 'plans', 'suites', 'testing', 'tp'] },
        { id: 'test-templates', label: t('commandPalette.commands.testTemplates.label'), description: t('commandPalette.commands.testTemplates.description'), url: `${workspaceTestBase}/templates`, keywords: ['test', 'templates', 'shared', 'steps', 'reusable', 'tt'] },
        { id: 'test-runs', label: t('commandPalette.commands.testRuns.label'), description: t('commandPalette.commands.testRuns.description'), url: `${workspaceTestBase}/runs`, keywords: ['test', 'runs', 'execution', 'results', 'tr'] },
        { id: 'test-reports', label: t('commandPalette.commands.testReports.label'), description: t('commandPalette.commands.testReports.description'), url: `${workspaceTestBase}/reports`, keywords: ['test', 'reports', 'results', 'analytics', 'trp'] },
        // Test Management Create Commands
        { id: 'create-test-case', label: t('commandPalette.commands.createTestCase.label'), description: t('commandPalette.commands.createTestCase.description'), type: 'create', createType: 'test-case', keywords: ['create', 'new', 'test', 'case', 'testing', 'qa', 'quality', 'ctc', 'ntc'] },
        { id: 'create-test-plan', label: t('commandPalette.commands.createTestPlan.label'), description: t('commandPalette.commands.createTestPlan.description'), type: 'create', createType: 'test-plan', keywords: ['create', 'new', 'test', 'plan', 'testing', 'qa', 'suite', 'ctp', 'ntp'] },
        { id: 'create-test-run', label: t('commandPalette.commands.createTestRun.label'), description: t('commandPalette.commands.createTestRun.description'), type: 'create', createType: 'test-run', keywords: ['create', 'new', 'test', 'run', 'execution', 'template', 'ctr', 'ntr'] },
      );
    }

    return commands;
  }
  
  // Quick workspace navigation commands
  function createWorkspaceCommands() {
    const workspaceCommands = [];

    // Add workspace navigation commands
    workspaces.slice(0, 8).forEach(workspace => {
      const url = workspace.is_personal ? '/personal' : `/workspaces/${workspace.id}`;
      workspaceCommands.push({
        id: `goto-workspace-${workspace.id}`,
        label: t('commandPalette.commands.goToWorkspace.label', { name: workspace.name }),
        description: t('commandPalette.commands.goToWorkspace.description', { name: workspace.name }),
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
        label: t('commandPalette.commands.workspaceOverview.label', { name: workspaceName }),
        description: t('commandPalette.commands.workspaceOverview.description'),
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
        close();
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('trigger-test-case-form'));
        }, 100);
        return;
      }

      if (command.createType === 'test-plan') {
        navigate(`${workspaceTestBase}/sets`);
        close();
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('trigger-test-plan-form'));
        }, 100);
        return;
      }

      if (command.createType === 'test-run') {
        navigate(`${workspaceTestBase}/runs`);
        close();
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('trigger-test-run-form'));
        }, 100);
        return;
      }
      
      // Show create modal with specific type for other create commands
      // Use window event for consistency with other components and reliability with dynamic loading
      const currentWorkspaceId = $currentRoute.params?.id;
      window.dispatchEvent(new CustomEvent('show-create-modal', {
        detail: {
          type: command.createType,
          workspaceId: currentWorkspaceId ? parseInt(currentWorkspaceId) : undefined
        }
      }));

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
        if (timerStore.canStart) {
          // Show error - need a workspace and project context
          alert(t('dialogs.alerts.startTimerFromItem'));
        } else if (timerStore.activeTimer) {
          alert(t('dialogs.alerts.timerAlreadyRunning'));
        }
      } else if (command.action === 'stop-timer') {
        // Stop the timer directly using the timer store
        if (timerStore.canStop) {
          try {
            await timerStore.stop();
          } catch (error) {
            console.error('Failed to stop timer:', error);
            alert(t('dialogs.alerts.stopTimerFailed', { error: error.message }));
          }
        } else if (!timerStore.activeTimer) {
          alert(t('dialogs.alerts.noTimerRunning'));
        } else if (timerStore.syncing) {
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
    timerStore.initialize();
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

  /* Melt UI combobox styling - highlighted/selected state */
  [data-highlighted] {
    background-color: var(--ds-background-neutral-hovered) !important;
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
