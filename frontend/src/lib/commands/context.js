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
 *   workItemsQuery?: string,
 *   item?: any,
 *   itemWorkspace?: any,
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
  workItemsQuery,
  item,
  itemWorkspace,
  activeTimer,
  t,
  query,
  pageResults,
}) {
  const path = route?.path || '';
  const params = route?.params || {};
  const candidates = [currentWorkspace, itemWorkspace, ...(workspaces || [])].filter(Boolean);
  let workspaceId = null;
  let collectionId = null;
  let itemId = null;
  let resolvedItem = null;

  if (/^\/workspaces\/\d+(?:\/|$)/.test(path)) {
    workspaceId = Number(params.id);
    collectionId = params.collectionId ?? null;
    itemId = params.itemId ?? null;
  } else if (/^\/personal(?:\/|$)/.test(path)) {
    workspaceId = candidates.find((ws) => ws.is_personal)?.id ?? null;
    itemId = params.itemId ?? null;
  } else if (/^\/collections\/\d+(?:\/|$)/.test(path)) {
    collectionId = params.id ?? null;
  } else if (/^\/(?:workspace\/[^/]+\/item\/\d+|item\/[^/]+)\/?$/.test(path)) {
    const key = params.itemKey || `${params.workspaceKey}-${params.itemNumber}`;
    const itemKey =
      item && `${item.workspace_key || itemWorkspace?.key}-${item.workspace_item_number}`;
    if (itemKey?.toLowerCase() === key.toLowerCase()) {
      workspaceId = item.workspace_id;
      itemId = item.id;
      resolvedItem = item;
    }
  }
  const workspace = workspaceId
    ? candidates.find((ws) => String(ws.id) === String(workspaceId)) || null
    : null;
  if (
    itemId &&
    String(item?.id) === String(itemId) &&
    String(item?.workspace_id) === String(workspaceId)
  ) {
    resolvedItem = item;
  }
  return {
    route,
    user: null,
    permissions: permissions || {},
    isSystemAdmin: !!isSystemAdmin,
    modules: modules || {},
    workspaces: workspaces || [],
    workspaceId,
    workspace,
    collectionId,
    itemId,
    item: resolvedItem,
    workItems: workItems || [],
    workItemsQuery,
    pageResults: pageResults || [],
    activeTimer: activeTimer || null,
    t,
    query: (query || '').trim(),
  };
}
