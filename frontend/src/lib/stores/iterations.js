import { writable } from 'svelte/store';
import { api } from '../api.js';

// Iterations store
function createIterationsStore() {
  const { subscribe, set, update } = writable([]);

  return {
    subscribe,

    // Initialize store by loading from API
    async init(filters = {}) {
      try {
        const iterations = await api.iterations.getAll(filters);
        set(iterations || []);
        return iterations || [];
      } catch (error) {
        console.error('Failed to load iterations:', error);
        set([]);
        return [];
      }
    },

    // Add a new iteration
    async add(iterationData) {
      try {
        const newIteration = await api.iterations.create(iterationData);
        update(iterations => [...iterations, newIteration]);
        return newIteration;
      } catch (error) {
        console.error('Failed to add iteration:', error);
        throw error;
      }
    },

    // Update an existing iteration
    async update(iterationId, updates) {
      try {
        const updatedIteration = await api.iterations.update(iterationId, updates);

        update(iterations => {
          const index = iterations.findIndex(i => i.id === iterationId);
          if (index !== -1) {
            iterations[index] = updatedIteration;
          }
          return iterations;
        });

        return updatedIteration;
      } catch (error) {
        console.error('Failed to update iteration:', error);
        throw error;
      }
    },

    // Delete an iteration
    async delete(iterationId) {
      try {
        await api.iterations.delete(iterationId);

        update(iterations => {
          return iterations.filter(i => i.id !== iterationId);
        });

        return true;
      } catch (error) {
        console.error('Failed to delete iteration:', error);
        throw error;
      }
    },

    // Filter iterations by type
    filterByType(iterations, typeId) {
      if (typeId === 'all' || !typeId) return iterations;
      return iterations.filter(i => i.type_id === parseInt(typeId));
    },

    // Group iterations by type
    groupByType(iterations, types) {
      const grouped = types.reduce((acc, type) => {
        acc[type.name] = iterations.filter(i => i.type_id === type.id);
        return acc;
      }, {
        'Uncategorized': iterations.filter(i => !i.type_id)
      });

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

    // Reset store (useful for testing or logout)
    reset() {
      set([]);
    }
  };
}

export const iterationsStore = createIterationsStore();
