import { writable } from 'svelte/store';
import { toExternal, toLogical } from './runtime/contextPath.js';

export const currentRoute = writable(
  /** @type {{path: string, view: string|null, params: Record<string, string>, query: Record<string, string>}} */ ({
    path: '/',
    view: null,
    params: {},
    query: {},
  })
);

// Available routes
const routes = {
  '/': 'homepage',
  '/homepage': 'homepage',
  '/workspaces': 'workspaces',
  '/workspaces/:id': 'workspace-detail',
  '/workspaces/:id/overview': 'workspace-overview',
  '/personal': 'personal-workspace',
  '/personal/calendar': 'workspace-calendar',
  '/personal/reviews': 'workspace-reviews',
  '/personal/plan': 'personal-plan',
  '/personal/items/:itemId': 'item-detail',
  '/workspaces/:id/calendar': 'workspace-calendar',
  '/workspaces/:id/reviews': 'workspace-reviews',
  '/workspaces/:id/settings': 'workspace-settings',
  '/workspaces/:id/settings/general': 'workspace-settings-general',
  '/workspaces/:id/look-and-feel': 'workspace-look-and-feel',
  '/workspaces/:id/settings/categories': 'workspace-settings-categories',
  '/workspaces/:id/settings/members': 'workspace-settings-members',
  '/workspaces/:id/settings/configuration': 'workspace-settings-configuration',
  '/workspaces/:id/settings/source-control': 'workspace-settings-source-control',
  '/workspaces/:id/settings/issue-sync': 'workspace-settings-issue-sync',
  '/workspaces/:id/settings/templates': 'workspace-settings-templates',
  '/workspaces/:id/settings/danger': 'workspace-settings-danger',
  '/workspaces/:id/actions': 'workspace-actions',
  '/workspaces/:id/actions/:actionId': 'workspace-actions',
  '/workspaces/:id/items/:itemId': 'item-detail',
  '/workspaces/:id/collections/:collectionId/items/:itemId': 'item-detail',
  // Stable key routes used by CLI/browser deep links.
  '/workspace/:workspaceKey/item/:itemNumber': 'item-detail',
  '/item/:itemKey': 'item-detail',
  '/zammad/:correlationKey': 'zammad-resolve',
  // Routes without collection (show all workspace items)
  '/workspaces/:id/board': 'workspace-board',
  '/workspaces/:id/board/configure': 'workspace-board-config',
  '/workspaces/:id/backlog': 'workspace-backlog',
  '/workspaces/:id/list': 'workspace-list',
  '/workspaces/:id/tree': 'workspace-tree',
  '/workspaces/:id/map': 'workspace-map',
  '/workspaces/:id/roadmap': 'workspace-roadmap',
  '/workspaces/:id/iterations': 'workspace-iterations',
  '/workspaces/:id/milestones': 'workspace-milestones',
  '/workspaces/:id/analytics': 'workspace-analytics',
  '/workspaces/:id/agents': 'workspace-agents',
  '/workspaces/:id/agents/new': 'workspace-agent-create',
  '/workspaces/:id/agents/:agentId': 'workspace-agent-profile',
  // Knowledge pages (workspace wiki)
  // archived must precede :pageId so the literal segment wins the
  // sequential-iteration match in updateRoute().
  '/workspaces/:id/pages': 'workspace-pages',
  '/workspaces/:id/pages/archived': 'workspace-pages-archived',
  '/workspaces/:id/pages/:pageId': 'workspace-pages',
  // Chrome-free print/PDF view for a single page (opened in a new tab).
  '/workspaces/:id/pages/:pageId/print': 'page-print',
  // Routes with collection ID filtering
  '/workspaces/:id/collections/:collectionId/board': 'workspace-board',
  '/workspaces/:id/collections/:collectionId/board/configure': 'workspace-board-config',
  '/workspaces/:id/collections/:collectionId/backlog': 'workspace-backlog',
  '/workspaces/:id/collections/:collectionId/list': 'workspace-list',
  '/workspaces/:id/collections/:collectionId/tree': 'workspace-tree',
  '/workspaces/:id/collections/:collectionId/map': 'workspace-map',
  '/workspaces/:id/collections/:collectionId/roadmap': 'workspace-roadmap',
  '/workspaces/:id/collections/:collectionId/overview': 'workspace-overview',
  '/workspaces/:id/collections/:collectionId': 'workspace-detail',
  '/collections': 'collections-list',
  '/collections/category/:categoryId': 'collections-list',
  '/collections/workspace': 'collections-list',
  // Global collection views (no workspace)
  '/collections/:id/board': 'collection-board',
  '/collections/:id/board/configure': 'collection-board-config',
  '/collections/:id/backlog': 'collection-backlog',
  '/collections/:id/list': 'collection-list',
  '/collections/:id/tree': 'collection-tree',
  '/collections/:id/map': 'collection-map',
  '/collections/:id/roadmap': 'collection-roadmap',
  '/collections/:id': 'collections-edit',
  '/notifications': 'notifications',
  '/search': 'search',
  '/manage/channels': 'channel-manager',
  '/channels': 'hub',
  '/channels/inbox': 'hub-inbox',
  '/admin/channels': 'admin',
  '/admin/channels/category/:categoryId': 'admin',
  '/admin/channels/type/:type': 'admin',
  '/admin/channels/:id': 'admin',
  '/admin/channels/:id/forms': 'admin',
  '/admin/channels/:id/portal': 'admin',
  '/organizations': 'organizations',
  '/organizations/contacts/:contactId': 'organization-contact-detail',
  '/time': 'time',
  '/time/organizations': 'time',
  '/time/projects': 'time',
  '/time/timesheet': 'time',
  '/time/worklogs': 'time',
  '/time/worklogs/print': 'time-report-print',
  // Workspace-scoped test management routes
  '/workspaces/:id/tests': 'test-cases',
  '/workspaces/:id/tests/cases': 'test-cases',
  '/workspaces/:id/tests/cases/:testId': 'test-case-detail',
  '/workspaces/:id/tests/cases/:testId/steps': 'test-steps',
  '/workspaces/:id/tests/sets': 'test-sets',
  '/workspaces/:id/tests/sets/:setId': 'test-set-detail',
  '/workspaces/:id/tests/templates': 'test-templates',
  '/workspaces/:id/tests/templates/:templateId': 'test-template-detail',
  '/workspaces/:id/tests/runs': 'test-runs',
  '/workspaces/:id/tests/runs/:runId': 'test-run-detail',
  '/workspaces/:id/tests/runs/:runId/print': 'test-run-summary-print',
  '/workspaces/:id/tests/runs/:runId/execute': 'test-execution',
  '/workspaces/:id/tests/reports': 'test-reports',
  '/milestones': 'milestones',
  '/milestones/category/:categoryId': 'milestones',
  '/milestones/:id': 'milestone-detail',
  '/iterations': 'iterations',
  '/iterations/type/:typeId': 'iterations',
  '/iterations/:id': 'iteration-detail',
  '/iterations/:id/dependencies': 'iteration-dependencies',
  '/logbook': 'logbook',
  '/logbook/bucket/:bucketId': 'logbook',
  '/logbook/documents/:documentId': 'logbook-document',
  '/assets': 'assets',
  '/assets/settings': 'asset-settings',
  '/assets/:id': 'asset-detail',
  '/teams': 'teams-list',
  '/teams/:id': 'team-detail',
  '/teams/:id/:section': 'team-detail',
  '/admin': 'admin',
  '/admin/permission-sets/:id': 'admin',
  '/admin/configuration-sets/:id': 'admin',
  '/admin/condition-sets/:id': 'admin',
  '/admin/approval-sets/:id': 'admin',
  '/approvals': 'approvals-inbox',
  '/approvals/mine': 'approvals-inbox',
  '/workspaces/:id/settings/recurrence': 'workspace-settings-recurrence',
  '/admin/:tab': 'admin',
  '/profile': 'profile',
  '/security': 'security',
  '/login': 'homepage',
  '/workflows/:id/design': 'workflow-designer',
  '/board/:slug': 'public-board',
  '/portal/:slug': 'portal',
  '/portal/:slug/verify': 'portal',
  '/portal/:slug/profile': 'portal',
  '/forms/:slug/:formId': 'public-form',
  '/forms/:slug': 'public-form',
  '/set-password/:token': 'set-password',
  '/about': 'about',
  '/licenses': 'licenses',
  // Mobile PWA surface (phone-focused shell, see lib/mobile/). The literal
  // '/m/personal' etc. must precede '/m/items/:id' so they win the
  // sequential-iteration match in updateRoute().
  '/m': 'mobile-my-work',
  '/m/personal': 'mobile-personal',
  '/m/timer': 'mobile-timer',
  '/m/notifications': 'mobile-notifications',
  '/m/search': 'mobile-search',
  '/m/chat': 'mobile-chat',
  '/m/items/:id': 'mobile-item-detail',
  '/api-docs': 'api-docs',
  '/cli/authorize': 'cli-authorize',
  '/oauth/authorize': 'oauth-authorize',
  '/404': '404',
};

