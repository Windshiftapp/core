import { GLOBAL_COLLECTION_VIEWS } from '../router.js';
import { MAIN_APP_TEST_VIEWS } from './mainAppRoutes.js';

function isPersonalWorkspaceView(view) {
  return (
    view?.startsWith('workspace-') ||
    view === 'personal-workspace' ||
    view === 'personal-plan' ||
    view === 'item-detail'
  );
}

function isRegularWorkspaceView(view) {
  return (
    view?.startsWith('workspace-') ||
    view === 'workspace' ||
    view === 'item-detail' ||
    view === 'item' ||
    MAIN_APP_TEST_VIEWS.has(view)
  );
}

export function resolveMainAppWorkspaceContext(currentRoute, personalWorkspaceId) {
  if (currentRoute.path?.startsWith('/personal') && isPersonalWorkspaceView(currentRoute.view)) {
    return personalWorkspaceId
      ? { kind: 'workspace', workspaceId: personalWorkspaceId }
      : { kind: 'personal-pending' };
  }

  if (
    currentRoute.params?.id &&
    /^\d+$/.test(String(currentRoute.params.id)) &&
    isRegularWorkspaceView(currentRoute.view)
  ) {
    return { kind: 'workspace', workspaceId: currentRoute.params.id };
  }

  return GLOBAL_COLLECTION_VIEWS.has(currentRoute.view)
    ? { kind: 'global-collection' }
    : { kind: 'none' };
}

export function getMainAppWorkspaceRedirect(currentRoute, activeWorkspace) {
  if (currentRoute.view !== 'workspace-detail' || !activeWorkspace) return null;
  const workspaceId = currentRoute.params?.id;
  if (!workspaceId) return null;
  if (String(activeWorkspace.id) !== String(workspaceId)) return null;

  const collectionId = currentRoute.params?.collectionId;
  const base = collectionId
    ? `/workspaces/${workspaceId}/collections/${collectionId}`
    : `/workspaces/${workspaceId}`;
  return `${base}/${activeWorkspace.default_view || 'board'}`;
}

// Route params that only pick a section inside a shell that stays mounted
// (sidebar + content pane). The shell swaps its own content for these, so
// they must not change the navigation identity: remounting the shell resets
// its sidebar scroll and state on every section click (#542).
const SHELL_SECTION_PARAMS = {
  admin: ['tab', 'categoryId', 'type'],
  'delegated-admin': ['tab'],
  'team-detail': ['section'],
};

export function mainAppNavigationIdentity(route, personalWorkspaceId) {
  const context = resolveMainAppWorkspaceContext(route, personalWorkspaceId);
  const params = route.params ?? {};
  const collectionId = context.kind === 'global-collection' ? params.id : params.collectionId;
  const sectionParams = SHELL_SECTION_PARAMS[route.view] ?? [];
  const entities = Object.entries(params)
    .filter(([key]) => key !== 'id' && key !== 'collectionId' && !sectionParams.includes(key))
    .sort(([a], [b]) => a.localeCompare(b));
  return JSON.stringify([
    context.kind,
    context.workspaceId ? Number(context.workspaceId) : null,
    collectionId ? String(collectionId) : null,
    route.view,
    entities,
  ]);
}
