// Keep every dynamic import literal in one registry so Vite preserves the
// existing route-level chunks while MainApp remains a small composition root.
export const MAIN_APP_COMPONENT_LOADERS = {
  workspaces: () => import('../workspaces/Workspaces.svelte'),
  'workspace-settings': () => import('../workspaces/WorkspaceSettings.svelte'),
  'collections-list': () => import('../features/collections/CollectionsList.svelte'),
  'collections-edit': () => import('../features/collections/Collections.svelte'),
  channels: () => import('../features/channels/Channels.svelte'),
  hub: () => import('../layout/Hub.svelte'),
  organizations: () => import('../workspaces/Customers.svelte'),
  'teams-list': () => import('../teams/TeamsList.svelte'),
  'team-detail': () => import('../teams/TeamDetail.svelte'),
  notifications: () => import('./NotificationsPage.svelte'),
  'approvals-inbox': () => import('./ApprovalsInbox.svelte'),
  search: () => import('./SearchPage.svelte'),
  profile: () => import('./UserProfile.svelte'),
  security: () => import('./Security.svelte'),
  about: () => import('./About.svelte'),
  'api-docs': () => import('./ApiDocs.svelte'),
  'cli-authorize': () => import('./CliAuthorize.svelte'),
  'oauth-authorize': () => import('./OAuthAuthorize.svelte'),
  404: () => import('./NotFound.svelte'),
  admin: () => import('./Admin.svelte'),
  time: () => import('../features/time/Time.svelte'),
  'test-cases': () => import('../features/testing/TestCases.svelte'),
  'test-sets': () => import('../features/testing/TestSets.svelte'),
  'test-templates': () => import('../features/testing/TestTemplates.svelte'),
  'test-runs': () => import('../features/testing/TestRuns.svelte'),
  'test-reports': () => import('../features/testing/TestReports.svelte'),
  'test-case-detail': () => import('../features/testing/TestCaseDetail.svelte'),
  'test-steps': () => import('../features/testing/TestSteps.svelte'),
  'test-execution': () => import('../features/testing/TestExecution.svelte'),
  'test-run-detail': () => import('../features/testing/TestRunDetail.svelte'),
  'test-template-detail': () => import('../features/testing/TestTemplateDetail.svelte'),
  milestones: () => import('../features/milestones/Milestones.svelte'),
  'milestone-detail': () => import('../features/milestones/MilestoneDetail.svelte'),
  iterations: () => import('../features/iterations/Iterations.svelte'),
  'iteration-detail': () => import('../features/iterations/IterationDetail.svelte'),
  'iteration-dependencies': () => import('../features/iterations/IterationDependencies.svelte'),
  assets: () => import('../features/assets/AssetBrowser.svelte'),
  'asset-detail': () => import('../features/assets/AssetBrowser.svelte'),
  'asset-settings': () => import('../features/assets/AssetManager.svelte'),
  'channel-manager': () => import('../features/channels/ManagerChannels.svelte'),
  'workspace-board': () => import('../features/collections/CollectionBoard.svelte'),
  'workspace-board-config': () => import('../settings/BoardConfigurationPage.svelte'),
  'workspace-backlog': () => import('../features/collections/CollectionBacklog.svelte'),
  'workspace-list': () => import('../features/collections/CollectionList.svelte'),
  'workspace-tree': () => import('../features/collections/CollectionTree.svelte'),
  'workspace-map': () => import('../features/collections/CollectionMap.svelte'),
  'workspace-roadmap': () => import('../features/collections/CollectionRoadmap.svelte'),
  'workspace-pages': () => import('../features/pages/PagesView.svelte'),
  'workspace-pages-archived': () => import('../features/pages/ArchivedPagesPage.svelte'),
  'collection-board': () => import('../features/collections/CollectionBoard.svelte'),
  'collection-board-config': () => import('../settings/BoardConfigurationPage.svelte'),
  'collection-backlog': () => import('../features/collections/CollectionBacklog.svelte'),
  'collection-list': () => import('../features/collections/CollectionList.svelte'),
  'collection-tree': () => import('../features/collections/CollectionTree.svelte'),
  'collection-map': () => import('../features/collections/CollectionMap.svelte'),
  'collection-roadmap': () => import('../features/collections/CollectionRoadmap.svelte'),
  'workspace-iterations': () => import('../features/iterations/Iterations.svelte'),
  'workspace-milestones': () => import('../features/milestones/Milestones.svelte'),
  'workspace-actions': () => import('../features/actions/ActionsSettings.svelte'),
  'workspace-analytics': () => import('../features/analytics/WorkspaceAnalytics.svelte'),
  'workspace-agents': () => import('../features/agents/AgentCatalog.svelte'),
  'workspace-agent-create': () => import('../features/agents/AgentCreate.svelte'),
  'workspace-agent-profile': () => import('../features/agents/AgentProfile.svelte'),
  'command-palette': () => import('../layout/CommandPalette.svelte'),
  'create-modal': () => import('../dialogs/CreateModal.svelte'),
  homepage: () => import('./Homepage.svelte'),
  licenses: () => import('./Licenses.svelte'),
  'item-detail': () => import('../features/items/ItemDetail.svelte'),
  'zammad-resolve': () => import('../features/items/ZammadLinkResolver.svelte'),
  'personal-task-detail': () => import('../features/personal/PersonalTaskDetail.svelte'),
  'workspace-detail': () => import('../workspaces/WorkspaceWelcome.svelte'),
  'workspace-overview': () => import('../workspaces/WorkspaceWelcome.svelte'),
  'personal-workspace': () => import('../workspaces/WorkspaceDetail.svelte'),
  'workspace-calendar': () => import('../features/time/WeeklyCalendar.svelte'),
  'workspace-reviews': () => import('../features/personal/PersonalReview.svelte'),
  'workflow-designer': () => import('../features/workflows/WorkflowDesigner.svelte'),
  'configuration-set-detail': () => import('../settings/ConfigurationSetDetail.svelte'),
  'workspace-look-and-feel': () => import('../workspaces/WorkspaceLookAndFeel.svelte'),
  'personal-plan': () => import('../features/personal/PlanMyDay.svelte'),
  logbook: () => import('../features/logbook/Logbook.svelte'),
  'logbook-document': () => import('../features/logbook/DocumentDetail.svelte'),
  'chat-panel': () => import('../features/chat/ChatPanel.svelte'),
  'terminal-panel': () => import('../features/terminal/TerminalPanel.svelte'),
};

