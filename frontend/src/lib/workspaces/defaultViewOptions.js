// Label keys for the workspace default-view selector. The support queue is a
// collection-scoped route like the board views, so it can be the default too.
const DEFAULT_VIEW_LABEL_KEYS = {
  board: 'workspaceSettings.views.board',
  backlog: 'workspaceSettings.views.backlog',
  list: 'workspaceSettings.views.list',
  tree: 'workspaceSettings.views.tree',
  map: 'workspaceSettings.views.map',
  overview: 'workspaceSettings.views.overview',
  queue: 'supportQueue.title',
};

/**
 * Builds the default-view options from a scope's enabled views. The current
 * value is always offered, even when settings drift out of sync, and ids
 * without a label (legacy values) are dropped.
 *
 * @param {string[] | null | undefined} enabledViewIds
 * @param {string | null | undefined} currentView
 * @param {(key: string) => string} translate
 */
export function buildDefaultViewOptions(enabledViewIds, currentView, translate) {
  const views = new Set([...(enabledViewIds ?? []), currentView].filter(Boolean));
  return [...views]
    .filter((value) => DEFAULT_VIEW_LABEL_KEYS[value])
    .map((value) => ({ value, label: translate(DEFAULT_VIEW_LABEL_KEYS[value]) }));
}
