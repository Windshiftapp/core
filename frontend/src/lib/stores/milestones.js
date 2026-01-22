import { api } from '../api.js';
import { createEntityStore } from './entityStoreFactory.js';

// Create base store using factory (with set exposed for direct updates)
const baseStore = createEntityStore(
  {
    getAll: () => api.milestones.getAll(),
    create: (data) => api.milestones.create(data),
    update: (id, updates) => api.milestones.update(id, updates),
    delete: (id) => api.milestones.delete(id)
  },
  'milestone',
  { exposeSet: true }
);

// Extend with milestone-specific methods
export const milestonesStore = {
  ...baseStore,

  // Filter milestones by category
  filterByCategory(milestones, categoryId) {
    if (categoryId === 'all') return milestones;
    return milestones.filter(m => m.category_id === parseInt(categoryId));
  },

  // Group milestones by category
  groupByCategory(milestones, categories) {
    const grouped = categories.reduce((acc, category) => {
      acc[category.name] = milestones.filter(m => m.category_id === category.id);
      return acc;
    }, {
      'Uncategorized': milestones.filter(m => !m.category_id)
    });

    return grouped;
  }
};