const route = (loadingMsg, errorMsg, options = {}) => ({ loadingMsg, errorMsg, ...options });
const workspaceCollectionProps = (currentRoute) => ({
  workspaceId: currentRoute.params.id,
  collectionId: currentRoute.params.collectionId,
});
const globalCollectionProps = (currentRoute) => ({
  workspaceId: null,
  collectionId: currentRoute.params.id,
});
const workspaceProps = (currentRoute) => ({ workspaceId: currentRoute.params.id });

/**
 * @typedef {object} MainAppRouteConfig
 * @property {string} [loadingMsg]
 * @property {string} [errorMsg]
 * @property {string} [wrapper]
 * @property {string} [requirePermission]
 * @property {string} [trigger]
 * @property {string[]} [matchViews]
 * @property {(currentRoute: any, context: any) => Record<string, any>} [getProps]
 */

/** @type {Record<string, MainAppRouteConfig>} */
export const MAIN_APP_ROUTE_CONFIG = {
  workspaces: route('Loading Workspaces...', 'Failed to load Workspaces', {
    getProps: () => ({ showAdminHeader: false }),
  }),
  'workspace-settings': route(
    'Loading Workspace Settings...',
    'Failed to load Workspace Settings',
    {
      wrapper: 'surface-padded',
      matchViews: [
        'workspace-settings-general',
        'workspace-settings-categories',
        'workspace-settings-members',
        'workspace-settings-configuration',
        'workspace-settings-source-control',
        'workspace-settings-coding-agents',
        'workspace-settings-issue-sync',
        'workspace-settings-recurrence',
        'workspace-settings-templates',
        'workspace-settings-danger',
      ],
      getProps: (currentRoute) => ({
        workspaceId: currentRoute.params.id,
        activeTab: WORKSPACE_SETTINGS_TABS[currentRoute.view] || 'general',
      }),
    }
  ),
  'collections-list': route('Loading Collections...', 'Failed to load Collections'),
  'collections-edit': route('Loading Collection...', 'Failed to load Collection', {
    getProps: (currentRoute) => ({ collectionId: currentRoute.params.id }),
  }),
  channels: route('Loading Channels...', 'Failed to load Channels', { wrapper: 'surface-full' }),
  hub: route('Loading Hub...', 'Failed to load Hub', {
    wrapper: 'surface',
    matchViews: ['hub-inbox'],
  }),
  organizations: route('Loading Organizations...', 'Failed to load Organizations', {
    matchViews: ['organization-contact-detail'],
  }),
  'teams-list': route('Loading Teams...', 'Failed to load Teams', { wrapper: 'surface-padded' }),
  'team-detail': route('Loading Team...', 'Failed to load Team', {
    wrapper: 'surface-padded',
    getProps: (currentRoute) => ({
      teamId: currentRoute.params.id,
      section: currentRoute.params.section || 'overview',
    }),
  }),
  notifications: route('Loading Notifications...', 'Failed to load Notifications'),
  'approvals-inbox': route('Loading Approvals...', 'Failed to load Approvals'),
  search: route('Loading Search...', 'Failed to load Search'),
  profile: route('Loading Profile...', 'Failed to load Profile', { wrapper: 'surface-padded' }),
  security: route('Loading Security...', 'Failed to load Security', { wrapper: 'surface-padded' }),
  about: route('Loading About...', 'Failed to load About'),
  'api-docs': route('Loading API Documentation...', 'Failed to load API Documentation'),
  'cli-authorize': route('Loading Authorization...', 'Failed to load Authorization'),
  'oauth-authorize': route('Loading Authorization...', 'Failed to load Authorization'),
  404: route('Loading...', 'Failed to load page', { wrapper: 'surface-padded' }),
  admin: route('Loading Admin Panel...', 'Failed to load Admin Panel', {
    requirePermission: 'systemAdmin',
  }),
  time: route('Loading Time & Projects...', 'Failed to load Time & Projects'),
  'test-cases': route('Loading Test Cases...', 'Failed to load Test Cases', {
    wrapper: 'none',
    getProps: workspaceProps,
  }),
  'test-sets': route('Loading Test Plans...', 'Failed to load Test Plans', {
    wrapper: 'none',
    matchViews: ['test-set-detail'],
    getProps: (currentRoute) => ({
      workspaceId: currentRoute.params.id,
      testSetId: currentRoute.params.setId || null,
    }),
  }),
  'test-templates': route('Loading Test Templates...', 'Failed to load Test Templates', {
    wrapper: 'none',
    getProps: workspaceProps,
  }),
  'test-runs': route('Loading Test Runs...', 'Failed to load Test Runs', {
    wrapper: 'none',
    getProps: workspaceProps,
  }),
  'test-reports': route('Loading Test Reports...', 'Failed to load Test Reports', {
    wrapper: 'none',
    getProps: workspaceProps,
  }),
  'test-case-detail': route('Loading Test Case...', 'Failed to load Test Case', {
    wrapper: 'none',
    getProps: (currentRoute) => ({
      workspaceId: currentRoute.params.id,
      testCaseId: currentRoute.params.testId,
      from: currentRoute.query?.from || null,
    }),
  }),
  'test-steps': route('Loading Test Steps...', 'Failed to load Test Steps', {
    wrapper: 'none',
    getProps: workspaceProps,
  }),
  'test-execution': route('Loading Test Execution...', 'Failed to load Test Execution', {
    wrapper: 'none',
  }),
  'test-run-detail': route('Loading Test Run Details...', 'Failed to load Test Run Details', {
    wrapper: 'none',
  }),
  'test-template-detail': route('Loading Template Details...', 'Failed to load Template Details', {
    wrapper: 'none',
  }),
  milestones: route('Loading Milestones...', 'Failed to load Milestones', {
    wrapper: 'surface-full',
  }),
  'milestone-detail': route('Loading Milestone...', 'Failed to load Milestone', {
    wrapper: 'surface-full',
    getProps: (currentRoute) => ({
      milestoneId: currentRoute.params.id,
      workspaceId: currentRoute.query?.workspaceId || null,
    }),
  }),
  iterations: route('Loading Iterations...', 'Failed to load Iterations', {
    wrapper: 'surface-full',
    getProps: (currentRoute) => ({ typeId: currentRoute.params.typeId }),
  }),
  'iteration-detail': route('Loading Iteration...', 'Failed to load Iteration', {
    wrapper: 'surface-full',
    getProps: (currentRoute) => ({
      iterationId: currentRoute.params.id,
      workspaceId: currentRoute.query?.workspaceId || null,
    }),
  }),
  'iteration-dependencies': route(
    'Loading Dependency Analysis...',
    'Failed to load Dependency Analysis',
    {
      wrapper: 'surface-full',
      getProps: (currentRoute) => ({ iterationId: currentRoute.params.id }),
    }
  ),
  assets: route('Loading Assets...', 'Failed to load Assets', { wrapper: 'surface-full' }),
  'channel-manager': route('Loading Channels...', 'Failed to load Channels', {
    wrapper: 'surface-padded',
  }),
  'asset-detail': route('Loading Asset...', 'Failed to load Asset', {
    wrapper: 'surface-full',
    getProps: (currentRoute) => ({ assetId: currentRoute.params.id }),
  }),
  'asset-settings': route('Loading Asset Settings...', 'Failed to load Asset Settings', {
    wrapper: 'surface-admin',
  }),
  'workspace-board': route('Loading Board View...', 'Failed to load Board View', {
    getProps: workspaceCollectionProps,
  }),
  'workspace-board-config': route(
    'Loading Board Configuration...',
    'Failed to load Board Configuration',
    { getProps: workspaceCollectionProps }
  ),
  'workspace-backlog': route('Loading Backlog View...', 'Failed to load Backlog View', {
    getProps: workspaceCollectionProps,
  }),
  'workspace-list': route('Loading List View...', 'Failed to load List View', {
    getProps: workspaceCollectionProps,
  }),
  'workspace-tree': route('Loading Tree View...', 'Failed to load Tree View', {
    getProps: workspaceCollectionProps,
  }),
  'workspace-map': route('Loading Map View...', 'Failed to load Map View', {
    getProps: workspaceCollectionProps,
  }),
  'workspace-roadmap': route('Loading Roadmap...', 'Failed to load Roadmap', {
    getProps: workspaceCollectionProps,
  }),
  'workspace-pages': route('Loading Pages...', 'Failed to load Pages', {
    wrapper: 'none',
    getProps: (currentRoute) => ({
      workspaceId: Number(currentRoute.params.id),
      pageId: currentRoute.params.pageId ? Number(currentRoute.params.pageId) : null,
    }),
  }),
  'workspace-pages-archived': route('Loading Archived Pages...', 'Failed to load Archived Pages', {
    wrapper: 'none',
    getProps: (currentRoute) => ({ workspaceId: Number(currentRoute.params.id) }),
  }),
  'collection-board': route('Loading Board View...', 'Failed to load Board View', {
    getProps: globalCollectionProps,
  }),
  'collection-board-config': route(
    'Loading Board Configuration...',
    'Failed to load Board Configuration',
    { getProps: globalCollectionProps }
  ),
  'collection-backlog': route('Loading Backlog View...', 'Failed to load Backlog View', {
    getProps: globalCollectionProps,
  }),
  'collection-list': route('Loading List View...', 'Failed to load List View', {
    getProps: globalCollectionProps,
  }),
  'collection-tree': route('Loading Tree View...', 'Failed to load Tree View', {
    getProps: globalCollectionProps,
  }),
  'collection-map': route('Loading Map View...', 'Failed to load Map View', {
    getProps: globalCollectionProps,
  }),
  'collection-roadmap': route('Loading Roadmap...', 'Failed to load Roadmap', {
    getProps: globalCollectionProps,
  }),
  'workspace-iterations': route('Loading Iterations...', 'Failed to load Iterations', {
    wrapper: 'surface-full',
    getProps: workspaceProps,
  }),
  'workspace-milestones': route('Loading Milestones...', 'Failed to load Milestones', {
    wrapper: 'surface-full',
    getProps: workspaceProps,
  }),
  'workspace-actions': route('Loading Actions...', 'Failed to load Actions', {
    wrapper: 'none',
    getProps: (currentRoute) => ({
      workspaceId: currentRoute.params.id,
      actionId: Number(currentRoute.params.actionId) || 0,
    }),
  }),
  'workspace-analytics': route('Loading Analytics...', 'Failed to load Analytics', {
    wrapper: 'surface-full',
    getProps: workspaceProps,
  }),
  'workspace-agents': route('Loading Agents...', 'Failed to load Agents', {
    wrapper: 'surface-full',
    getProps: workspaceProps,
  }),
  'workspace-agent-create': route('Opening Agent Studio...', 'Failed to open Agent Studio', {
    wrapper: 'surface-full',
    getProps: workspaceProps,
  }),
  'workspace-agent-profile': route('Loading Agent...', 'Failed to load Agent', {
    wrapper: 'surface-full',
    getProps: (currentRoute) => ({
      workspaceId: currentRoute.params.id,
      agentId: currentRoute.params.agentId,
      tab: currentRoute.query?.tab,
    }),
  }),
  'command-palette': { trigger: 'showCommandPalette' },
  'create-modal': { trigger: 'showCreateModal' },
  homepage: route('Loading Homepage...', 'Failed to load Homepage', {
    wrapper: 'surface-full',
  }),
  licenses: route('Loading Licenses...', 'Failed to load Licenses', {
    wrapper: 'none',
  }),
  'item-detail': route('Loading Item Details...', 'Failed to load Item Details', {
    getProps: (currentRoute, context) => {
      const personal = currentRoute.path.startsWith('/personal');
      const workspaceParam = currentRoute.params.workspaceKey || currentRoute.params.id;
      const itemParam =
        currentRoute.params.itemKey || currentRoute.params.itemNumber || currentRoute.params.itemId;
      const fullKeyMatch = !personal ? String(itemParam || '').match(/^([^/\s-]+)-(\d+)$/) : null;
      const workspaceParamIsKey = !!workspaceParam && !/^\d+$/.test(String(workspaceParam));
      const keyWorkspace = fullKeyMatch?.[1] || (workspaceParamIsKey ? workspaceParam : null);
      const keyItemNumber = fullKeyMatch?.[2] || (keyWorkspace ? itemParam : null);

      return {
        workspaceId: personal ? context.personalWorkspaceId : keyWorkspace ? null : workspaceParam,
        itemId: keyItemNumber || itemParam,
        workspaceKey: !personal ? keyWorkspace : null,
        itemNumber: !personal ? keyItemNumber : null,
        canonicalizeKeyRoute: !personal && !!keyWorkspace,
        tab: currentRoute.query.tab || 'comments',
        moduleSettings: context.moduleSettings,
      };
    },
  }),
  'zammad-resolve': route('Resolving Zammad link...', 'Failed to resolve Zammad link', {
    wrapper: 'surface-full',
    getProps: (currentRoute) => ({ correlationKey: currentRoute.params.correlationKey }),
  }),
  'personal-task-detail': route('Loading Task...', 'Failed to load Task', {
    getProps: (currentRoute, context) => ({
      workspaceId: currentRoute.path.startsWith('/personal')
        ? context.personalWorkspaceId
        : currentRoute.params.id,
      itemId: currentRoute.params.itemId,
      isModal: false,
    }),
  }),
  'workspace-detail': route('Loading Workspace...', 'Failed to load Workspace', {
    getProps: workspaceCollectionProps,
  }),
  'workspace-overview': route('Loading Workspace...', 'Failed to load Workspace', {
    getProps: workspaceCollectionProps,
  }),
  'personal-workspace': route(
    'Loading Personal Workspace...',
    'Failed to load Personal Workspace',
    {
      getProps: (_currentRoute, context) => ({ workspaceId: context.personalWorkspaceId }),
    }
  ),
  'workspace-calendar': route('Loading Calendar...', 'Failed to load Calendar', {
    wrapper: 'surface-full',
    getProps: (currentRoute, context) => ({
      workspaceId: currentRoute.path.startsWith('/personal')
        ? context.personalWorkspaceId
        : currentRoute.params.id,
    }),
  }),
  'workspace-reviews': route('Loading Reviews...', 'Failed to load Reviews', {
    wrapper: 'surface-full',
    getProps: (currentRoute, context) => ({
      currentUser: context.currentUser,
      workspaceId: currentRoute.path.startsWith('/personal')
        ? context.personalWorkspaceId
        : currentRoute.params.id,
    }),
  }),
  'workflow-designer': route('Loading workflow designer...', 'Failed to load workflow designer'),
  'configuration-set-detail': route(
    'Loading configuration set...',
    'Failed to load configuration set'
  ),
  'workspace-look-and-feel': route('Loading Look and Feel...', 'Failed to load Look and Feel', {
    wrapper: 'none',
    getProps: workspaceProps,
  }),
  'personal-plan': route('Loading Plan My Day...', 'Failed to load Plan My Day', {
    wrapper: 'surface-full',
  }),
  logbook: route('Loading Knowledge Base...', 'Failed to load Knowledge Base', {
    wrapper: 'surface-full',
  }),
  'logbook-document': route('Loading Document...', 'Failed to load Document', {
    wrapper: 'surface-full',
    getProps: (currentRoute) => ({ documentId: currentRoute.params.documentId }),
  }),
};

