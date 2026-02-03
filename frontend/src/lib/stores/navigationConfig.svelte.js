/**
 * Navigation Configuration Store
 *
 * Centralized configuration for post-creation navigation behavior.
 * Maps view names to whether they should navigate to item detail after creation.
 *
 * Usage:
 *   import { shouldNavigateAfterCreate } from './stores/navigationConfig.svelte.js';
 *   if (shouldNavigateAfterCreate(currentView)) {
 *     navigate('/item/123');
 *   }
 */

// Configuration mapping: view name -> should navigate after creation
// false = stay on current view, true/undefined = navigate to item detail
const viewNavigationConfig = {
  'workspace-board': false, // Stay on board after creating items
  'workspace-backlog': false, // Stay on backlog after creating items
  // Add more view configurations as needed:
  // 'workspace-list': false,
  // 'collection-board': false,
  // 'personal-tasks': false,
};

/**
 * Determines whether the app should navigate to item detail after creation
 * based on the current view.
 *
 * @param {string} viewName - The current route view name (e.g., 'workspace-board')
 * @returns {boolean} - true if should navigate to item detail, false to stay on current view
 */
export function shouldNavigateAfterCreate(viewName) {
  // Default to true (navigate to item) for undefined views
  // This maintains backward compatibility with existing behavior
  return viewNavigationConfig[viewName] ?? true;
}

/**
 * Get the full navigation configuration (for debugging/testing)
 * @returns {Object} The view navigation configuration map
 */
export function getNavigationConfig() {
  return { ...viewNavigationConfig };
}
