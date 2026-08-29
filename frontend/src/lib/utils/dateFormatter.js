/**
 * Date formatting utilities
 * Centralized date formatting functions to avoid duplication across components
 */

import { i18n, t } from '../stores/i18n.svelte.js';
import { serverNow } from './serverClock.js';

export const DEFAULT_TIMEZONE = 'UTC';

/**
 * Get the app's current locale for date formatting.
 * Falls back to 'en' if i18n hasn't initialized yet.
 * @returns {string} Locale code (e.g., 'en', 'de', 'es')
 */
function getAppLocale() {
  return i18n.locale || 'en';
}

/**
 * Validate an IANA timezone and fall back to UTC for missing or legacy values.
 * Request and schedule validation remains a backend responsibility.
 * @param {string|null|undefined} timezone
 * @returns {string}
 */
export function resolveTimezone(timezone) {
  const candidate = typeof timezone === 'string' ? timezone.trim() : '';
  if (!candidate) return DEFAULT_TIMEZONE;
  if (candidate === 'Local' || /^[+-]\d{2}(?::?\d{2})?$/.test(candidate)) {
    return DEFAULT_TIMEZONE;
  }

  try {
    new Intl.DateTimeFormat('en-US', { timeZone: candidate }).format();
    return candidate;
  } catch (_error) {
    return DEFAULT_TIMEZONE;
  }
}

/**
 * Format an instant in an explicit, validated timezone.
 * @param {string|number|Date} value
 * @param {string} timezone
 * @param {Intl.DateTimeFormatOptions} options
 * @returns {string}
 */
export function formatInstant(value, timezone = DEFAULT_TIMEZONE, options = {}) {
  if (value === null || value === undefined || value === '') return '';

  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return '';

  try {
    return new Intl.DateTimeFormat(getAppLocale(), {
      ...options,
      timeZone: resolveTimezone(timezone),
    }).format(date);
  } catch (_error) {
    return '';
  }
}

/**
 * Format a calendar date without applying a user or browser timezone.
 * @param {string|Date} value
 * @param {Intl.DateTimeFormatOptions} options
 * @returns {string}
 */
export function formatDateOnly(value, options = {}) {
  if (!value) return '';

  let ymd;
  if (typeof value === 'string') {
    ymd = value.slice(0, 10);
  } else if (value instanceof Date && !Number.isNaN(value.getTime())) {
    ymd = value.toISOString().slice(0, 10);
  } else {
    return '';
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(ymd)) return '';

  const [year, month, day] = ymd.split('-').map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (Number.isNaN(date.getTime()) || date.toISOString().slice(0, 10) !== ymd) return '';

  try {
    /** @type {Intl.DateTimeFormatOptions} */
    const displayOptions =
      Object.keys(options).length > 0
        ? options
        : { year: 'numeric', month: 'short', day: 'numeric' };
    return new Intl.DateTimeFormat(getAppLocale(), {
      ...displayOptions,
      timeZone: DEFAULT_TIMEZONE,
    }).format(date);
  } catch (_error) {
    return '';
  }
}

/**
 * Return the stored calendar key without applying a timezone.
 * @param {string|Date} value
 * @returns {string}
 */
export function dateOnlyKey(value) {
  if (typeof value === 'string') {
    const key = value.slice(0, 10);
    return /^\d{4}-\d{2}-\d{2}$/.test(key) ? key : '';
  }
  if (value instanceof Date && !Number.isNaN(value.getTime())) {
    return value.toISOString().slice(0, 10);
  }
  return '';
}

/**
 * Decode a persisted UTC-midnight worklog date key.
 * @param {number} epochSeconds
 * @returns {string}
 */
export function worklogDateKey(epochSeconds) {
  if (!Number.isFinite(epochSeconds)) return '';
  return new Date(epochSeconds * 1000).toISOString().slice(0, 10);
}

function civilDayNumber(value) {
  const key = dateOnlyKey(value);
  if (!key) return null;
  const [year, month, day] = key.split('-').map(Number);
  return Date.UTC(year, month - 1, day) / 86400000;
}

