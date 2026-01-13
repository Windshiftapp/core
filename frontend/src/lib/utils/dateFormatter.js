/**
 * Date formatting utilities
 * Centralized date formatting functions to avoid duplication across components
 */

/**
 * Format a date string to YYYY-MM-DD format
 * @param {string|Date} dateString - Date string or Date object to format
 * @returns {string} Formatted date in YYYY-MM-DD format, or empty string if invalid
 */
export function formatDate(dateString) {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    return date.toISOString().split('T')[0];
  } catch (error) {
    console.error('Error formatting date:', error);
    return '';
  }
}

/**
 * Format a date string to include time (YYYY-MM-DD HH:MM:SS)
 * @param {string|Date} dateString - Date string or Date object to format
 * @returns {string} Formatted date with time, or empty string if invalid
 */
export function formatDateTime(dateString) {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    return date.toISOString().replace('T', ' ').split('.')[0];
  } catch (error) {
    console.error('Error formatting datetime:', error);
    return '';
  }
}

/**
 * Format a date string using locale-specific formatting
 * @param {string|Date} dateString - Date string or Date object to format
 * @param {object} options - Intl.DateTimeFormat options
 * @returns {string} Formatted date string, or empty string if invalid
 */
export function formatDateLocale(dateString, options = {}) {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    const defaultOptions = {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      ...options
    };
    return date.toLocaleDateString(undefined, defaultOptions);
  } catch (error) {
    console.error('Error formatting date with locale:', error);
    return '';
  }
}

/**
 * Format a date string to a short format (e.g., "Jan 15, 2024")
 * @param {string|Date} dateString - Date string or Date object to format
 * @returns {string} Formatted date string, or empty string if invalid
 */
export function formatDateShort(dateString) {
  return formatDateLocale(dateString, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  });
}

/**
 * Format a date string to a long format (e.g., "January 15, 2024")
 * @param {string|Date} dateString - Date string or Date object to format
 * @returns {string} Formatted date string, or empty string if invalid
 */
export function formatDateLong(dateString) {
  return formatDateLocale(dateString, {
    year: 'numeric',
    month: 'long',
    day: 'numeric'
  });
}

/**
 * Format a date string to include time in locale format
 * @param {string|Date} dateString - Date string or Date object to format
 * @returns {string} Formatted date with time, or empty string if invalid
 */
export function formatDateTimeLocale(dateString) {
  return formatDateLocale(dateString, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  });
}

/**
 * Get a relative time string (e.g., "2 hours ago", "in 3 days")
 * @param {string|Date} dateString - Date string or Date object
 * @returns {string} Relative time string, or empty string if invalid
 */
export function formatRelativeTime(dateString) {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    const now = new Date();
    const diffMs = now - date;
    const diffSecs = Math.floor(diffMs / 1000);
    const diffMins = Math.floor(diffSecs / 60);
    const diffHours = Math.floor(diffMins / 60);
    const diffDays = Math.floor(diffHours / 24);

    if (diffSecs < 60) return 'just now';
    if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;
    if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
    if (diffDays < 7) return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;
    if (diffDays < 30) {
      const weeks = Math.floor(diffDays / 7);
      return `${weeks} week${weeks !== 1 ? 's' : ''} ago`;
    }
    if (diffDays < 365) {
      const months = Math.floor(diffDays / 30);
      return `${months} month${months !== 1 ? 's' : ''} ago`;
    }
    const years = Math.floor(diffDays / 365);
    return `${years} year${years !== 1 ? 's' : ''} ago`;
  } catch (error) {
    console.error('Error formatting relative time:', error);
    return '';
  }
}

/**
 * Check if a date is today
 * @param {string|Date} dateString - Date string or Date object
 * @returns {boolean} True if the date is today
 */
export function isToday(dateString) {
  if (!dateString) return false;
  try {
    const date = new Date(dateString);
    const today = new Date();
    return date.toDateString() === today.toDateString();
  } catch (error) {
    return false;
  }
}

/**
 * Check if a date is in the past
 * @param {string|Date} dateString - Date string or Date object
 * @returns {boolean} True if the date is in the past
 */
export function isPast(dateString) {
  if (!dateString) return false;
  try {
    const date = new Date(dateString);
    const now = new Date();
    return date < now;
  } catch (error) {
    return false;
  }
}

/**
 * Format a date string with timezone awareness
 * @param {string|Date} dateString - Date string or Date object to format
 * @param {string} timezone - IANA timezone string (e.g., "America/New_York") or 'UTC'
 * @param {object} options - Intl.DateTimeFormat options
 * @returns {string} Formatted date string, or empty string if invalid
 */
