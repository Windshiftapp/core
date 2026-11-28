/**
 * Build the narrow per-request context blob sent to the AI chat backend.
 *
 * Keep this intentionally small: it is used only to nudge the agent toward the
 * object the user is already viewing. Backend tools still re-check all
 * authorization before reading or mutating anything.
 */
const planningViews = new Set([
  'workspace-board',
  'collection-board',
  'workspace-backlog',
  'collection-backlog',
  'workspace-list',
  'collection-list',
  'workspace-roadmap',
  'collection-roadmap',
  'workspace-queue',
]);
const testRunViews = new Set(['test-run-detail', 'test-run-summary-print', 'test-execution']);

export const timeReportChatContext = { current: null };

function positiveID(value) {
  const id = Number(value);
  return Number.isSafeInteger(id) && id > 0 ? id : undefined;
}

export function buildChatContext(route, search = '', timeReport = null) {
  if (!route) return undefined;

  if (route.view === 'workspace-actions') {
    const ctx = { view: route.view };
    const wsId = Number(route.params?.id);
    if (wsId) ctx.workspace_id = wsId;
    const actionId = Number(route.params?.actionId);
    if (actionId) ctx.action_id = actionId;
    return ctx;
  }

  if (route.view === 'workspace-pages') {
    const ctx = { view: route.view };
    const wsId = Number(route.params?.id);
    if (wsId) ctx.workspace_id = wsId;
    const pageId = Number(route.params?.pageId);
    if (pageId) ctx.page_id = pageId;
    return ctx;
  }

  if (route.view === 'item-detail') {
    const ctx = { view: route.view };
    const wsId = Number(route.params?.id);
    if (wsId) ctx.workspace_id = wsId;

    const itemId = Number(route.params?.itemId);
    if (itemId) {
      ctx.item_id = itemId;
    } else if (route.params?.itemKey) {
      ctx.item_key = route.params.itemKey;
    } else if (route.params?.workspaceKey && route.params?.itemNumber) {
      ctx.item_key = `${route.params.workspaceKey}-${route.params.itemNumber}`;
    }

    return Object.keys(ctx).length > 1 ? ctx : undefined;
  }

  if (planningViews.has(route.view)) {
    const collectionOnly = route.view.startsWith('collection-');
    const ctx = {
      view: route.view,
      workspace_id: collectionOnly ? undefined : positiveID(route.params?.id),
      collection_id: positiveID(collectionOnly ? route.params?.id : route.params?.collectionId),
    };
    if (route.view === 'workspace-queue') {
      const queue = new URLSearchParams(search).get('queue') ?? route.query?.queue ?? '';
      ctx.queue_id = positiveID(queue);
      if (!ctx.queue_id && /^[a-z][a-z0-9_-]{0,49}$/.test(queue)) ctx.queue_key = queue;
    }
    return ctx;
  }
  if (route.view === 'iteration-detail' || route.view === 'iteration-dependencies') {
    return { view: route.view, iteration_id: positiveID(route.params?.id) };
  }
  if (route.view === 'milestone-detail') {
    return { view: route.view, milestone_id: positiveID(route.params?.id) };
  }
  if (testRunViews.has(route.view)) {
    return {
      view: route.view,
      workspace_id: positiveID(route.params?.id),
      test_run_id: positiveID(route.params?.runId),
    };
  }
  if (route.view === 'test-reports') {
    return { view: route.view, workspace_id: positiveID(route.params?.id) };
  }
  if (route.view === 'time' || route.view === 'time-report-print') {
    const report = !route.path || route.path === '/time/worklogs' ? timeReport : null;
    const validDate = (date) => (/^\d{4}-\d{2}-\d{2}$/.test(date ?? '') ? date : undefined);
    return {
      view: route.view,
      project_id: positiveID(report?.project_id ?? route.query?.project_id),
      customer_id: positiveID(report?.customer_id ?? route.query?.customer_id),
      mine_only: report?.mine_only === true ? true : undefined,
      date_from: validDate(report?.date_from ?? route.query?.date_from),
      date_to: validDate(report?.date_to ?? route.query?.date_to),
    };
  }

  return undefined;
}
