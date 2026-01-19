/**
 * Status Categories Store - Svelte 5 runes version
 * Provides shared access to status categories with color lookup utilities
 */

import { api } from '../api.js';
import { getTextColorForBackground, getStatusType } from '../utils/statusColors.js';

// Reactive state using Svelte 5 runes
let categories = $state([]);

/**
 * Initialize store by loading status categories from API
 */
async function init() {
  try {
    categories = await api.statusCategories.getAll() || [];
  } catch (error) {
    console.error('Failed to load status categories:', error);
    categories = [];
  }
}

/**
 * Get color for a category by ID
 * @param {string} categoryId - The category ID to look up
 * @returns {string|null} Hex color or null if not found
 */
function getCategoryColor(categoryId) {
  if (!categoryId) return null;
  const category = categories.find(c => c.id === categoryId);
  return category?.color || null;
}

/**
 * Get complete inline style for a status badge
 * Handles category color lookup and fallback to name-based styling
 * @param {Object} status - Status object with name and category_id
 * @returns {string} Inline CSS style string
 */
function getStatusStyle(status) {
  if (!status) {
    return 'background-color: var(--ds-status-neutral-bg); color: var(--ds-status-neutral-text);';
  }

  // Try category color first
  const color = getCategoryColor(status.category_id);
  if (color) {
    const textColor = getTextColorForBackground(color);
    return `background-color: ${color}; color: ${textColor};`;
  }

  // Fallback to name-based design system colors
  const statusType = getStatusType(status.name);
  return `background-color: var(--ds-status-${statusType}-bg); color: var(--ds-status-${statusType}-text);`;
}

/**
 * Reset store (useful for testing or logout)
 */
function reset() {
  categories = [];
}

// Export the store object with reactive getters
export const statusCategoriesStore = {
  get categories() { return categories; },
  init,
  getCategoryColor,
  getStatusStyle,
  reset
};

// Direct exports for convenience
export { getCategoryColor, getStatusStyle };
