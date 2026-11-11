import { GENERIC_SUBTASK_HIERARCHY_LEVEL } from '../../utils/hierarchy.js';

/**
 * Merge configured item types with filtered level counts into the level options
 * shown by the picker. Levels present in either source are included so an item
 * with no type, or a type with no matches, is still reachable. Sub-task (-1) is
 * level-independent and never offered as a root.
 */
export function buildHierarchyLevels(itemTypes = [], counts = []) {
  const byLevel = new Map();
  for (const type of itemTypes) {
    const level = Number(type.hierarchy_level ?? 0);
    if (level === GENERIC_SUBTASK_HIERARCHY_LEVEL) continue;
    if (!byLevel.has(level)) byLevel.set(level, { level, types: [], name: '', total: 0 });
    byLevel.get(level).types.push(type);
  }
  for (const entry of counts) {
    const level = Number(entry.level);
    if (level === GENERIC_SUBTASK_HIERARCHY_LEVEL) continue;
    if (!byLevel.has(level)) byLevel.set(level, { level, types: [], name: '', total: 0 });
    const bucket = byLevel.get(level);
    bucket.total = entry.total ?? 0;
    if (!bucket.name) bucket.name = entry.name ?? '';
  }
  return [...byLevel.values()]
    .map((entry) => ({
      ...entry,
      label: entry.types.length
        ? entry.types.map((type) => type.name).join(', ')
        : entry.name || `Level ${entry.level}`,
    }))
    .sort((a, b) => a.level - b.level);
}

/**
 * Default level for a scope: the shallowest level with matches, falling back to
 * the shallowest known level so a filter that matches nothing still resolves.
 * Returns null when no level is known yet.
 */
export function defaultHierarchyLevel(levels) {
  if (!levels || levels.length === 0) return null;
  const withMatches = levels.find((entry) => entry.total > 0);
  return (withMatches ?? levels[0]).level;
}