export function formatDateTimeWithTimezone(dateString, timezone = 'UTC', options = {}) {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    const defaultOptions = {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      timeZone: timezone,
      ...options
    };
    return date.toLocaleString(undefined, defaultOptions);
  } catch (error) {
    console.error('Error formatting date with timezone:', error);
    return '';
  }
}

/**
 * Format a timestamp for display in item history with timezone
 * Displays as "Jan 15, 2025 at 3:45 PM EST" or similar
 * @param {string|Date} dateString - Date string or Date object
 * @param {string} timezone - IANA timezone string or 'UTC'
 * @returns {string} Formatted timestamp with timezone abbreviation
 */
export function formatHistoryTimestamp(dateString, timezone = 'UTC') {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);

    // Format date and time
    const dateOptions = {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      timeZone: timezone
    };
    const timeOptions = {
      hour: 'numeric',
      minute: '2-digit',
      timeZone: timezone
    };

    const datePart = date.toLocaleDateString(undefined, dateOptions);
    const timePart = date.toLocaleTimeString(undefined, timeOptions);

    // Get timezone abbreviation
    const formatter = new Intl.DateTimeFormat(undefined, {
      timeZone: timezone,
      timeZoneName: 'short'
    });
    const parts = formatter.formatToParts(date);
    const timeZonePart = parts.find(part => part.type === 'timeZoneName');
    const tzAbbr = timeZonePart ? timeZonePart.value : '';

    return `${datePart} at ${timePart} ${tzAbbr}`.trim();
  } catch (error) {
    console.error('Error formatting history timestamp:', error);
    return '';
  }
}

/**
 * Get the user's configured timezone from the current user object
 * Falls back to browser timezone, then UTC
 * @param {object} currentUser - Current user object with timezone property
 * @returns {string} IANA timezone string
 */
export function getUserTimezone(currentUser) {
  // Use user's configured timezone if available
  if (currentUser && currentUser.timezone) {
    return currentUser.timezone;
  }

  // Fall back to browser timezone
  try {
    const browserTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    if (browserTimezone) {
      return browserTimezone;
    }
  } catch (error) {
    console.warn('Could not determine browser timezone:', error);
  }

  // Final fallback to UTC
  return 'UTC';
}

/**
 * Format a relative time string in compact format for widgets (e.g., "5m ago", "2h ago", "3d ago")
 * @param {Date|string} date - Date object or date string
 * @returns {string} Compact relative time string
 */
export function formatRelativeCompact(date) {
  if (!date) return 'Unknown';

  const d = date instanceof Date ? date : new Date(date);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const minutes = Math.floor(diffMs / 60000);
  const hours = Math.floor(diffMs / 3600000);
  const days = Math.floor(diffMs / 86400000);

  if (minutes < 1) return 'Just now';
  if (minutes < 60) return `${minutes}m ago`;
  if (hours < 24) return `${hours}h ago`;
  if (days === 1) return 'Yesterday';
  if (days < 7) return `${days}d ago`;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

/**
 * Format a due date for display with contextual text
 * @param {Date|string} dueDate - Due date
 * @returns {string} Formatted due date text (e.g., "Due today", "Overdue by 3 days")
 */
export function formatDueDate(dueDate) {
  if (!dueDate) return 'No due date';

  const d = dueDate instanceof Date ? dueDate : new Date(dueDate);
  const now = new Date();
  const diffMs = d.getTime() - now.getTime();
  const days = Math.round(diffMs / 86400000);

  if (days > 7) return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  if (days > 1) return `Due in ${days} days`;
  if (days === 1) return 'Due tomorrow';
  if (days === 0) return 'Due today';
  if (days === -1) return 'Due yesterday';
  return `Overdue by ${Math.abs(days)} days`;
}

/**
 * Get CSS class for due date badge based on urgency
 * @param {Date|string} dueDate - Due date
 * @returns {string} Tailwind CSS classes for the badge
 */
export function getDueBadgeClass(dueDate) {
  if (!dueDate) return 'bg-gray-100 text-gray-600';

  const d = dueDate instanceof Date ? dueDate : new Date(dueDate);
  const now = new Date();
  const diff = d.getTime() - now.getTime();

  if (diff < 0) return 'bg-rose-100 text-rose-700';
  if (diff <= 2 * 86400000) return 'bg-amber-100 text-amber-700';
  return 'bg-blue-50 text-blue-700';
}

/**
 * Calculate days overdue
 * @param {Date|string} dueDate - Due date
 * @returns {number} Days overdue (negative if not overdue)
 */
export function getDaysOverdue(dueDate) {
  if (!dueDate) return 0;

  const d = dueDate instanceof Date ? dueDate : new Date(dueDate);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  return Math.floor(diffMs / 86400000);
}
