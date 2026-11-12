export const VIEW_BACKGROUND_CONTEXT = 'windshift:shared-view-background';

export function isCollectionContentView(view) {
  return /^(workspace|collection)-(board|backlog|list|tree|map|roadmap)$/.test(view ?? '');
}
