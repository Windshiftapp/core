import { api } from '../api.js';
import { createEntityStore } from './entityStoreFactory.js';

// Create base store using factory
const baseStore = createEntityStore(
  {
    getAll: (filters) => api.iterations.getAll(filters),
    create: (data) => api.iterations.create(data),
    update: (id, updates) => api.iterations.update(id, updates),
    delete: (id) => api.iterations.delete(id),
  },
  'iteration'
);

// Extend with iteration-specific methods
export const iterationsStore = {
  ...baseStore,

  // Filter iterations by type
  filterByType(iterations, typeId) {
    if (typeId === 'all' || !typeId) return iterations;
    return iterations.filter((i) => i.type_id === parseInt(typeId, 10));
  },

  // Group iterations by type
  groupByType(iterations, types) {
    const grouped = types.reduce(
      (acc, type) => {
        acc[type.name] = iterations.filter((i) => i.type_id === type.id);
        return acc;
      },
      {
        Uncategorized: iterations.filter((i) => !i.type_id),
      }
    );

    return grouped;
  },

  // Get progress for an iteration
  async getProgress(iterationId) {
    try {
      return await api.iterations.getProgress(iterationId);
    } catch (error) {
      console.error('Failed to get iteration progress:', error);
      throw error;
    }
  },
};
