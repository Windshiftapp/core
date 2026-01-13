import { api } from '../api.js';

/**
 * Generic service for updating work item fields
 * Extracted from ItemDetail.svelte to avoid code duplication
 */
export class ItemUpdateService {
  constructor() {
    this.saving = false;
  }

  /**
   * Update a single field of a work item
   * @param {Object} item - The current item data
   * @param {string} field - Field name to update 
   * @param {any} value - New value for the field
   * @param {Function} onSuccess - Callback with updated item data
   * @param {Function} onError - Error callback
   * @returns {Promise<Object>} Updated item data
   */
  async updateField(item, field, value, onSuccess = null, onError = null) {
    if (this.saving) {
      console.warn('Update already in progress, skipping');
      return item;
    }

    try {
      this.saving = true;
      
      // Validate and prepare update data
      const updateData = this._prepareUpdateData(item, field, value);
      
      // Skip update if no changes detected
      if (!updateData) {
        return item;
      }

      
      // Make API call
      const updatedItem = await api.items.update(item.id, updateData);
      
      // Merge updated data with existing item
      const mergedItem = { ...item, ...updatedItem };
      
      // Call success callback
      if (onSuccess) {
        onSuccess(mergedItem, field, value);
      }
      
      return mergedItem;
      
    } catch (error) {
      console.error('Failed to update field:', field, error);
      
      // Call error callback
      if (onError) {
        onError(error, field, value);
      } else {
        // Default error handling
        throw error;
      }
      
      return item; // Return original item on error
      
    } finally {
      this.saving = false;
    }
  }

  /**
   * Check if currently saving
   */
  isSaving() {
    return this.saving;
  }

  /**
   * Prepare update data for API call
   * @private
   */
  _prepareUpdateData(item, field, value) {
    const updateData = {};
    
    switch (field) {
      case 'title':
        const newTitle = value?.trim();
        if (!newTitle || newTitle === item.title) {
          return null; // No changes
        }
        updateData.title = newTitle;
        break;
        
      case 'description':
        const newDescription = value || '';
        if (newDescription === (item.description || '')) {
          return null; // No changes
        }
        updateData.description = newDescription;
        break;
        
      case 'status':
        if (!value || value === item.status) {
          return null; // No changes
        }
        updateData.status = value;
        break;
        
      case 'priority':
        if (!value || value === item.priority) {
          return null; // No changes
        }
        updateData.priority = value;
        break;

      case 'priority_id':
        const newPriorityId = value !== undefined && value !== null ? value : null;
        if (newPriorityId === item.priority_id) {
          return null; // No changes
        }
        updateData.priority_id = newPriorityId;
        break;

      case 'milestone':
        const newMilestone = value !== undefined ? value : null;
        if (newMilestone === item.milestone_id) {
          return null; // No changes
        }
        updateData.milestone_id = newMilestone;
        break;
        
      case 'assignee':
        const newAssignee = value !== undefined ? value : null;
        if (newAssignee === item.assignee_id) {
          return null; // No changes
        }
        updateData.assignee_id = newAssignee;
        break;
        
      default:
        // Handle custom fields
        if (field.startsWith('custom_field_')) {
          const fieldId = field.replace('custom_field_', '');
          const currentCustomValues = item.custom_field_values || {};
          
          if (value === currentCustomValues[fieldId]) {
            return null; // No changes
          }
          
          updateData.custom_field_values = {
            ...currentCustomValues,
            [fieldId]: value
          };
        } else {
          throw new Error(`Unknown field: ${field}`);
        }
        break;
    }
    
    return updateData;
  }

  /**
   * Validate field value
   * @param {string} field - Field name
   * @param {any} value - Value to validate
   * @returns {Object} { isValid: boolean, error?: string }
   */
  validateField(field, value) {
    switch (field) {
      case 'title':
        if (!value || !value.trim()) {
          return { isValid: false, error: 'Title is required' };
        }
        if (value.trim().length > 255) {
          return { isValid: false, error: 'Title must be less than 255 characters' };
        }
        break;
        
      case 'status':
        if (!value) {
          return { isValid: false, error: 'Status is required' };
        }
        break;
        
      case 'priority':
        // Priority string validation - backend will validate against actual priorities
        if (value && typeof value !== 'string') {
          return { isValid: false, error: 'Priority must be a string' };
        }
        break;

      case 'priority_id':
        // Priority ID validation - must be a number if provided
        if (value !== null && value !== undefined && typeof value !== 'number') {
          return { isValid: false, error: 'Priority ID must be a number' };
        }
        break;
    }
    
    return { isValid: true };
  }
}

// Create singleton instance
export const itemUpdateService = new ItemUpdateService();