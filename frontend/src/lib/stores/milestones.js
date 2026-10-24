import { milestones } from '../api/milestones.js';
import { createEntityStore } from './entityStoreFactory.js';

// Create base store using factory (with set exposed for direct updates)
const baseStore = createEntityStore(
  {
    getAll: () => milestones.getAll(),
    create: (data) => milestones.create(data),
    update: (id, updates) => milestones.update(id, updates),
    delete: (id) => milestones.delete(id),
  },
  'milestone',
  { exposeSet: true }
);

// Extend with milestone-specific methods
export const milestonesStore = {
  ...baseStore,

  // Reorder milestones within a scope. Optimistically reassigns local
  // positions from the ordered id list (so the UI snaps instantly), then
  // persists via the scope-specific reorder endpoint and rolls back on
  // error. `scope` is { is_global, workspace_id, category_id }.
  async reorder(scope, orderedIds, allMilestones) {
    // Snapshot for rollback before mutating.
    const previous = allMilestones;
    const step = 1000;
    const positionById = new Map(orderedIds.map((id, i) => [id, (i + 1) * step]));
    const next = allMilestones.map((m) =>
      positionById.has(m.id) ? { ...m, position: positionById.get(m.id) } : m
    );
    // Sort the snapshot by the new positions so the optimistic render matches
    // the server's post-reorder order immediately.
    next.sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
    milestonesStore.set(next);

    try {
      await milestones.reorder(scope, orderedIds);
    } catch (error) {
      // Roll back to the pre-drag snapshot.
      milestonesStore.set(previous);
      throw error;
    }
  },

  // Filter milestones by category
  filterByCategory(milestones, categoryId) {
    if (categoryId === 'all') return milestones;
    return milestones.filter((m) => m.category_id === parseInt(categoryId, 10));
  },

  // Group milestones by category
  groupByCategory(milestones, categories) {
    const grouped = categories.reduce(
      (acc, category) => {
        acc[category.name] = milestones.filter((m) => m.category_id === category.id);
        return acc;
      },
      {
        Uncategorized: milestones.filter((m) => !m.category_id),
      }
    );

    return grouped;
  },
};
