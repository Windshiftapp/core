import { writable } from 'svelte/store';
import { confirmDelete } from './useConfirm.js';

/**
 * Composable for CRUD operations with common patterns
 * @param {Object} options - Configuration options
 * @param {Function} options.loadFn - Function to load all items
 * @param {Function} options.createFn - Function to create an item
 * @param {Function} options.updateFn - Function to update an item
 * @param {Function} options.deleteFn - Function to delete an item
 * @param {Object} options.defaultFormData - Default form data structure
 * @param {string} options.itemName - Display name for confirmations (e.g. "status", "category")
 */
export function useCrud(options) {
  const {
    loadFn,
    createFn,
    updateFn,
    deleteFn,
    defaultFormData = {},
    itemName = 'item'
  } = options;

  // Reactive stores
  const items = writable([]);
  const loading = writable(false);
  const error = writable(null);
  const showCreateForm = writable(false);
  const editingId = writable(null);
  const formData = writable({ ...defaultFormData });
  const saving = writable(false);

  // Load all items
  async function loadItems() {
    loading.set(true);
    error.set(null);
    
    try {
      const result = await loadFn();
      items.set(result || []);
    } catch (err) {
      console.error(`Failed to load ${itemName}s:`, err);
      error.set(err.message);
      items.set([]);
    } finally {
      loading.set(false);
    }
  }

  // Start creating a new item
  function startCreate() {
    formData.set({ ...defaultFormData });
    editingId.set(null);
    showCreateForm.set(true);
  }

  // Start editing an existing item
  function startEdit(item) {
    const editData = { ...defaultFormData };
    // Copy existing item data into form
    Object.keys(item).forEach(key => {
      if (key in editData) {
        editData[key] = item[key];
      }
    });
    
    formData.set(editData);
    editingId.set(item.id);
    showCreateForm.set(true);
  }

  // Cancel form (close modal and reset)
  function cancelForm() {
    showCreateForm.set(false);
    editingId.set(null);
    formData.set({ ...defaultFormData });
  }

  // Save item (create or update)
  async function saveItem(data = null) {
    let currentFormData, currentEditingId;
    
    // Get current values
    if (data) {
      currentFormData = data.formData;
      currentEditingId = data.editingId;
    } else {
      currentFormData = get(formData);
      currentEditingId = get(editingId);
    }

    saving.set(true);
    error.set(null);

    try {
      let result;
      if (currentEditingId) {
        // Update existing item
        result = await updateFn(currentEditingId, currentFormData);
        items.update(currentItems =>
          currentItems.map(item => 
            item.id === currentEditingId ? result : item
          )
        );
      } else {
        // Create new item
        result = await createFn(currentFormData);
        items.update(currentItems => [...currentItems, result]);
      }

      // Close form on success
      cancelForm();
      return result;
    } catch (err) {
      console.error(`Failed to save ${itemName}:`, err);
      error.set(err.message);
      throw err;
    } finally {
      saving.set(false);
    }
  }

  // Delete item with confirmation
  async function deleteItem(item) {
    const confirmed = await confirmDelete(item.name || item.title || item.id, itemName);
    
    if (!confirmed) {
      return false;
    }

    try {
      await deleteFn(item.id);
      items.update(currentItems =>
        currentItems.filter(i => i.id !== item.id)
      );
      return true;
    } catch (err) {
      console.error(`Failed to delete ${itemName}:`, err);
      error.set(err.message);
      throw err;
    }
  }

  // Helper to get current values (for use outside reactive context)
  function get(store) {
    let value;
    store.subscribe(v => value = v)();
    return value;
  }

  return {
    // Stores
    items,
    loading,
    error,
    showCreateForm,
    editingId,
    formData,
    saving,

    // Actions
    loadItems,
    startCreate,
    startEdit,
    cancelForm,
    saveItem,
    deleteItem
  };
}