// Parse URL parameters
function parseParams(path, route) {
  const params = {};
  const pathParts = path.split('/').filter(Boolean);
  const routeParts = route.split('/').filter(Boolean);

  routeParts.forEach((part, index) => {
    if (part.startsWith(':')) {
      const paramName = part.slice(1);
      params[paramName] = pathParts[index];
    }
  });

  return params;
}

// Parse query string
function parseQuery(search) {
  const query = {};
  if (search) {
    const params = new URLSearchParams(search);
    for (const [key, value] of params) {
      query[key] = value;
    }
  }
  return query;
}

// Navigate to a route
export function navigate(path, { replace = false } = {}) {
  const logicalPath = toLogical(path);
  const externalPath = toExternal(logicalPath);
  if (externalPath === window.location.pathname + window.location.search) {
    return;
  }
  if (replace) {
    window.history.replaceState({}, '', externalPath);
  } else {
    window.history.pushState({}, '', externalPath);
  }
  updateRoute();
}

// Update query params without full navigation. Uses replaceState by default.
// Pass { push: true } to add a history entry (e.g. pagination).
// Values of null/undefined delete the param.
export function updateQueryParams(updates, { push = false } = {}) {
  const params = new URLSearchParams(window.location.search);
  for (const [key, value] of Object.entries(updates)) {
    if (value === null || value === undefined) {
      params.delete(key);
    } else {
      params.set(key, String(value));
    }
  }
  const qs = params.toString();
  const path = toExternal(toLogical(window.location.pathname) + (qs ? `?${qs}` : ''));
  if (push) {
    window.history.pushState({}, '', path);
  } else {
    window.history.replaceState({}, '', path);
  }
  updateRoute();
}

