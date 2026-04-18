/**
 * Central URL builders for every routed entity in the app.
 *
 * Use these instead of building URL strings inline. Keeping them here means:
 *   - one place to update when the router table (see lib/router.js) changes
 *   - safe to use as `href` on `<a>` / `<Link>` so cmd/ctrl-click and middle-click
 *     open links in a new tab (the router intercepts plain clicks).
 *
 * All builders return a path (not an absolute URL). Pass the result straight
 * into `<a href={...}>`, `<Link href={...}>`, or `navigate(...)`.
 */

function qs(params) {
  const entries = Object.entries(params).filter(
    ([, v]) => v !== undefined && v !== null && v !== ''
  );
  if (entries.length === 0) return '';
  const usp = new URLSearchParams();
  for (const [k, v] of entries) usp.set(k, String(v));
  return `?${usp.toString()}`;
}

// ───────────────────────── Items ─────────────────────────

/**
 * @param {object} opts
 * @param {string|number} [opts.workspaceId]
 * @param {string|number} opts.itemId
 * @param {string|number} [opts.collectionId]
 * @param {boolean} [opts.isPersonal] - use /personal/items/:id
 * @param {string} [opts.tab] - appended as ?tab=...
 */
export function itemUrl({ workspaceId, itemId, collectionId, isPersonal = false, tab } = {}) {
  let path;
  if (isPersonal) {
    path = `/personal/items/${itemId}`;
  } else if (collectionId) {
    path = `/workspaces/${workspaceId}/collections/${collectionId}/items/${itemId}`;
  } else {
    path = `/workspaces/${workspaceId}/items/${itemId}`;
  }
  return path + qs({ tab });
}

export function personalItemUrl(itemId, { tab } = {}) {
  return itemUrl({ itemId, isPersonal: true, tab });
}

// ───────────────────────── Portal ────────────────────────

export function portalUrl(slug) {
  return `/portal/${slug}`;
}

export function portalRequestUrl(slug, itemId) {
  return `/portal/${slug}${qs({ view: 'requests', id: itemId })}`;
}

export function portalRequestTypeUrl(slug, requestTypeId) {
  return `/portal/${slug}${qs({ 'request-type': requestTypeId })}`;
}

// ───────────────────────── Assets ────────────────────────

export function assetsListUrl() {
  return '/assets';
}

export function assetUrl(id) {
  return `/assets/${id}`;
}

export function assetSettingsUrl() {
  return '/assets/settings';
}

// ─────────────────────── Customers ───────────────────────

export function customersUrl() {
  return '/customers';
}

export function customerContactUrl(contactId) {
  return `/customers/contacts/${contactId}`;
}

// ─────────────────────── Milestones ──────────────────────

export function milestonesUrl({ categoryId } = {}) {
  return categoryId ? `/milestones/category/${categoryId}` : '/milestones';
}

export function milestoneUrl(id) {
  return `/milestones/${id}`;
}

// ─────────────────────── Iterations ──────────────────────

export function iterationsUrl({ typeId } = {}) {
  return typeId ? `/iterations/type/${typeId}` : '/iterations';
}

export function iterationUrl(id) {
  return `/iterations/${id}`;
}

export function iterationDependenciesUrl(id) {
  return `/iterations/${id}/dependencies`;
}

// ───────────────────────── Logbook ───────────────────────

export function logbookUrl() {
  return '/logbook';
}

export function logbookBucketUrl(bucketId) {
  return `/logbook/bucket/${bucketId}`;
}

export function logbookDocumentUrl(documentId) {
  return `/logbook/documents/${documentId}`;
}

// ───────────────────────── Tests ─────────────────────────

export function testsBaseUrl(workspaceId) {
  return `/workspaces/${workspaceId}/tests`;
}

export function testCasesUrl(workspaceId) {
  return `/workspaces/${workspaceId}/tests/cases`;
}

export function testCaseUrl(workspaceId, testId) {
  return `/workspaces/${workspaceId}/tests/cases/${testId}`;
}

export function testCaseStepsUrl(workspaceId, testId) {
  return `/workspaces/${workspaceId}/tests/cases/${testId}/steps`;
}

