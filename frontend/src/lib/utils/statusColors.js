/**
 * Shared utility functions for status category colors and styling
 */

/**
 * Find status category for a given status name
 * @param {string} statusName - The status name to look up
 * @param {Array} statuses - Array of status objects
 * @param {Array} statusCategories - Array of status category objects
 * @returns {Object|null} Status category object or null if not found
 */
export function getStatusCategory(statusName, statuses, statusCategories) {
  if (!statusName || !statuses.length) return null;
  
  // Normalize the input status name
  const normalizedStatusName = statusName.toLowerCase().trim();
  
  // Try different matching strategies
  let status = null;
  
  // 1. Exact match
  status = statuses.find(s => s.name === statusName);
  
  // 2. Case-insensitive exact match
  if (!status) {
    status = statuses.find(s => s.name.toLowerCase() === normalizedStatusName);
  }
  
  // 3. Convert underscores to spaces and match
  if (!status) {
    const withSpaces = statusName.replace(/_/g, ' ');
    status = statuses.find(s => s.name.toLowerCase() === withSpaces.toLowerCase());
  }
  
  // 4. Convert spaces to underscores and match
  if (!status) {
    const withUnderscores = statusName.replace(/ /g, '_');
    status = statuses.find(s => s.name.toLowerCase() === withUnderscores.toLowerCase());
  }
  
  // 5. Title case conversion (to_do -> To Do)
  if (!status) {
    const titleCase = statusName.replace(/_/g, ' ').split(' ')
      .map(word => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
      .join(' ');
    status = statuses.find(s => s.name === titleCase);
  }
  
  if (!status || !status.category_id) {
    return null;
  }
  
  return statusCategories.find(cat => cat.id === status.category_id);
}

/**
 * Get status color styling using official status category colors
 * @param {string} status - The status name
 * @param {Array} statuses - Array of status objects
 * @param {Array} statusCategories - Array of status category objects
 * @returns {string} CSS classes for styling
 */
export function getStatusColor(status, statuses, statusCategories) {
  const category = getStatusCategory(status, statuses, statusCategories);
  
  if (category && category.color) {
    // Use actual category color with dynamic text color
    const textColor = getTextColorForBackground(category.color);
    return `${textColor}`;
  }
  
  // Fallback to hardcoded colors
  const colors = {
    open: 'bg-blue-100 text-blue-800',
    in_progress: 'bg-yellow-100 text-yellow-800', 
    completed: 'bg-green-100 text-green-800',
    cancelled: 'bg-red-100 text-red-800'
  };
  return colors[status] || 'bg-gray-100 text-gray-800';
}

/**
 * Get inline styles for status with official category colors
 * @param {string} status - The status name
 * @param {Array} statuses - Array of status objects
 * @param {Array} statusCategories - Array of status category objects
 * @returns {string} Inline CSS styles
 */
export function getStatusInlineStyle(status, statuses, statusCategories) {
  const category = getStatusCategory(status, statuses, statusCategories);
  
  if (category && category.color) {
    const textColor = getTextColorForBackground(category.color);
    const color = textColor === 'text-gray-800' ? '#1f2937' : '#ffffff';
    return `background-color: ${category.color}; color: ${color};`;
  }
  
  return '';
}

/**
 * Utility function to determine text color based on background brightness
 * @param {string} backgroundColor - Hex color code (with or without #)
 * @returns {string} Tailwind CSS text color class
 */
export function getTextColorForBackground(backgroundColor) {
  if (!backgroundColor) return 'text-gray-800';

  // Remove # if present
  const hex = backgroundColor.replace('#', '');

  // Convert to RGB
  const r = parseInt(hex.substr(0, 2), 16);
  const g = parseInt(hex.substr(2, 2), 16);
  const b = parseInt(hex.substr(4, 2), 16);

  // Calculate luminance using WCAG formula
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;

  // Calculate saturation to distinguish grey colors from saturated colors
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const saturation = max === 0 ? 0 : (max - min) / max;

  // For grey/desaturated colors (low saturation), use lower luminance threshold
  // For saturated colors, use higher luminance threshold
  if (saturation < 0.15) {
    // Grey color - use black text if luminance > 0.4
    return luminance > 0.4 ? 'text-gray-800' : 'text-white';
  } else {
    // Saturated color - use black text only if luminance > 0.65
    return luminance > 0.65 ? 'text-gray-800' : 'text-white';
  }
}