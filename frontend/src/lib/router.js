import { writable } from 'svelte/store';

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
  '/workspaces/:id/settings/danger': 'workspace-settings-danger',
  '/workspaces/:id/actions': 'workspace-actions',
  '/workspaces/:id/items/:itemId': 'item-detail',
  '/workspaces/:id/collections/:collectionId/items/:itemId': 'item-detail',
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
  '/channels': 'hub',
  '/channels/inbox': 'hub-inbox',
  '/admin/channels': 'admin',
  '/admin/channels/category/:categoryId': 'admin',
  '/admin/channels/type/:type': 'admin',
  '/admin/channels/:id': 'admin',
  '/admin/channels/:id/forms': 'admin',
  '/admin/channels/:id/portal': 'admin',
  '/customers': 'customers',
  '/customers/contacts/:contactId': 'customer-contact-detail',
  '/dashboard': 'dashboard',
  '/time': 'time',
  '/time/customers': 'time',
  '/time/projects': 'time',
  '/time/timesheet': 'time',
  '/time/worklogs': 'time',
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
  '/admin': 'admin',
  '/admin/permission-sets/:id': 'admin',
  '/admin/configuration-sets/:id': 'admin',
  '/admin/condition-sets/:id': 'admin',
  '/workspaces/:id/settings/recurrence': 'workspace-settings-recurrence',
  '/admin/:tab': 'admin',
  '/profile': 'profile',
  '/security': 'security',
  '/workflows/:id/design': 'workflow-designer',
  '/board/:slug': 'public-board',
  '/portal/:slug': 'portal',
  '/portal/:slug/verify': 'portal',
  '/forms/:slug': 'public-form',
  '/set-password/:token': 'set-password',
  '/about': 'about',
  '/cli/authorize': 'cli-authorize',
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
  if (path === window.location.pathname + window.location.search) {
    return;
  }
  if (replace) {
    window.history.replaceState({}, '', path);
  } else {
    window.history.pushState({}, '', path);
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
  const path = window.location.pathname + (qs ? `?${qs}` : '');
  if (push) {
    window.history.pushState({}, '', path);
  } else {
    window.history.replaceState({}, '', path);
  }
  updateRoute();
}

// Update current route from URL
function updateRoute() {
  const path = window.location.pathname;
  const search = window.location.search;

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

// Initialize router
export function initRouter() {
  // Update route on page load
  updateRoute();

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
    if (!anchor.href.startsWith(window.location.origin)) return;
    // Respect explicit opt-outs: new-tab targets, downloads, non-http protocols.
    const target = anchor.getAttribute('target');
    if (target && target !== '_self') return;
    if (anchor.hasAttribute('download')) return;
    e.preventDefault();
    const url = new URL(anchor.href);
    navigate(url.pathname + url.search);
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