export const MAIN_APP_TEST_VIEWS = new Set([
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
  'test-set-detail',
]);

export const WORKSPACE_SETTINGS_TABS = {
  'workspace-settings-categories': 'categories',
  'workspace-settings-members': 'members',
  'workspace-settings-configuration': 'configuration',
  'workspace-settings-source-control': 'source-control',
  'workspace-settings-coding-agents': 'coding-agents',
  'workspace-settings-issue-sync': 'issue-sync',
  'workspace-settings-recurrence': 'recurrence',
  'workspace-settings-templates': 'templates',
  'workspace-settings-danger': 'danger',
};

export const WORKSPACE_SETTINGS_VIEWS = new Set([
  'workspace-settings',
  'workspace-settings-general',
  ...Object.keys(WORKSPACE_SETTINGS_TABS),
]);

export const CREATE_MODAL_WORKSPACE_VIEWS = new Set([
  'workspace-detail',
  'workspace-calendar',
  'workspace-reviews',
  ...[...WORKSPACE_SETTINGS_VIEWS].filter((view) => view !== 'workspace-settings-recurrence'),
  'workspace-look-and-feel',
  'workspace-board',
  'workspace-backlog',
  'workspace-list',
  'workspace-tree',
  'workspace-map',
  'workspace-roadmap',
  'workspace-actions',
  'item-detail',
]);

