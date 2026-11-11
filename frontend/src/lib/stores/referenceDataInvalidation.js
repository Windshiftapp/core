import { API_MUTATION_EVENT } from '../api/core.js';
import { collectionEditorOptions } from './collectionEditorOptions.svelte.js';
import { workspaceDataStore } from './workspaceDataStore.svelte.js';

/**
 * Mutating a reference endpoint must drop the directories that cached it, or
 * boards, backlogs, lists, and pickers keep offering the pre-mutation data.
 *
 * Each rule names what to clear after a successful mutation of a matching
 * endpoint:
 * - `store`: a `workspaceDataStore` field,
 * - `editor`: a `collectionEditorOptions` family,
 * - `screenConfigs`: cached effective configurations,
 * - `fullRefresh`: everything (used for cross-cutting changes).
 *
 * Matching is deliberately coarse: re-fetching an unchanged family is cheap and
 * always safer than serving a stale directory. A workspace id is read from
 * `/workspaces/:id/...`; without one the mutation is treated as global and
 * clears every scoped editor cache.
 */
const REFERENCE_ENDPOINTS = [
  { match: /\/(?:statuses)(?:\/|$)/, store: 'statuses', editor: 'statuses', screenConfigs: true },
  { match: /\/(?:status-categories)(?:\/|$)/, store: 'statusCategories', screenConfigs: true },
  { match: /\/(?:item-types)(?:\/|$)/, store: 'itemTypes', screenConfigs: true },
  { match: /\/(?:hierarchy-levels)(?:\/|$)/, store: 'itemTypes', screenConfigs: true },
  {
    match: /\/(?:priorities)(?:\/|$)/,
    store: 'priorities',
    editor: 'priorities',
    screenConfigs: true,
  },
  { match: /\/(?:custom-fields)(?:\/|$)/, store: 'customFieldDefinitions', screenConfigs: true },
  { match: /\/(?:milestones)(?:\/|$)/, store: 'milestones', editor: 'milestones' },
  { match: /\/(?:iterations)(?:\/|$)/, store: 'iterations', editor: 'iterations' },
  { match: /\/(?:agent-profiles)(?:\/|$)/, store: 'users', editor: 'users' },
  { match: /\/(?:users)(?:\/|$)/, store: 'users', editor: 'users' },
  { match: /\/(?:groups)(?:\/|$)/, editor: 'teams' },
  { match: /\/(?:teams)(?:\/|$)/, editor: 'teams' },
  { match: /\/(?:time\/projects)(?:\/|$)/, store: 'projects', editor: 'projects' },
  { match: /\/(?:labels)(?:\/|$)/, store: 'labels' },
  { match: /\/(?:configuration-sets|screens|workflows)(?:\/|$)/, screenConfigs: true },
  // Object translations change display names across several directories.
  { match: /\/(?:object-translations)(?:\/|$)/, fullRefresh: true },
  { match: /^\/workspaces\/\d+$/, store: 'workspace' },
];

function workspaceIdFromEndpoint(endpoint) {
  const match = /\/(?:workspaces)\/(\d+)(?:\/|$)/.exec(endpoint);
  return match ? Number(match[1]) : null;
}

/** Rules whose endpoint pattern matches; exported for unit tests. */
export function referenceRulesForEndpoint(endpoint) {
  if (typeof endpoint !== 'string' || endpoint === '') return [];
  return REFERENCE_ENDPOINTS.filter((rule) => rule.match.test(endpoint));
}

/**
 * Apply the invalidation for one mutation endpoint. Returns true when a rule
 * matched. Cache arguments are injectable so the mapping can be unit tested
 * without the real stores.
 */
export function applyReferenceInvalidation(
  endpoint,
  { store = workspaceDataStore, editorOptions = collectionEditorOptions } = {}
) {
  const rules = referenceRulesForEndpoint(endpoint);
  if (rules.length === 0) return false;

  const workspaceId = workspaceIdFromEndpoint(endpoint);
  const activeId = store.workspaceId;
  const hasActiveWorkspace = activeId != null && activeId !== 'global';

  for (const rule of rules) {
    if (rule.fullRefresh) {
      void store.refresh?.();
      editorOptions.reset?.();
      continue;
    }
    if (rule.store && hasActiveWorkspace) {
      void store.invalidate(rule.store);
    }
    if (rule.editor) {
      if (workspaceId == null) editorOptions.invalidateFamily(rule.editor);
      else editorOptions.invalidate(workspaceId, rule.editor);
    }
    if (rule.screenConfigs) store.invalidateScreenConfigs?.();
  }

  return true;
}

/** Subscribe the app shell to successful mutations. Returns an unsubscribe. */
export function registerReferenceDataInvalidation() {
  if (typeof window === 'undefined') return () => {};
  const handler = (event) => applyReferenceInvalidation(event?.detail?.endpoint);
  window.addEventListener(API_MUTATION_EVENT, handler);
  return () => window.removeEventListener(API_MUTATION_EVENT, handler);
}
