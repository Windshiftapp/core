import { BUCKET } from '../buckets.js';
import { createCommand } from '../types.js';

/**
 * Knowledge-page search results. The caller (desktop palette or mobile sheet)
 * debounces the API fan-out and writes tagged results into ctx.pageResults;
 * each row links to the page reader (mobile URLs are rewritten by the
 * mobile executor).
 */
export function pageSearchProvider(ctx) {
  const { pageResults } = ctx;
  if (!pageResults?.length) return [];

  return pageResults.map((p) =>
    createCommand({
      id: `goto-page-${p.workspace_id}-${p.id}`,
      label: p.title ?? '',
      description: p.workspace_name || '',
      bucket: BUCKET.SEARCH_RESULTS,
      keywords: [p.title?.toLowerCase(), p.workspace_name?.toLowerCase()].filter(Boolean),
      url: `/m/pages/${p.workspace_id}/${p.id}`,
    })
  );
}
