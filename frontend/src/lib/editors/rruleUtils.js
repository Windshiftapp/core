/**
 * RRULE utilities for parsing, formatting, and building RRULE strings.
 *
 * RRULE format: "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE,FR"
 */

const DAY_NAMES = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'];
const DAY_LABELS = {
  MO: 'Mon',
  TU: 'Tue',
  WE: 'Wed',
  TH: 'Thu',
  FR: 'Fri',
  SA: 'Sat',
  SU: 'Sun',
};

const FREQ_LABELS = {
  DAILY: 'Daily',
  WEEKLY: 'Weekly',
  MONTHLY: 'Monthly',
  YEARLY: 'Yearly',
};

/**
 * Parse an RRULE string into a form-friendly state object.
 * @param {string} rrule - e.g. "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE,FR;COUNT=10"
 * @returns {object} form state
 */
export function parseRRule(rrule) {
  const state = {
    frequency: 'WEEKLY',
    interval: 1,
    byDay: [],
    byMonthDay: null,
    endType: 'never', // 'never' | 'date' | 'count'
    endDate: '',
    count: null,
  };

  if (!rrule) return state;

  const parts = rrule.split(';');
  for (const part of parts) {
    const [key, value] = part.split('=');
    switch (key) {
      case 'FREQ':
        state.frequency = value;
        break;
      case 'INTERVAL':
        state.interval = parseInt(value, 10) || 1;
        break;
      case 'BYDAY':
        state.byDay = value.split(',');
        break;
      case 'BYMONTHDAY':
        state.byMonthDay = parseInt(value, 10);
        break;
      case 'COUNT':
        state.count = parseInt(value, 10);
        state.endType = 'count';
        break;
      case 'UNTIL':
        // UNTIL can be either a date (YYYYMMDD, length 8) or a datetime
        // (YYYYMMDDTHHMMSS or YYYYMMDDTHHMMSSZ, length 15 or 16). The previous
        // code only handled the date form — datetime values were stuffed
        // verbatim into endDate, which an <input type="date"> can't render, so
        // the radio group showed "On date" with an empty input and re-saving
        // silently dropped the UNTIL clause. Both forms now extract the date
        // portion since the editor only exposes day-granularity end dates.
        if (value.length === 8 || value.length === 15 || value.length === 16) {
          state.endDate = `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
        } else {
          state.endDate = value;
        }
        state.endType = 'date';
        break;
    }
  }

  return state;
}

/**
 * Build an RRULE string from form state.
 * @param {object} state - form state from parseRRule
 * @returns {string} RRULE string
 */
export function buildRRule(state) {
  const parts = [`FREQ=${state.frequency}`];

  if (state.interval && state.interval > 1) {
    parts.push(`INTERVAL=${state.interval}`);
  }

  if (state.frequency === 'WEEKLY' && state.byDay?.length > 0) {
    // Sort days in standard order
    const sorted = [...state.byDay].sort((a, b) => DAY_NAMES.indexOf(a) - DAY_NAMES.indexOf(b));
    parts.push(`BYDAY=${sorted.join(',')}`);
  }

  if (state.frequency === 'MONTHLY' && state.byMonthDay) {
    parts.push(`BYMONTHDAY=${state.byMonthDay}`);
  }

  if (state.endType === 'count' && state.count) {
    parts.push(`COUNT=${state.count}`);
  } else if (state.endType === 'date' && state.endDate) {
    // Convert YYYY-MM-DD to YYYYMMDD format
    const cleaned = state.endDate.replace(/-/g, '');
    parts.push(`UNTIL=${cleaned}`);
  }

  return parts.join(';');
}

/**
 * Convert an RRULE string to a human-readable description.
 * @param {string} rrule
 * @returns {string}
 */
export function rruleToText(rrule) {
  if (!rrule) return '';

  const state = parseRRule(rrule);
  const parts = [];

  // Frequency + interval
  const interval = state.interval || 1;
  switch (state.frequency) {
    case 'DAILY':
      parts.push(interval === 1 ? 'Daily' : `Every ${interval} days`);
      break;
    case 'WEEKLY':
      parts.push(interval === 1 ? 'Weekly' : `Every ${interval} weeks`);
      break;
    case 'MONTHLY':
      parts.push(interval === 1 ? 'Monthly' : `Every ${interval} months`);
      break;
    case 'YEARLY':
      parts.push(interval === 1 ? 'Yearly' : `Every ${interval} years`);
      break;
    default:
      parts.push(state.frequency);
  }

  // Day-of-week for weekly
  if (state.frequency === 'WEEKLY' && state.byDay?.length > 0) {
    const dayLabels = state.byDay.map((d) => DAY_LABELS[d] || d);
    parts.push(`on ${dayLabels.join(', ')}`);
  }

  // Day-of-month for monthly
  if (state.frequency === 'MONTHLY' && state.byMonthDay) {
    parts.push(`on the ${ordinal(state.byMonthDay)}`);
  }

  // End condition
  if (state.endType === 'count' && state.count) {
    parts.push(`for ${state.count} occurrences`);
  } else if (state.endType === 'date' && state.endDate) {
    parts.push(`until ${state.endDate}`);
  }

  return parts.join(' ');
}

/**
 * Get ordinal suffix for a number (1st, 2nd, 3rd, etc.)
 */
function ordinal(n) {
  const s = ['th', 'st', 'nd', 'rd'];
  const v = n % 100;
  return n + (s[(v - 20) % 10] || s[v] || s[0]);
}

export { DAY_LABELS, DAY_NAMES, FREQ_LABELS };