function dueDayOffset(dueDate) {
  const due = civilDayNumber(dueDate);
  const today = civilDayNumber(formatDate(serverNow()));
  return due === null || today === null ? null : due - today;
}

/**
 * Bind a validated display timezone for repeated formatting.
 * @param {string|null|undefined} timezone
 */
export function createTemporalFormatter(timezone) {
  const resolvedTimezone = resolveTimezone(timezone);
  return {
    timezone: resolvedTimezone,
    formatInstant: (value, options = {}) => formatInstant(value, resolvedTimezone, options),
    formatDateOnly,
  };
}

/**
 * Convert an HTML date input value to the UTC timestamp expected by timestamp APIs.
 * @param {string|null|undefined} value - Date in YYYY-MM-DD format
 * @returns {string|null} RFC 3339 timestamp at midnight UTC, or null when empty
 */
export function dateInputToISOString(value) {
  if (!value) return null;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    throw new RangeError('Date input must use YYYY-MM-DD format');
  }

  const date = new Date(`${value}T00:00:00.000Z`);
  if (Number.isNaN(date.getTime()) || date.toISOString().slice(0, 10) !== value) {
    throw new RangeError('Date input must be a valid calendar date');
  }
  return date.toISOString();
}

/**
 * Format a date string to YYYY-MM-DD format
 * @param {string|Date} dateString - Date string or Date object to format
 * @returns {string} Formatted date in YYYY-MM-DD format, or empty string if invalid
 */
export function formatDate(dateString) {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
  } catch (error) {
    console.error('Error formatting date:', error);
    return '';
  }
}

/**
 * Format a date string using locale-specific formatting
 * @param {string|Date} dateString - Date string or Date object to format
 * @param {object} options - Intl.DateTimeFormat options
 * @returns {string} Formatted date string, or empty string if invalid
 */
function formatDateLocale(dateString, options = {}) {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    const defaultOptions = {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      ...options,
    };
    return date.toLocaleDateString(getAppLocale(), defaultOptions);
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
    day: 'numeric',
  });
}

/**
 * Format a date-only string without timezone drift.
 * Expects "YYYY-MM-DD" or an ISO timestamp; either way, the calendar day is
 * preserved (parsed + rendered in UTC) so a stored "2026-01-15" never
 * displays as the 14th or 16th depending on the viewer's timezone.
 * @param {string|Date} dateString
 * @returns {string}
 */
export function formatCustomFieldDate(dateString) {
  return formatDateOnly(dateString);
}

/**
 * Format a date string to include time in locale format
 * @param {string|Date} dateString - Date string or Date object to format
 * @param {string} timezone - Validated user or explicit public timezone
 * @returns {string} Formatted date with time, or empty string if invalid
 */
