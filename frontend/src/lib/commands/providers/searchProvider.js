import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/**
 * Surface workspace-scoped or workspace-key-prefixed work item search
 * results. The palette debounces the API call and writes results into ctx.
 * Bucket-rank promotes search-results above global-navigation when the query
 * looks like an item key (`ABC-123`).
 */
export function searchProvider(ctx) {
  const { workItems, workItemsQuery } = ctx;
  if (!workItems?.length) return [];

  return workItems.map((item) => {
    const itemKey = `${item.workspace_key || 'WORK'}-${item.workspace_item_number || item.id}`;
    return createCommand({
      id: `goto-item-${item.id}`,
      label: `${itemKey}: ${item.title}`,
      description: `${item.workspace_name} • ${item.status}${item.priority ? ` • ${item.priority}` : ''}`,
      bucket: BUCKET.SEARCH_RESULTS,
      searchQuery: workItemsQuery,
      keywords: [
        itemKey.toLowerCase(),
        item.title?.toLowerCase(),
        item.workspace_name?.toLowerCase(),
        item.workspace_key?.toLowerCase(),
        String(item.workspace_item_number || ''),
        String(item.id),
      ].filter(Boolean),
      url: `/workspaces/${item.workspace_id}/items/${item.id}`,
    });
  });
}
