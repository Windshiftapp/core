import { writable } from 'svelte/store';

// Current route store
export const currentRoute = writable({
  path: '/',
  params: {},
  query: {}
});

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
  '/personal/items/:itemId': 'item-detail',
  '/workspaces/:id/calendar': 'workspace-calendar',
  '/workspaces/:id/reviews': 'workspace-reviews',
  '/workspaces/:id/settings': 'workspace-settings',
  '/workspaces/:id/settings/general': 'workspace-settings-general',
  '/workspaces/:id/settings/appearance': 'workspace-settings-appearance',
  '/workspaces/:id/settings/categories': 'workspace-settings-categories',
  '/workspaces/:id/settings/members': 'workspace-settings-members',
  '/workspaces/:id/settings/configuration': 'workspace-settings-configuration',
  '/workspaces/:id/settings/source-control': 'workspace-settings-source-control',
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
  '/workspaces/:id/iterations': 'workspace-iterations',
  '/workspaces/:id/milestones': 'workspace-milestones',
  // Routes with collection ID filtering
  '/workspaces/:id/collections/:collectionId/board': 'workspace-board',
  '/workspaces/:id/collections/:collectionId/board/configure': 'workspace-board-config',
  '/workspaces/:id/collections/:collectionId/backlog': 'workspace-backlog',
  '/workspaces/:id/collections/:collectionId/list': 'workspace-list',
  '/workspaces/:id/collections/:collectionId/tree': 'workspace-tree',
  '/workspaces/:id/collections/:collectionId/map': 'workspace-map',
  '/workspaces/:id/collections/:collectionId': 'workspace-detail',
  '/collections': 'collections-list',
  '/collections/category/:categoryId': 'collections-list',
  '/collections/workspace': 'collections-list',
  '/collections/:id': 'collections-edit',
  '/notifications': 'notifications',
  '/search': 'search',
  '/channels': 'channels',
  '/channels/category/:categoryId': 'channels',
  '/channels/type/:type': 'channels',
  '/channels/:id': 'channels',
  '/customers': 'customers',
  '/dashboard': 'dashboard',
  '/time': 'time',
  '/time/customers': 'time',
  '/time/projects': 'time',
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
  '/assets': 'assets',
  '/assets/:id': 'asset-detail',
  '/admin': 'admin',
  '/admin/permission-sets/:id': 'admin',
  '/admin/configuration-sets/:id': 'admin',
  '/admin/:tab': 'admin',
  '/profile': 'profile',
  '/security': 'security',
  '/workflows/:id/design': 'workflow-designer',
  '/portal/:slug': 'portal',
  '/about': 'about',
  '/404': '404'
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
export function navigate(path) {
  window.history.pushState({}, '', path);
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
        const routeRegex = new RegExp('^' + route.replace(/:[^/]+/g, '([^/]+)') + '$');
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

  currentRoute.set({
    path,
    view: matchedRoute,
    params,
    query: parseQuery(search)
  });
}

// Initialize router
export function initRouter() {
  // Update route on page load
  updateRoute();
  
  // Handle browser back/forward buttons
  window.addEventListener('popstate', updateRoute);
  
  // Handle link clicks
  document.addEventListener('click', (e) => {
    // Use closest() to find the anchor tag even when clicking nested elements
    const anchor = e.target.closest('a');
    if (anchor && anchor.href && anchor.href.startsWith(window.location.origin)) {
      e.preventDefault();
      const url = new URL(anchor.href);
      navigate(url.pathname + url.search);
    }
  });
}

// Get current view for routing
export function getCurrentView() {
  let currentView = 'workspaces';
  currentRoute.subscribe(route => {
    if (route.view && route.view !== '404') {
      currentView = route.view;
    }
  });
  return currentView;
}

// Check if a view is a workspace-related route
export function isWorkspaceRoute(view) {
  return view === 'workspaces' ||
         view === 'personal-workspace' ||
         view?.startsWith('workspace-') ||
         view?.startsWith('test-') ||
         view === 'item-detail';
}
