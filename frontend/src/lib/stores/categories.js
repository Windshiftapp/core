import { writable } from 'svelte/store';
import { api } from '../api.js';

// Categories store for milestones
function createCategoriesStore() {
  const { subscribe, set, update } = writable([]);

  return {
    subscribe,
    
    // Initialize store by loading from API
    async init() {
      try {
        const categories = await api.milestoneCategories.getAll();
        set(categories || []);
        return categories || [];
      } catch (error) {
        console.error('Failed to load categories:', error);
        set([]);
        return [];
      }
    },

    // Add a new category
    async add(categoryData) {
      try {
        const colors = ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ef4444', '#ec4899', '#06b6d4', '#84cc16'];
        
        // Set default color if not provided
        if (!categoryData.color) {
          categoryData.color = colors[Math.floor(Math.random() * colors.length)];
        }
        
        const newCategory = await api.milestoneCategories.create({
          name: categoryData.name.trim(),
          color: categoryData.color,
          description: categoryData.description || ''
        });

        update(categories => [...categories, newCategory]);
        return newCategory;
      } catch (error) {
        console.error('Failed to add category:', error);
        throw error;
      }
    },

    // Update an existing category
    async update(categoryId, updates) {
      try {
        const updatedCategory = await api.milestoneCategories.update(categoryId, updates);
        
        update(categories => {
          const index = categories.findIndex(c => c.id === categoryId);
          if (index !== -1) {
            categories[index] = updatedCategory;
          }
          return categories;
        });

        return updatedCategory;
      } catch (error) {
        console.error('Failed to update category:', error);
        throw error;
      }
    },

    // Delete a category
    async delete(categoryId) {
      try {
        await api.milestoneCategories.delete(categoryId);
        
        update(categories => {
          return categories.filter(c => c.id !== categoryId);
        });

        return true;
      } catch (error) {
        console.error('Failed to delete category:', error);
        throw error;
      }
    },

    // Get category by ID
    getById(categoryId, currentCategories) {
      return currentCategories.find(c => c.id === categoryId);
    },

    // Reset store (useful for testing or logout)
    reset() {
      set([]);
    }
  };
}

export const categoriesStore = createCategoriesStore();