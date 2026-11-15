import { writable } from 'svelte/store';

/**
 * Factory function to create an entity store with CRUD operations
 * @param {Object} apiMethods - API methods for CRUD operations
 * @param {Function} apiMethods.getAll - Fetch all entities (can accept optional filters)
 * @param {Function} apiMethods.create - Create a new entity
 * @param {Function} apiMethods.update - Update an entity
 * @param {Function} apiMethods.delete - Delete an entity
 * @param {string} entityName - Name of the entity for error messages (e.g., 'iteration', 'milestone')
 * @param {{ exposeSet?: boolean }} [options] - Optional configuration
 * @returns {Object} Svelte store with entity management methods
 */
export function createEntityStore(apiMethods, entityName, options = {}) {
  const { subscribe, set, update } = writable([]);
  const { exposeSet = false } = options;
  let generation = 0;

  const store = {
    subscribe,
    captureGeneration: () => {
      const captured = generation;
      return () => captured === generation;
    },

    // Initialize store by loading from API
    async init(filters = {}) {
      const captured = generation;
      try {
        const entities = await apiMethods.getAll(filters);
        if (captured !== generation) return [];
        set(entities || []);
        return entities || [];
      } catch (error) {
        if (captured !== generation) return [];
        console.error(`Failed to load ${entityName}s:`, error);
        set([]);
        return [];
      }
    },

    // Add a new entity
    async add(entityData) {
      const captured = generation;
      try {
        const newEntity = await apiMethods.create(entityData);
        if (captured === generation) update((entities) => [...entities, newEntity]);
        return newEntity;
      } catch (error) {
        console.error(`Failed to add ${entityName}:`, error);
        throw error;
      }
    },

    // Update an existing entity. Return a NEW array (not the mutated original)
    // so downstream `$derived` reads in Svelte 5 see a changed reference and
    // re-evaluate. In-place `entities[i] = updated; return entities` notifies
    // store subscribers but fine-grained derives can short-circuit on identity.
    async update(entityId, updates) {
      const captured = generation;
      try {
        const updatedEntity = await apiMethods.update(entityId, updates);

        update((entities) => {
          if (captured !== generation) return entities;
          const index = entities.findIndex((e) => e.id === entityId);
          if (index === -1) return entities;
          const next = entities.slice();
          next[index] = updatedEntity;
          return next;
        });

        return updatedEntity;
      } catch (error) {
        console.error(`Failed to update ${entityName}:`, error);
        throw error;
      }
    },

    // Delete an entity
    async delete(entityId) {
      const captured = generation;
      try {
        await apiMethods.delete(entityId);

        update((entities) => {
          if (captured !== generation) return entities;
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
      generation += 1;
      set([]);
    },
  };

  // Optionally expose set for direct updates
  if (exposeSet) {
    store.set = set;
  }

  return store;
}
