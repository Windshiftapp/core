import GanttChart from '@lucide/svelte/icons/gantt-chart';
import Adjustments from '@tabler/icons-svelte-runes/icons/adjustments';
import Zap from '@tabler/icons-svelte-runes/icons/bolt';
import Book from '@tabler/icons-svelte-runes/icons/book';
import Calendar from '@tabler/icons-svelte-runes/icons/calendar';
import BarChart3 from '@tabler/icons-svelte-runes/icons/chart-bar';
import Clock from '@tabler/icons-svelte-runes/icons/clock';
import FileCheck from '@tabler/icons-svelte-runes/icons/file-check';
import FileStack from '@tabler/icons-svelte-runes/icons/file-stack';
import Milestone from '@tabler/icons-svelte-runes/icons/flag';
import GitBranch from '@tabler/icons-svelte-runes/icons/git-branch';
import IconInbox from '@tabler/icons-svelte-runes/icons/inbox';
import Key from '@tabler/icons-svelte-runes/icons/key';
import SquareKanban from '@tabler/icons-svelte-runes/icons/layout-kanban';
import Rows_3 from '@tabler/icons-svelte-runes/icons/layout-rows';
import List from '@tabler/icons-svelte-runes/icons/list';
import ListTree from '@tabler/icons-svelte-runes/icons/list-tree';
import MapPin from '@tabler/icons-svelte-runes/icons/map-pin';
import MessageSquareText from '@tabler/icons-svelte-runes/icons/message-circle';
import Package from '@tabler/icons-svelte-runes/icons/package';
import Play from '@tabler/icons-svelte-runes/icons/player-play';
import Refresh from '@tabler/icons-svelte-runes/icons/refresh';
import Repeat from '@tabler/icons-svelte-runes/icons/repeat';
import SettingsCog from '@tabler/icons-svelte-runes/icons/settings';
import Tags from '@tabler/icons-svelte-runes/icons/tags';
import Trash from '@tabler/icons-svelte-runes/icons/trash';
import TrendingUp from '@tabler/icons-svelte-runes/icons/trending-up';
import AgentIcon from '@tabler/icons-svelte-runes/icons/user-star';
import Users from '@tabler/icons-svelte-runes/icons/users';

/**
 * @typedef {Object} WorkspaceView
 * @property {string} id
 * @property {string} labelKey
 * @property {any}    icon
 * @property {string} [tooltipKey]
 * @property {string} [testId]
 * @property {string[]} [activeViews]  Route view names that highlight this item.
 */

/**
 * Collection-scoped workspace views (visible inside collections too). The
 * queue trails the board views: its route follows the selected collection so
 * collection-scoped queues are reachable, while its visibility toggle stays
 * in the workspace nav set because queue definitions are workspace-owned.
 * @type {WorkspaceView[]}
 */
export const workspaceViewItems = [
  {
    id: 'backlog',
    labelKey: 'workspaceSettings.views.backlog',
    icon: Rows_3,
    testId: 'workspace-nav-backlog',
  },
  {
    id: 'board',
    labelKey: 'workspaceSettings.views.board',
    icon: SquareKanban,
    testId: 'workspace-nav-board',
  },
  {
    id: 'list',
    labelKey: 'workspaceSettings.views.list',
    icon: List,
    testId: 'workspace-nav-list',
  },
  {
    id: 'tree',
    labelKey: 'workspaceSettings.views.tree',
    icon: ListTree,
    testId: 'workspace-nav-tree',
  },
  {
    id: 'map',
    labelKey: 'workspaceSettings.views.map',
    icon: MapPin,
    testId: 'workspace-nav-map',
  },
  {
    id: 'roadmap',
    labelKey: 'collections.roadmap',
    icon: GanttChart,
    testId: 'workspace-nav-roadmap',
  },
  {
    id: 'queue',
    labelKey: 'supportQueue.title',
    tooltipKey: 'supportQueue.description',
    icon: IconInbox,
    testId: 'workspace-nav-queue',
    activeViews: ['workspace-queue'],
  },
];

/**
 * Collection-scoped view ids, the only entries a collection-scope navigation
 * override may toggle; mirrors models.BoardViewIDs on the backend. The queue
 * deliberately stays out: it renders in the views group but only the
 * workspace scope can toggle it.
 * @type {Set<string>}
 */
export const COLLECTION_VIEW_IDS = new Set(['backlog', 'board', 'list', 'tree', 'map', 'roadmap']);

/**
 * Workspace tools which are not scoped to a collection.
 * @type {WorkspaceView[]}
 */