export function formatDateTimeLocale(dateString, timezone = DEFAULT_TIMEZONE) {
  return formatInstant(dateString, timezone, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
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
    if (Number.isNaN(date.getTime())) return '';
    const now = serverNow();
    const diffMs = date.getTime() - now.getTime();
    const absoluteMs = Math.abs(diffMs);
    const formatter = new Intl.RelativeTimeFormat(getAppLocale(), { numeric: 'always' });
    const immediateFormatter = new Intl.RelativeTimeFormat(getAppLocale(), { numeric: 'auto' });
    const valueFor = (unitMs) => Math.sign(diffMs) * Math.floor(absoluteMs / unitMs);

    if (absoluteMs < 60_000) return immediateFormatter.format(0, 'second');
    if (absoluteMs < 3_600_000) return formatter.format(valueFor(60_000), 'minute');
    if (absoluteMs < 86_400_000) return formatter.format(valueFor(3_600_000), 'hour');
    if (absoluteMs < 604_800_000) return formatter.format(valueFor(86_400_000), 'day');
    if (absoluteMs < 2_592_000_000) return formatter.format(valueFor(604_800_000), 'week');
    if (absoluteMs < 31_536_000_000) return formatter.format(valueFor(2_592_000_000), 'month');
    return formatter.format(valueFor(31_536_000_000), 'year');
  } catch (error) {
    console.error('Error formatting relative time:', error);
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
export function formatHistoryTimestamp(dateString, timezone = DEFAULT_TIMEZONE) {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    const resolvedTimezone = resolveTimezone(timezone);

    // Format date and time
    const dateOptions = {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      timeZone: resolvedTimezone,
    };
    const timeOptions = {
      hour: 'numeric',
      minute: '2-digit',
      timeZone: resolvedTimezone,
    };

    const datePart = date.toLocaleDateString(getAppLocale(), /** @type {any} */ (dateOptions));
    const timePart = date.toLocaleTimeString(getAppLocale(), /** @type {any} */ (timeOptions));

    // Get timezone abbreviation
    const formatter = new Intl.DateTimeFormat(getAppLocale(), {
      timeZone: resolvedTimezone,
      timeZoneName: 'short',
    });
    const parts = formatter.formatToParts(date);
    const timeZonePart = parts.find((part) => part.type === 'timeZoneName');
    const tzAbbr = timeZonePart ? timeZonePart.value : '';

    return `${datePart} at ${timePart} ${tzAbbr}`.trim();
  } catch (error) {
    console.error('Error formatting history timestamp:', error);
    return '';
  }
}

/**
 * Get the user's configured timezone from the current user object
 * Falls back to UTC for missing or invalid stored values.
 * @param {object} currentUser - Current user object with timezone property
 * @returns {string} IANA timezone string
 */
export function getUserTimezone(currentUser) {
  return resolveTimezone(currentUser?.timezone);
}

/**
 * Format a relative time string in compact format for widgets (e.g., "5m ago", "2h ago", "3d ago")
 * @param {Date|string} date - Date object or date string
 * @returns {string} Compact relative time string
 */
export function formatRelativeCompact(date) {
  if (!date) return 'Unknown';

  const d = date instanceof Date ? date : new Date(date);
  const now = serverNow();
  const diffMs = now.getTime() - d.getTime();
  const minutes = Math.floor(diffMs / 60000);
  const hours = Math.floor(diffMs / 3600000);
  const days = Math.floor(diffMs / 86400000);

  if (minutes < 1) return 'Just now';
  if (minutes < 60) return `${minutes}m ago`;
  if (hours < 24) return `${hours}h ago`;
  if (days === 1) return 'Yesterday';
  if (days < 7) return `${days}d ago`;
  return d.toLocaleDateString(getAppLocale(), { month: 'short', day: 'numeric' });
}

/**
 * Format how long something has been in a state as a compact age label.
 * Unlike formatRelativeCompact this expresses an elapsed duration ("3d", "2w")
 * rather than a point in time ("3d ago"), suitable for "time in status" chips.
 * @param {Date|string} since - Timestamp the state was entered
 * @returns {string|null} Compact age (e.g. "now", "5m", "3h", "4d", "2w"), or null
 */
export function formatStatusAge(since) {
  if (!since) return null;

  const d = since instanceof Date ? since : new Date(since);
  if (Number.isNaN(d.getTime())) return null;

  const diffMs = serverNow().getTime() - d.getTime();
  if (diffMs < 60000) return 'now';

  const minutes = Math.floor(diffMs / 60000);
  const hours = Math.floor(diffMs / 3600000);
  const days = Math.floor(diffMs / 86400000);

  if (minutes < 60) return `${minutes}m`;
  if (hours < 24) return `${hours}h`;
  if (days < 14) return `${days}d`;
  return `${Math.floor(days / 7)}w`;
}

/**
 * Format a due date for display with contextual text
 * @param {Date|string} dueDate - Due date
 * @returns {string} Formatted due date text (e.g., "Due today", "Overdue by 3 days")
 */
export function formatDueDate(dueDate) {
  if (!dueDate) return t('dueDate.noDueDate');

  const days = dueDayOffset(dueDate);
  if (days === null) return '';

  if (days > 7) return formatDateOnly(dueDate, { month: 'short', day: 'numeric' });
  if (days > 1) return t('dueDate.dueInDays', { days });
  if (days === 1) return t('dueDate.dueTomorrow');
  if (days === 0) return t('dueDate.dueToday');
  if (days === -1) return t('dueDate.dueYesterday');
  return t('dueDate.overdueByDays', { days: Math.abs(days) });
}

/**
 * Compact due-date label carrying only the triage order (e.g. "68d", "3w").
 * @param {Date|string} dueDate - Due date
 * @returns {string} Compact day/week count
 */
export function formatDueCompact(dueDate) {
  const offset = dueDayOffset(dueDate);
  if (offset === null) return '';
  const days = Math.abs(offset);
  if (days < 14) return `${days}d`;
  return `${Math.floor(days / 7)}w`;
}

/**
 * Severity bucket for a due date: drives icon and Badge variant.
 * @param {Date|string} dueDate - Due date
 * @returns {'overdue'|'soon'|'later'|null}
 */
export function getDueSeverity(dueDate) {
  if (!dueDate) return null;

  const diff = dueDayOffset(dueDate);
  if (diff === null) return null;

  if (diff < 0) return 'overdue';
  if (diff <= 2) return 'soon';
  return 'later';
}

/**
 * Full-sentence due tooltip text (e.g. "Overdue by 68 days — was due 20 May 2026").
 * @param {Date|string} dueDate - Due date
 * @returns {string} Localised tooltip sentence
 */
export function formatDueTooltip(dueDate) {
  const date = formatDateOnly(dueDate, {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  });
  const severity = getDueSeverity(dueDate);

  if (severity === 'overdue') {
    const days = Math.abs(dueDayOffset(dueDate));
    return t('dueDate.overdueTooltip', { days, date });
  }
  if (severity === 'soon') {
    const days = dueDayOffset(dueDate);
    return t('dueDate.dueSoonTooltip', { days, date });
  }
  return t('dueDate.dueLaterTooltip', { date });
}

/**
 * Format a date using the app locale with no special options.
 * Drop-in replacement for bare `date.toLocaleDateString()` calls in components.
 * @param {string|Date} dateString - Date string or Date object
 * @returns {string} Locale-formatted date string, or empty string if invalid
 */
export function formatDateSimple(dateString) {
  if (!dateString) return '';
  try {
    const date = dateString instanceof Date ? dateString : new Date(dateString);
    return date.toLocaleDateString(getAppLocale());
  } catch (error) {
    console.error('Error formatting date:', error);
    return '';
  }
}

/**
 * Format a date using the app locale with custom Intl.DateTimeFormat options.
 * Generic escape hatch for one-off format patterns in components.
 * @param {string|Date} dateString - Date string or Date object
 * @param {object} options - Intl.DateTimeFormat options
 * @returns {string} Locale-formatted date string, or empty string if invalid
 */
export function formatDateWithOptions(dateString, options = {}) {
  if (!dateString) return '';
  try {
    const date = dateString instanceof Date ? dateString : new Date(dateString);
    return date.toLocaleDateString(getAppLocale(), options);
  } catch (error) {
    console.error('Error formatting date with options:', error);
    return '';
  }
}

/**
 * Calculate days until a target date and return a display object.
 * @param {string|Date} targetDate - The target/end date
 * @param {{ overdue: (n: number) => string, today: string, oneDay: string, remaining: (n: number) => string }} labels
 * @returns {{ text: string, overdue: boolean } | null}
 */
export function daysUntil(targetDate, labels) {
  if (!targetDate) return null;
  const today = serverNow();
  const target = new Date(targetDate);
  const diffTime = target.getTime() - today.getTime();
  const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

  if (diffDays < 0) return { text: labels.overdue(Math.abs(diffDays)), overdue: true };
  if (diffDays === 0) return { text: labels.today, overdue: false };
  if (diffDays === 1) return { text: labels.oneDay, overdue: false };
  return { text: labels.remaining(diffDays), overdue: false };
}

/**
 * Calculate days overdue
 * @param {Date|string} dueDate - Due date
 * @returns {number} Days overdue (negative if not overdue)
 */
export function getDaysOverdue(dueDate) {
  if (!dueDate) return 0;
  const offset = dueDayOffset(dueDate);
  return offset === null || offset === 0 ? 0 : -offset;
}
