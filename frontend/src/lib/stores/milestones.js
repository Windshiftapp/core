import { writable } from 'svelte/store';
import { api } from '../api.js';

// Milestones store
function createMilestonesStore() {
  const { subscribe, set, update } = writable([]);

  return {
    subscribe,
    set, // Expose set for direct updates

    // Initialize store by loading from API
    async init() {
      try {
        const milestones = await api.milestones.getAll();
        set(milestones || []);
        return milestones || [];
      } catch (error) {
        console.error('Failed to load milestones:', error);
        set([]);
        return [];
      }
    },

    // Add a new milestone
    async add(milestoneData) {
      try {
        const newMilestone = await api.milestones.create(milestoneData);
        update(milestones => [...milestones, newMilestone]);
        return newMilestone;
      } catch (error) {
        console.error('Failed to add milestone:', error);
        throw error;
      }
    },

    // Update an existing milestone
    async update(milestoneId, updates) {
      try {
        const updatedMilestone = await api.milestones.update(milestoneId, updates);
        
        update(milestones => {
          const index = milestones.findIndex(m => m.id === milestoneId);
          if (index !== -1) {
            milestones[index] = updatedMilestone;
          }
          return milestones;
        });

        return updatedMilestone;
      } catch (error) {
        console.error('Failed to update milestone:', error);
        throw error;
      }
    },

    // Delete a milestone
    async delete(milestoneId) {
      try {
        await api.milestones.delete(milestoneId);
        
        update(milestones => {
          return milestones.filter(m => m.id !== milestoneId);
        });

        return true;
      } catch (error) {
        console.error('Failed to delete milestone:', error);
        throw error;
      }
    },

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
    },

    // Reset store (useful for testing or logout)
    reset() {
      set([]);
    }
  };
}

export const milestonesStore = createMilestonesStore();