export const workspaceOnlyViews = [
  {
    id: 'agents',
    labelKey: 'users.agents.title',
    tooltipKey: 'users.agents.description',
    icon: AgentIcon,
    testId: 'workspace-nav-agents',
    activeViews: ['workspace-agents', 'workspace-agent-profile', 'workspace-agent-create'],
  },
  {
    id: 'iterations',
    labelKey: 'commandPalette.commands.iterations.label',
    tooltipKey: 'commandPalette.commands.iterations.description',
    icon: Calendar,
  },
  {
    id: 'milestones',
    labelKey: 'commandPalette.commands.milestones.label',
    tooltipKey: 'commandPalette.commands.milestones.description',
    icon: Milestone,
  },
  {
    id: 'analytics',
    labelKey: 'commandPalette.commands.analytics.label',
    tooltipKey: 'commandPalette.commands.analytics.description',
    icon: TrendingUp,
  },
  { id: 'actions', labelKey: 'actions.title', icon: Zap, testId: 'workspace-nav-actions' },
  {
    id: 'pages',
    labelKey: 'pages.treeHeading',
    icon: Book,
    activeViews: ['workspace-pages'],
    testId: 'workspace-nav-pages',
  },
];

/**
 * Test management navigation items, visible only when the test-management
 * module is enabled AND the user has view permission.
 * @type {WorkspaceView[]}
 */
export const testNavigationItems = [
  {
    id: 'test-cases',
    labelKey: 'commandPalette.commands.testCases.label',
    tooltipKey: 'commandPalette.commands.testCases.description',
    icon: FileCheck,
    activeViews: ['test-cases', 'test-case-detail', 'test-steps'],
  },
  {
    id: 'test-sets',
    labelKey: 'commandPalette.commands.testPlans.label',
    tooltipKey: 'commandPalette.commands.testPlans.description',
    icon: Package,
    activeViews: ['test-sets', 'test-set-detail'],
  },
  {
    id: 'test-templates',
    labelKey: 'commandPalette.commands.testTemplates.label',
    tooltipKey: 'commandPalette.commands.testTemplates.description',
    icon: FileStack,
    activeViews: ['test-templates', 'test-template-detail'],
  },
  {
    id: 'test-runs',
    labelKey: 'commandPalette.commands.testRuns.label',
    tooltipKey: 'commandPalette.commands.testRuns.description',
    icon: Play,
    activeViews: ['test-runs', 'test-run-detail', 'test-execution'],
  },
  {
    id: 'test-reports',
    labelKey: 'commandPalette.commands.testReports.label',
    tooltipKey: 'commandPalette.commands.testReports.description',
    icon: BarChart3,
    activeViews: ['test-reports'],
  },
];

/**
 * @typedef {Object} WorkspaceSettingsItem
 * @property {string}  id        Module id (matches the `/settings/<id>` route segment).
 * @property {string}  labelKey  i18n key for the module label (resolve with `t()`).
 * @property {any}     icon      Icon component.
 * @property {string}  view      Route view name that highlights this item.
 * @property {boolean} [danger]  Styled as a destructive action when true.
 * @property {string}  [permission] Workspace permission key required to see the module.
 */

/**
 * Workspace admin (Settings) modules, in display order. Drives the folded
 * admin sidebar (`WorkspaceAdminNav`) and its collapsed icon rail. Routes are
 * `/workspaces/:id/settings/<id>` — use `workspaceSettingsRoute(workspaceId, id)`.
 * @type {WorkspaceSettingsItem[]}
 */
export const workspaceSettingsItems = [
  {
    id: 'general',
    labelKey: 'workspaceSettings.tabs.general',
    icon: SettingsCog,
    view: 'workspace-settings-general',
  },
  {
    id: 'categories',
    labelKey: 'workspaceSettings.tabs.categories',
    icon: Tags,
    view: 'workspace-settings-categories',
  },
  {
    id: 'members',
    labelKey: 'workspaceSettings.tabs.members',
    icon: Users,
    view: 'workspace-settings-members',
  },
  {
    id: 'configuration',
    labelKey: 'workspaceSettings.tabs.configurationSets',
    icon: Adjustments,
    view: 'workspace-settings-configuration',
  },
  {
    id: 'source-control',
    labelKey: 'workspaceSettings.tabs.sourceControl',
    icon: GitBranch,
    view: 'workspace-settings-source-control',
  },
  {
    id: 'issue-sync',
    labelKey: 'workspaceSettings.tabs.issueSync',
    icon: Refresh,
    view: 'workspace-settings-issue-sync',
  },
  {
    id: 'action-credentials',
    labelKey: 'workspaceSettings.tabs.actionCredentials',
    icon: Key,
    view: 'workspace-settings-action-credentials',
    permission: 'action.credential.manage',
  },
  {
    id: 'recurrence',
    labelKey: 'workspaceSettings.tabs.recurrence',
    icon: Repeat,
    view: 'workspace-settings-recurrence',
  },
  {
    id: 'templates',
    labelKey: 'workspaceSettings.tabs.templates',
    icon: FileStack,
    view: 'workspace-settings-templates',
  },
  {
    id: 'canned-responses',
    labelKey: 'workspaceSettings.tabs.cannedResponses',
    icon: MessageSquareText,
    view: 'workspace-settings-canned-responses',
  },
  {
    id: 'service-levels',
    labelKey: 'workspaceSettings.tabs.serviceLevels',
    icon: Clock,
    view: 'workspace-settings-service-levels',
  },
  {
    id: 'danger',
    labelKey: 'workspaceSettings.tabs.removeWorkspace',
    icon: Trash,
    view: 'workspace-settings-danger',
    danger: true,
  },
];