export function testSetsUrl(workspaceId) {
  return `/workspaces/${workspaceId}/tests/sets`;
}

export function testSetUrl(workspaceId, setId) {
  return `/workspaces/${workspaceId}/tests/sets/${setId}`;
}

export function testTemplatesUrl(workspaceId) {
  return `/workspaces/${workspaceId}/tests/templates`;
}

export function testTemplateUrl(workspaceId, templateId) {
  return `/workspaces/${workspaceId}/tests/templates/${templateId}`;
}

export function testRunsUrl(workspaceId) {
  return `/workspaces/${workspaceId}/tests/runs`;
}

export function testRunUrl(workspaceId, runId) {
  return `/workspaces/${workspaceId}/tests/runs/${runId}`;
}

export function testRunExecuteUrl(workspaceId, runId) {
  return `/workspaces/${workspaceId}/tests/runs/${runId}/execute`;
}

export function testReportsUrl(workspaceId) {
  return `/workspaces/${workspaceId}/tests/reports`;
}

// ─────────────────────── Workspaces ──────────────────────

/**
 * @param {string|number} id
 * @param {string} [view]
 *   One of: 'overview' | 'board' | 'backlog' | 'list' | 'tree' | 'map' |
 *   'roadmap' | 'iterations' | 'milestones' | 'analytics' | 'actions' |
 *   'calendar' | 'reviews' | 'look-and-feel'. Omit for the workspace root.
 */
export function workspaceUrl(id, view) {
  return view ? `/workspaces/${id}/${view}` : `/workspaces/${id}`;
}

/** Personal workspace counterpart of workspaceUrl. */
export function personalUrl(view) {
  if (!view) return '/personal';
  if (view === 'plan' || view === 'calendar' || view === 'reviews') {
    return `/personal/${view}`;
  }
  return '/personal';
}

/**
 * @param {string|number} id
 * @param {string} [tab]
 *   'general' | 'categories' | 'members' | 'configuration' | 'source-control'
 *   | 'issue-sync' | 'recurrence' | 'danger'. Omit for the settings root.
 */
export function workspaceSettingsUrl(id, tab) {
  return tab ? `/workspaces/${id}/settings/${tab}` : `/workspaces/${id}/settings`;
}

export function workspaceBoardConfigureUrl(id, collectionId) {
  return collectionId
    ? `/workspaces/${id}/collections/${collectionId}/board/configure`
    : `/workspaces/${id}/board/configure`;
}

// ─────────────────────── Collections ─────────────────────

export function collectionsListUrl({ categoryId, scope } = {}) {
  if (categoryId) return `/collections/category/${categoryId}`;
  if (scope === 'workspace') return '/collections/workspace';
  return '/collections';
}

/**
 * @param {string|number} id
 * @param {string} [view]
 *   'board' | 'board/configure' | 'backlog' | 'list' | 'tree' | 'map' | 'roadmap'.
 *   Omit for the collection edit view.
 */
export function collectionUrl(id, view) {
  return view ? `/collections/${id}/${view}` : `/collections/${id}`;
}

export function workspaceCollectionUrl(workspaceId, collectionId, view) {
  const base = `/workspaces/${workspaceId}/collections/${collectionId}`;
  return view ? `${base}/${view}` : base;
}

// ────────────────────────── Misc ─────────────────────────

export function homepageUrl() {
  return '/';
}

export function searchUrl(query) {
  return `/search${qs({ q: query })}`;
}

export function dashboardUrl() {
  return '/dashboard';
}

export function notificationsUrl() {
  return '/notifications';
}

export function hubUrl() {
  return '/channels';
}

export function hubInboxUrl() {
  return '/channels/inbox';
}

export function profileUrl() {
  return '/profile';
}

export function securityUrl() {
  return '/security';
}

export function aboutUrl() {
  return '/about';
}

export function publicBoardUrl(slug) {
  return `/board/${slug}`;
}

export function publicFormUrl(slug) {
  return `/forms/${slug}`;
}

export function setPasswordUrl(token) {
  return `/set-password/${token}`;
}

export function workflowDesignerUrl(id) {
  return `/workflows/${id}/design`;
}
