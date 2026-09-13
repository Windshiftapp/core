/**
 * Build a CommandContext snapshot for the palette providers. The palette
 * calls this with current store values inside a $derived so providers re-run
 * automatically when anything changes.
 *
 * @param {{
 *   route: any,
 *   permissions: Record<string, any>,
 *   isSystemAdmin: boolean,
 *   modules: Record<string, any>,
 *   workspaces: any[],
 *   currentWorkspace: any,
 *   workItems: any[],
 *   activeTimer: any,
 *   t: (key:string, fallback?:any) => string,
 *   query: string,
 *   pageResults?: any[],
 * }} input
 */
export function buildContext({
  route,
  permissions,
  isSystemAdmin,
  modules,
  workspaces,
  currentWorkspace,
  workItems,
  activeTimer,
  t,
  query,
  pageResults,
}) {
  const workspaceId = route?.params?.id ? Number(route.params.id) : null;
  const collectionId = route?.params?.collectionId ?? null;
  const itemId = route?.params?.itemId ?? null;
  return {
    route,
    user: null,
    permissions: permissions || {},
    isSystemAdmin: !!isSystemAdmin,
    modules: modules || {},
    workspaces: workspaces || [],
    workspaceId,
    workspace: currentWorkspace || null,
    collectionId,
    itemId,
    item: null,
    workItems: workItems || [],
    pageResults: pageResults || [],
    activeTimer: activeTimer || null,
    t,
    query: query || '',
  };
}
