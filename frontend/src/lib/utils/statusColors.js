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
 * Map status names to design system status types for fallback styling
 * @param {string} status - The status name
 * @returns {string} Design system status type (info, success, warning, danger, neutral)
 */
export function getStatusType(status) {
  const normalizedStatus = status?.toLowerCase().replace(/[_\s]/g, '') || '';

  const statusTypeMap = {
    // Info/Open states
    open: 'info',
    new: 'info',
    todo: 'info',
    backlog: 'info',

    // Warning/In Progress states
    inprogress: 'warning',
    pending: 'warning',
    review: 'warning',
    inreview: 'warning',
    blocked: 'warning',

    // Success/Completed states
    completed: 'success',
    done: 'success',
    closed: 'success',
    resolved: 'success',
    approved: 'success',
    passed: 'success',

    // Danger/Cancelled states
    cancelled: 'danger',
    canceled: 'danger',
    rejected: 'danger',
    failed: 'danger'
  };

  return statusTypeMap[normalizedStatus] || 'neutral';
}

/**
 * Get inline styles for status using design system colors (simple version)
 * Use this when you don't have access to statuses/statusCategories arrays
 * @param {string} status - The status name
 * @returns {string} Inline CSS styles
 */
export function getStatusStyle(status) {
  const statusType = getStatusType(status);
  return `background-color: var(--ds-status-${statusType}-bg); color: var(--ds-status-${statusType}-text);`;
}

/**
 * Map a category hex color to design system status styles
 * Use this when you have the category_color directly on the status object
 * @param {string} color - Hex color code
 * @returns {string} Inline CSS styles
 */
export function getStatusStyleFromCategoryColor(color) {
  const colorMap = {
    '#6b7280': 'neutral',
    '#3b82f6': 'info',
    '#10b981': 'success',
    '#f59e0b': 'warning',
    '#ef4444': 'danger'
  };

  const statusType = colorMap[color] || 'neutral';
  return `background-color: var(--ds-status-${statusType}-bg); color: var(--ds-status-${statusType}-text);`;
}

/**
 * Get inline styles for status by looking up in a statuses array with category_color
 * Use this when you have a statuses array where each status has category_color directly
 * @param {string} statusName - The status name to look up
 * @param {Array} statuses - Array of status objects with category_color property
 * @returns {string} Inline CSS styles
 */
export function getStatusStyleFromStatuses(statusName, statuses) {
  if (!statusName || !statuses?.length) {
    return getStatusStyle(statusName);
  }

  const statusObj = statuses.find(s => s.name?.toLowerCase() === statusName?.toLowerCase());
  if (!statusObj?.category_color) {
    return getStatusStyle(statusName);
  }

  return getStatusStyleFromCategoryColor(statusObj.category_color);
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
    return `background-color: ${category.color}; color: ${textColor};`;
  }

  // Fallback to design system status colors
  const statusType = getStatusType(status);
  return `background-color: var(--ds-status-${statusType}-bg); color: var(--ds-status-${statusType}-text);`;
}

/**
 * Get status color styling using official status category colors
 * Returns inline styles instead of Tailwind classes for theme compatibility
 * @param {string} status - The status name
 * @param {Array} statuses - Array of status objects
 * @param {Array} statusCategories - Array of status category objects
 * @returns {string} Inline CSS styles
 */
export function getStatusColor(status, statuses, statusCategories) {
  return getStatusInlineStyle(status, statuses, statusCategories);
}

/**
 * Utility function to determine text color based on background brightness
 * @param {string} backgroundColor - Hex color code (with or without #)
 * @returns {string} CSS color value
 */
export function getTextColorForBackground(backgroundColor) {
  if (!backgroundColor) return 'var(--ds-text)';

  // If it's a CSS variable, return appropriate fallback
  if (backgroundColor.startsWith('var(')) {
    return 'var(--ds-text)';
  }

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
    // Grey color - use dark text if luminance > 0.4
    return luminance > 0.4 ? 'var(--ds-text)' : 'var(--ds-text-inverse)';
  } else {
    // Saturated color - use dark text only if luminance > 0.65
    return luminance > 0.65 ? 'var(--ds-text)' : 'var(--ds-text-inverse)';
  }
}
