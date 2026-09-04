/** Shared page-tree predicates.
 *
 * Pages carry a materialized `path` of ancestor ids in the schema format
 * "/a/b/c/" (root pages are "/"). The children of page N therefore live at
 * `${N.path}${N.id}/`, and every descendant of N has a path starting with
 * that prefix. Both the drag-and-drop drop-target check and the Move
 * dialog's candidate list need this rule, and they disagreed on the details
 * when written separately — keep it in one place.
 */

/** The path prefix shared by every descendant of `page`. */
export function descendantPathPrefix(page) {
  return `${page.path}${page.id}/`;
}

/** True when `candidate` is `page` itself or sits anywhere beneath it.
 *
 * Reparenting a page into its own subtree would orphan the cycle, so this
 * is the client-side guard for both drop targets and move candidates. The
 * backend remains the authority: PageService.Move re-checks and answers
 * 409 "move would create a cycle".
 */
export function isSelfOrDescendant(candidate, page) {
  if (!candidate || !page) return false;
  if (candidate.id === page.id) return true;
  return candidate.path.startsWith(descendantPathPrefix(page));
}

/** Return a flat page list in parent-before-children display order. */
export function orderPagesDepthFirst(pages) {
  const pageIds = new Set(pages.map((page) => page.id));
  const childrenByParent = new Map();
  const roots = [];

  for (const page of pages) {
    if (page.parent_id == null || !pageIds.has(page.parent_id)) {
      roots.push(page);
      continue;
    }
    const children = childrenByParent.get(page.parent_id) ?? [];
    children.push(page);
    childrenByParent.set(page.parent_id, children);
  }

  const ordered = [];
  const append = (page) => {
    ordered.push(page);
    for (const child of childrenByParent.get(page.id) ?? []) append(child);
  };
  for (const root of roots) append(root);
  return ordered;
}
