// Worklog civil-date helpers anchored to an explicit reporting timezone.
//
// Worklog labels, filters, and daily totals derive from the stored start/end
// timestamps interpreted in the selected timezone. A worklog that spans local
// midnight is split at each boundary without changing its total minutes; a
// half-open interval means an entry ending exactly at midnight contributes
// nothing to the following day.

import { resolveTimezone } from './dateFormatter.js';

const civilFormatterCache = new Map();

function civilFormatter(timezone) {
  const tz = resolveTimezone(timezone);
  let formatter = civilFormatterCache.get(tz);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: tz,
      hour12: false,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      weekday: 'short',
    });
    civilFormatterCache.set(tz, formatter);
  }
  return formatter;
}

function civilParts(instantMs, timezone) {
  const parts = {};
  for (const part of civilFormatter(timezone).formatToParts(new Date(instantMs))) {
    parts[part.type] = part.value;
  }
  return {
    year: Number(parts.year),
    month: Number(parts.month),
    day: Number(parts.day),
    hour: Number(parts.hour) % 24,
    minute: Number(parts.minute),
    weekday: parts.weekday,
  };
}

function pad(value) {
  return String(value).padStart(2, '0');
}

function civilKey(year, month, day) {
  return `${year}-${pad(month)}-${pad(day)}`;
}

// Offset between the timezone's wall clock and UTC at the given instant (ms).
function zoneOffsetMs(instantMs, timezone) {
  const parts = civilParts(instantMs, timezone);
  const asUTC = Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute);
  return asUTC - Math.floor(instantMs / 60000) * 60000;
}

// UTC instant of a civil wall-clock time in the timezone. Two offset passes
// keep DST boundaries exact for minute-aligned inputs.
function zonedTimeToUtcMs(year, month, day, hour, minute, timezone) {
  const guess = Date.UTC(year, month - 1, day, hour, minute);
  const firstOffset = zoneOffsetMs(guess, timezone);
  const first = guess - firstOffset;
  const secondOffset = zoneOffsetMs(first, timezone);
  return secondOffset === firstOffset ? first : guess - secondOffset;
}

/**
 * Civil date key (YYYY-MM-DD) of a Unix-seconds instant in the timezone.
 * @param {number} unixSeconds
 * @param {string} timezone
 * @returns {string}
 */
export function dateKeyInZone(unixSeconds, timezone) {
  if (!Number.isFinite(unixSeconds)) return '';
  const parts = civilParts(unixSeconds * 1000, timezone);
  return civilKey(parts.year, parts.month, parts.day);
}

/**
 * Wall clock (HH:MM) of a Unix-seconds instant in the timezone.
 * @param {number} unixSeconds
 * @param {string} timezone
 * @returns {string}
 */
export function formatClockInZone(unixSeconds, timezone) {
  if (!Number.isFinite(unixSeconds)) return '';
  const parts = civilParts(unixSeconds * 1000, timezone);
  return `${pad(parts.hour)}:${pad(parts.minute)}`;
}

/**
 * Split a [start, end) interval into minutes per civil day key in the
 * timezone. Entries are minute-aligned in practice; each slice is rounded to
 * the nearest minute and totals always sum back to the elapsed duration.
 * @param {number} startUnix
 * @param {number} endUnix
 * @param {string} timezone
 * @returns {Map<string, number>}
 */
export function splitWorklogMinutesByDay(startUnix, endUnix, timezone) {
  const result = new Map();
  if (!Number.isFinite(startUnix) || !Number.isFinite(endUnix) || endUnix <= startUnix) {
    return result;
  }
  const endMs = endUnix * 1000;
  let cursorMs = startUnix * 1000;
  while (cursorMs < endMs) {
    const parts = civilParts(cursorMs, timezone);
    let boundaryMs = zonedTimeToUtcMs(parts.year, parts.month, parts.day + 1, 0, 0, timezone);
    if (boundaryMs <= cursorMs) {
      boundaryMs = zonedTimeToUtcMs(parts.year, parts.month, parts.day + 2, 0, 0, timezone);
    }
    const sliceEndMs = Math.min(endMs, boundaryMs);
    const minutes = Math.round((sliceEndMs - cursorMs) / 60000);
    if (minutes > 0) {
      const key = civilKey(parts.year, parts.month, parts.day);
      result.set(key, (result.get(key) || 0) + minutes);
    }
    cursorMs = sliceEndMs;
  }
  return result;
}

/**
 * Month bounds (YYYY-MM-DD) of the calendar month containing an instant.
 * @param {string} timezone
 * @param {Date} [now]
 * @returns {{ from: string, to: string }}
 */
export function monthBoundsInZone(timezone, now = new Date()) {
  const parts = civilParts(now.getTime(), timezone);
  const from = civilKey(parts.year, parts.month, 1);
  const lastDay = new Date(Date.UTC(parts.year, parts.month, 0)).getUTCDate();
  const to = civilKey(parts.year, parts.month, lastDay);
  return { from, to };
}

const WEEKDAY_ORDER = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

/**
 * Civil key of the Monday starting the local week of an instant.
 * @param {string} timezone
 * @param {Date} [now]
 * @returns {string}
 */
export function mondayKeyInZone(timezone, now = new Date()) {
  const parts = civilParts(now.getTime(), timezone);
  const weekdayIndex = Math.max(0, WEEKDAY_ORDER.indexOf(parts.weekday));
  return new Date(Date.UTC(parts.year, parts.month - 1, parts.day - weekdayIndex))
    .toISOString()
    .slice(0, 10);
}

/**
 * Shift a civil date key by a whole number of days.
 * @param {string} key
 * @param {number} days
 * @returns {string}
 */
export function addDaysToKey(key, days) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(key);
  if (!match) return key;
  const shifted = new Date(
    Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]) + days)
  );
  return shifted.toISOString().slice(0, 10);
}
