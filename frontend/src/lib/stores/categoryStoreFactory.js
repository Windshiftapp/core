import { writable } from 'svelte/store';

const DEFAULT_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ef4444', '#ec4899', '#06b6d4', '#84cc16'];

/**
 * Factory function to create a category store
 * @param {Object} apiMethods - API methods for CRUD operations
 * @param {Function} apiMethods.getAll - Fetch all categories
 * @param {Function} apiMethods.create - Create a new category
 * @param {Function} apiMethods.update - Update a category
 * @param {Function} apiMethods.delete - Delete a category
 * @param {string} entityName - Name of the entity for error messages (e.g., 'milestone', 'channel', 'collection')
 * @returns {Object} Svelte store with category management methods
 */
export function createCategoryStore(apiMethods, entityName) {
  const { subscribe, set, update } = writable([]);

  return {
    subscribe,

    // Initialize store by loading from API
    async init() {
      try {
        const categories = await apiMethods.getAll();
        set(categories || []);
        return categories || [];
      } catch (error) {
        console.error(`Failed to load ${entityName} categories:`, error);
        set([]);
        return [];
      }
    },

    // Add a new category
    async add(categoryData) {
      try {
        // Set default color if not provided
        if (!categoryData.color) {
          categoryData.color = DEFAULT_COLORS[Math.floor(Math.random() * DEFAULT_COLORS.length)];
        }

        const newCategory = await apiMethods.create({
          name: categoryData.name.trim(),
          color: categoryData.color,
          description: categoryData.description || ''
        });

        update(categories => [...categories, newCategory]);
        return newCategory;
      } catch (error) {
        console.error(`Failed to add ${entityName} category:`, error);
        throw error;
      }
    },

    // Update an existing category
    async update(categoryId, updates) {
      try {
        const updatedCategory = await apiMethods.update(categoryId, updates);

        update(categories => {
          const index = categories.findIndex(c => c.id === categoryId);
          if (index !== -1) {
            categories[index] = updatedCategory;
          }
          return categories;
        });

        return updatedCategory;
      } catch (error) {
        console.error(`Failed to update ${entityName} category:`, error);
        throw error;
      }
    },

    // Delete a category
    async delete(categoryId) {
      try {
        await apiMethods.delete(categoryId);

        update(categories => {
          return categories.filter(c => c.id !== categoryId);
        });

        return true;
      } catch (error) {
        console.error(`Failed to delete ${entityName} category:`, error);
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