/** Per-module settings page-header subtitle keys (id → i18n key). */
export const workspaceSettingsSubtitleKeys = {
  general: 'workspaceSettings.headers.general',
  categories: 'workspaceSettings.headers.categories',
  members: 'workspaceSettings.headers.members',
  configuration: 'workspaceSettings.headers.configuration',
  'source-control': 'workspaceSettings.headers.sourceControl',
  'coding-agents': 'workspaceSettings.headers.codingAgents',
  'issue-sync': 'workspaceSettings.headers.issueSync',
  'action-credentials': 'workspaceSettings.headers.actionCredentials',
  recurrence: 'workspaceSettings.headers.recurrence',
  templates: 'workspaceSettings.headers.templates',
  'canned-responses': 'workspaceSettings.headers.cannedResponses',
  'service-levels': 'workspaceSettings.headers.serviceLevels',
  danger: 'workspaceSettings.headers.danger',
};

/** All route view names that belong to the workspace admin area. */
export const workspaceSettingsViews = [
  'workspace-settings',
  ...workspaceSettingsItems.map((item) => item.view),
];

/** Settings modules visible to the current user in a workspace. Modules with
 * a `permission` require that workspace permission; the rest only need
 * workspace.admin (the settings area's own gate). */
export function visibleWorkspaceSettingsItems(workspaceId, hasPermission) {
  return workspaceSettingsItems.filter(
    (item) => !item.permission || hasPermission(workspaceId, item.permission)
  );
}

/** Build the route for a settings module. */
export function workspaceSettingsRoute(workspaceId, id) {
  return `/workspaces/${workspaceId}/settings/${id}`;
}

/**
 * View names that have a collection-scoped route. Any other `workspace-*`
 * view (settings, agents, pages, calendar, reviews, …) is workspace-only, so
 * switching to a collection falls back to that collection's default view
 * instead of inventing a collection URL that would 404.
 * @type {Set<string>}
 */
export const COLLECTION_SCOPED_VIEWS = new Set([
  'workspace-overview',
  'workspace-detail',
  'workspace-queue',
  'workspace-board',
  'workspace-backlog',
  'workspace-list',
  'workspace-tree',
  'workspace-map',
  'workspace-roadmap',
  'workspace-board-config',
  'workspace-nav-config',
  'item-detail',
]);

/**
 * Builds the URL for re-scoping the current view to another collection (or to
 * "All items" when collectionId is null). It works from the current route path
 * rather than the view name so compound paths such as `board/configure`,
 * `settings/general`, and detail params survive the switch. Views without a
 * collection-scoped route fall back to the collection's default view.
 *
 * @param {{path?: string, view?: string|null}} route
 * @param {number|string} workspaceId
 * @param {number|string|null} collectionId
 * @param {string} defaultView
 * @returns {string}
 */
export function collectionSwitchUrl(route, workspaceId, collectionId, defaultView = 'backlog') {
  const workspaceBase = `/workspaces/${workspaceId}`;
  const routePath = route?.path || workspaceBase;
  // Strip an existing collection segment so the same view path can be rebuilt.
  const withoutCollection = routePath.replace(
    new RegExp(`^/workspaces/${workspaceId}/collections/[^/]+`),
    workspaceBase
  );
  if (collectionId === null || collectionId === undefined) {
    return withoutCollection;
  }
  if (COLLECTION_SCOPED_VIEWS.has(route?.view)) {
    return `${workspaceBase}/collections/${collectionId}${withoutCollection.slice(workspaceBase.length)}`;
  }
  return `${workspaceBase}/collections/${collectionId}/${defaultView}`;
}