export function resolveMainAppRoute(view) {
  if (!view) return { key: null, config: null };
  if (MAIN_APP_ROUTE_CONFIG[view]) {
    return { key: view, config: MAIN_APP_ROUTE_CONFIG[view] };
  }

  for (const [key, config] of Object.entries(MAIN_APP_ROUTE_CONFIG)) {
    if (config.matchViews?.includes(view)) return { key, config };
  }

  return { key: null, config: null };
}

export function resolveEffectiveMainAppView(currentRoute, personalWorkspaceId) {
  if (currentRoute.view !== 'item-detail') return currentRoute.view;

  const workspaceId = currentRoute.path?.startsWith('/personal')
    ? personalWorkspaceId
    : Number.parseInt(currentRoute.params?.id, 10);

  return personalWorkspaceId && workspaceId === personalWorkspaceId
    ? 'personal-task-detail'
    : currentRoute.view;
}

export function getMainAppRouteProps(view, currentRoute, context = {}) {
  const { config } = resolveMainAppRoute(view);
  return config?.getProps?.(currentRoute, context) || {};
}

export function getMainAppLazyState(lazyComponents, view) {
  const { key, config } = resolveMainAppRoute(view);
  const loaderKey = key || view;

  return {
    component: lazyComponents.getComponent(view) || (key ? lazyComponents.getComponent(key) : null),
    loading: lazyComponents.isLoading(view) || Boolean(key && lazyComponents.isLoading(key)),
    error: lazyComponents.getError(view) || (key ? lazyComponents.getError(key) : null),
    config,
    loaderKey,
  };
}
