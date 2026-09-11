/**
 * Items whose parent is not part of the loaded set. Pagination and filters
 * split hierarchies across pages, so a child can be loaded while its parent
 * is not. Returns a map keyed by the missing parent id because one ancestor
 * fetch per missing parent covers every orphan below it.
 */
export function findOrphansByMissingParent(items) {
  const itemIds = new Set(items.map((item) => item.id));
  const orphansByParent = new Map();
  for (const item of items) {
    if (item.parent_id == null || itemIds.has(item.parent_id)) continue;
    const orphans = orphansByParent.get(item.parent_id);
    if (orphans) {
      orphans.push(item);
    } else {
      orphansByParent.set(item.parent_id, [item]);
    }
  }
  return orphansByParent;
}

/**
 * Adds fetched ancestor chains (root -> parent order) to the loaded items so
 * hierarchies render intact. Entries whose parent has since been loaded by the
 * regular query are ignored, and items already present are never duplicated.
 */
export function mergeAncestorContext(items, ancestorsByParent) {
  if (!ancestorsByParent) return items;
  const itemIds = new Set(items.map((item) => item.id));
  const merged = [...items];
  for (const [parentId, chain] of Object.entries(ancestorsByParent)) {
    if (itemIds.has(Number(parentId))) continue;
    for (const ancestor of chain ?? []) {
      if (itemIds.has(ancestor.id)) continue;
      itemIds.add(ancestor.id);
      merged.push(ancestor);
    }
  }
  return merged;
}

export function indexCollectionHierarchy(items) {
  const itemIds = new Set(items.map((item) => item.id));
  const childrenByParent = new Map();
  const roots = [];

  for (const item of items) {
    if (item.parent_id == null || !itemIds.has(item.parent_id)) {
      roots.push(item);
      continue;
    }
    const children = childrenByParent.get(item.parent_id);
    if (children) {
      children.push(item);
    } else {
      childrenByParent.set(item.parent_id, [item]);
    }
  }

  return { roots, childrenByParent };
}
