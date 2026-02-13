/**
 * Shared utility functions for status category colors and styling
 */

import { getTextColorForBackground as _getTextColorForBackground } from './colorUtils.js';
import { escapeHtml } from './sanitize.ts';

// Re-export getTextColorForBackground from colorUtils for backward compatibility
export { getTextColorForBackground } from './colorUtils.js';

// Local reference for use in this module
const getTextColorForBackground = _getTextColorForBackground;

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
  status = statuses.find((s) => s.name === statusName);

  // 2. Case-insensitive exact match
  if (!status) {
    status = statuses.find((s) => s.name.toLowerCase() === normalizedStatusName);
  }

  // 3. Convert underscores to spaces and match
  if (!status) {
    const withSpaces = statusName.replace(/_/g, ' ');
    status = statuses.find((s) => s.name.toLowerCase() === withSpaces.toLowerCase());
  }

  // 4. Convert spaces to underscores and match
  if (!status) {
    const withUnderscores = statusName.replace(/ /g, '_');
    status = statuses.find((s) => s.name.toLowerCase() === withUnderscores.toLowerCase());
  }

  // 5. Title case conversion (to_do -> To Do)
  if (!status) {
    const titleCase = statusName
      .replace(/_/g, ' ')
      .split(' ')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
      .join(' ');
    status = statuses.find((s) => s.name === titleCase);
  }

  if (!status || !status.category_id) {
    return null;
  }

  return statusCategories.find((cat) => cat.id === status.category_id);
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
    failed: 'danger',
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
    '#ef4444': 'danger',
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

  const statusObj = statuses.find((s) => s.name?.toLowerCase() === statusName?.toLowerCase());
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

  if (category?.color) {
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

// ============================================
// Test Status Colors (merged from testStatusColors.js)
// ============================================

export const TEST_STATUS = {
  NOT_RUN: 'not_run',
  PASSED: 'passed',
  FAILED: 'failed',
  BLOCKED: 'blocked',
  SKIPPED: 'skipped',
  IN_PROGRESS: 'in_progress',
  COMPLETED: 'completed',
};

/**
 * Get test status badge styles (inline CSS for theme support)
 */
export function getTestStatusBadgeStyle(status) {
  const styles = {
    not_run: {
      background: 'var(--ds-status-neutral-bg)',
      color: 'var(--ds-status-neutral-text)',
      border: 'var(--ds-status-neutral-border)',
    },
    passed: {
      background: 'var(--ds-status-success-bg)',
      color: 'var(--ds-status-success-text)',
      border: 'var(--ds-status-success-border)',
    },
    failed: {
      background: 'var(--ds-status-danger-bg)',
      color: 'var(--ds-status-danger-text)',
      border: 'var(--ds-status-danger-border)',
    },
    blocked: {
      background: 'var(--ds-status-warning-bg)',
      color: 'var(--ds-status-warning-text)',
      border: 'var(--ds-status-warning-border)',
    },
    skipped: {
      background: 'var(--ds-status-neutral-bg)',
      color: 'var(--ds-status-neutral-text)',
      border: 'var(--ds-status-neutral-border)',
    },
    in_progress: {
      background: 'var(--ds-status-info-bg)',
      color: 'var(--ds-status-info-text)',
      border: 'var(--ds-status-info-border)',
    },
    completed: {
      background: 'var(--ds-status-success-bg)',
      color: 'var(--ds-status-success-text)',
      border: 'var(--ds-status-success-border)',
    },
  };
  return styles[status] || styles.not_run;
}

// Alias for backward compatibility
export const getStatusBadgeStyle = getTestStatusBadgeStyle;

/**
 * Get CSS string for test status badge
 */
export function getStatusBadgeCSS(status) {
  const style = getTestStatusBadgeStyle(status);
  return `background-color: ${style.background}; color: ${style.color}; border: 1px solid ${style.border};`;
}

/**
 * Get test status button styles (for Pass/Fail/Blocked/Skip buttons)
 */
export function getStatusButtonStyle(status, isSelected) {
  const colors = {
    passed: {
      active: 'var(--ds-status-success-solid)',
      border: 'var(--ds-status-success-border)',
      text: 'var(--ds-status-success-text)',
    },
    failed: {
      active: 'var(--ds-status-danger-solid)',
      border: 'var(--ds-status-danger-border)',
      text: 'var(--ds-status-danger-text)',
    },
    blocked: {
      active: 'var(--ds-status-warning-solid)',
      border: 'var(--ds-status-warning-border)',
      text: 'var(--ds-status-warning-text)',
    },
    skipped: {
      active: 'var(--ds-status-neutral-solid)',
      border: 'var(--ds-status-neutral-border)',
      text: 'var(--ds-status-neutral-text)',
    },
  };

  const color = colors[status] || colors.skipped;

  if (isSelected) {
    return `background-color: ${color.active}; color: white; border: 1px solid ${color.active};`;
  }
  return `background-color: transparent; color: ${color.text}; border: 1px solid ${color.border};`;
}

/**
 * Get hover style for test status buttons
 */
export function getStatusButtonHoverStyle(status) {
  const colors = {
    passed: 'var(--ds-status-success-bg)',
    failed: 'var(--ds-status-danger-bg)',
    blocked: 'var(--ds-status-warning-bg)',
    skipped: 'var(--ds-status-neutral-bg)',
  };
  return `background-color: ${colors[status] || colors.skipped};`;
}

/**
 * Get human-readable test status label
 */
export function getStatusLabel(status) {
  const labels = {
    not_run: 'Not Run',
    passed: 'Passed',
    failed: 'Failed',
    blocked: 'Blocked',
    skipped: 'Skipped',
    in_progress: 'In Progress',
    completed: 'Completed',
  };
  return labels[status] || status;
}

/**
 * Generate test status badge HTML for DataTable render functions
 */
export function renderStatusBadge(status) {
  const style = getStatusBadgeCSS(status);
  const label = getStatusLabel(status);
  return `<span class="inline-flex px-2 py-1 text-xs font-semibold rounded-full" style="${style}">${label}</span>`;
}

/**
 * Generate milestone badge HTML for DataTable render functions
 */
export function renderMilestoneBadge(name) {
  if (!name) {
    return `<span style="color: var(--ds-text-subtle);">No milestone</span>`;
  }
  return `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium" style="background-color: var(--ds-status-info-bg); color: var(--ds-status-info-text);">${escapeHtml(name)}</span>`;
}
