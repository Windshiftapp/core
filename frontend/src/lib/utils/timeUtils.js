// Shared helpers for time parsing and synchronization between duration strings and HH:MM values.

/**
 * Parse a duration string like "2h", "30m", "2h30m", or "1d" (8 hours by default).
 * Returns total minutes.
 */
export function parseDuration(durationStr, hoursPerDay = 8) {
  if (!durationStr) return 0;

  const str = durationStr.toLowerCase().trim();
  let totalMinutes = 0;

  if (str.endsWith('d')) {
    const days = parseFloat(str.slice(0, -1));
    return days * hoursPerDay * 60;
  }

  const hoursMatch = str.match(/(\d+(?:\.\d+)?)h/);
  const minutesMatch = str.match(/(\d+(?:\.\d+)?)m/);

  if (hoursMatch) {
    totalMinutes += parseFloat(hoursMatch[1]) * 60;
  }
  if (minutesMatch) {
    totalMinutes += parseFloat(minutesMatch[1]);
  }

  return totalMinutes;
}

/**
 * Add minutes to an HH:MM time string and return the resulting HH:MM.
 */
export function addMinutesToTime(timeStr, minutes) {
  if (!timeStr) return '';

  const [hours, mins] = timeStr.split(':').map(Number);
  const date = new Date();
  date.setHours(hours, mins, 0, 0);
  date.setMinutes(date.getMinutes() + minutes);

  return date.toTimeString().slice(0, 5);
}

/**
 * Convert total minutes to a compact duration string (e.g., 90 -> "1h30m", 30 -> "30m").
 */
export function durationToString(totalMinutes) {
  const minutes = Math.max(0, Math.round(totalMinutes));
  const hours = Math.floor(minutes / 60);
  const mins = minutes % 60;

  if (hours === 0) return `${mins}m`;
  if (mins === 0) return `${hours}h`;
  return `${hours}h${mins}m`;
}

/**
 * Compute positive minutes between two HH:MM times; returns 0 if end <= start or inputs are invalid.
 */
export function minutesBetweenTimes(startTime, endTime) {
  if (!startTime || !endTime) return 0;

  const [startHours, startMins] = startTime.split(':').map(Number);
  const [endHours, endMins] = endTime.split(':').map(Number);

  const startTotal = startHours * 60 + startMins;
  const endTotal = endHours * 60 + endMins;

  return endTotal > startTotal ? endTotal - startTotal : 0;
}

/**
 * Provide guarded duration sync helpers to avoid infinite loops when updating start/end/duration.
 */
export function createDurationSync() {
  let isUpdating = false;

  function guard(fn) {
    if (isUpdating) return;
    isUpdating = true;
    try {
      fn();
    } finally {
      isUpdating = false;
    }
  }

  return {
    guard,
    isUpdating: () => isUpdating,
  };
}
