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
