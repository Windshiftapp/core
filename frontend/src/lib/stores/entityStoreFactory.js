import { writable } from 'svelte/store';

/**
 * Factory function to create an entity store with CRUD operations
 * @param {Object} apiMethods - API methods for CRUD operations
 * @param {Function} apiMethods.getAll - Fetch all entities (can accept optional filters)
 * @param {Function} apiMethods.create - Create a new entity
 * @param {Function} apiMethods.update - Update an entity
 * @param {Function} apiMethods.delete - Delete an entity
 * @param {string} entityName - Name of the entity for error messages (e.g., 'iteration', 'milestone')
 * @param {Object} options - Optional configuration
 * @param {boolean} options.exposeSet - Whether to expose the set method (default: false)
 * @returns {Object} Svelte store with entity management methods
 */
export function createEntityStore(apiMethods, entityName, options = {}) {
  const { subscribe, set, update } = writable([]);
  const { exposeSet = false } = options;

  const store = {
    subscribe,

    // Initialize store by loading from API
    async init(filters = {}) {
      try {
        const entities = await apiMethods.getAll(filters);
        set(entities || []);
        return entities || [];
      } catch (error) {
        console.error(`Failed to load ${entityName}s:`, error);
        set([]);
        return [];
      }
    },

    // Add a new entity
    async add(entityData) {
      try {
        const newEntity = await apiMethods.create(entityData);
        update((entities) => [...entities, newEntity]);
        return newEntity;
      } catch (error) {
        console.error(`Failed to add ${entityName}:`, error);
        throw error;
      }
    },

    // Update an existing entity
    async update(entityId, updates) {
      try {
        const updatedEntity = await apiMethods.update(entityId, updates);

        update((entities) => {
          const index = entities.findIndex((e) => e.id === entityId);
          if (index !== -1) {
            entities[index] = updatedEntity;
          }
          return entities;
        });

        return updatedEntity;
      } catch (error) {
        console.error(`Failed to update ${entityName}:`, error);
        throw error;
      }
    },

    // Delete an entity
    async delete(entityId) {
      try {
        await apiMethods.delete(entityId);

        update((entities) => {
          return entities.filter((e) => e.id !== entityId);
        });

        return true;
      } catch (error) {
        console.error(`Failed to delete ${entityName}:`, error);
        throw error;
      }
    },

    // Reset store (useful for testing or logout)
    reset() {
      set([]);
    },
  };

  // Optionally expose set for direct updates
  if (exposeSet) {
    store.set = set;
  }

  return store;
}