// Update current route from URL
function updateRoute() {
  let path = toLogical(window.location.pathname);
  const search = window.location.search;

  // Agent Studio permanently owns the former Coding Agents destination.
  // Preserve old bookmarks while keeping the legacy editor unreachable.
  const legacyAgentSettings = path.match(/^\/workspaces\/([^/]+)\/settings\/coding-agents$/);
  if (legacyAgentSettings) {
    path = `/workspaces/${legacyAgentSettings[1]}/agents`;
    window.history.replaceState({}, '', toExternal(path + search));
  }

  // Find matching route
  let matchedRoute = routes[path];
  let params = {};

  if (!matchedRoute) {
    // Try to find a parameterized route match
    for (const [route, view] of Object.entries(routes)) {
      if (route.includes(':')) {
        const routeRegex = new RegExp(`^${route.replace(/:[^/]+/g, '([^/]+)')}$`);
        if (routeRegex.test(path)) {
          matchedRoute = view;
          params = parseParams(path, route);
          break;
        }
      }
    }
  }

  // If no route matches, show 404
  if (!matchedRoute) {
    matchedRoute = '404';
  }

  currentRoute.set(
    /** @type {any} */ ({
      path,
      view: matchedRoute,
      params,
      query: parseQuery(search),
    })
  );
}

// Guards against double-binding of the global popstate and click listeners
// when initRouter is called more than once (HMR reloads, test re-inits).
// Without this, every re-init stacked another listener that would never be
// removed and would re-trigger updateRoute / navigate for every event.
let routerInitialized = false;

// Initialize router
export function initRouter() {
  // Update route on page load
  updateRoute();

  if (routerInitialized) return;
  routerInitialized = true;

  // Handle browser back/forward buttons
  window.addEventListener('popstate', updateRoute);

  // Handle link clicks
  document.addEventListener('click', (e) => {
    // Let the browser handle any modified click natively (cmd/ctrl = new tab,
    // shift = new window, alt = download, middle/right button = menu/new tab).
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (e.button !== undefined && e.button !== 0) return;
    if (e.defaultPrevented) return;
    // Use closest() to find the anchor tag even when clicking nested elements
    const anchor = /** @type {HTMLElement} */ (e.target).closest('a');
    if (!anchor?.href) return;
    // Respect explicit opt-outs: new-tab targets, downloads, non-http protocols.
    const target = anchor.getAttribute('target');
    if (target && target !== '_self') return;
    if (anchor.hasAttribute('download')) return;
    const url = new URL(anchor.href);
    if (url.origin !== window.location.origin) return;
    // Native fragment navigation updates the hash and scrolls to the target.
    // pushState cannot reproduce that scroll behavior for same-document links.
    if (
      url.hash &&
      url.pathname === window.location.pathname &&
      url.search === window.location.search
    ) {
      return;
    }
    e.preventDefault();
    navigate(toLogical(url.pathname) + url.search + url.hash);
  });
}

// Global collection view names (no workspace context)
export const GLOBAL_COLLECTION_VIEWS = new Set([
  'collection-board',
  'collection-board-config',
  'collection-backlog',
  'collection-list',
  'collection-tree',
  'collection-map',
  'collection-roadmap',
]);

// Check if a view belongs to the mobile PWA surface. App.svelte uses this to
// render MobileShell instead of the desktop MainApp chrome.
export function isMobileRoute(view) {
  return view?.startsWith('mobile-');
}

// Check if a view is a workspace-related route
export function isWorkspaceRoute(view) {
  return (
    view === 'workspaces' ||
    view === 'personal-workspace' ||
    view === 'personal-plan' ||
    view?.startsWith('workspace-') ||
    view?.startsWith('test-') ||
    view === 'item-detail'
  );
}
