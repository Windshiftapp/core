// Shared helpers for time parsing and synchronization between duration strings and HH:MM values.

/**
 * Parse a duration string like "2h", "30m", "2h30m", "1d", or "3d 4h" (8 hours/day by default).
 * Returns total minutes.
 */
export function parseDuration(durationStr, hoursPerDay = 8) {
  if (!durationStr) return 0;

  const str = durationStr.toLowerCase().trim();
  let totalMinutes = 0;

  const daysMatch = str.match(/(\d+(?:\.\d+)?)\s*d/);
  const hoursMatch = str.match(/(\d+(?:\.\d+)?)\s*h/);
  const minutesMatch = str.match(/(\d+(?:\.\d+)?)\s*m/);

  if (daysMatch) {
    totalMinutes += parseFloat(daysMatch[1]) * hoursPerDay * 60;
  }
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
 * Convert total minutes to a compact duration string.
 * Default: "1h30m" / "30m" / "8h".
 * With `withDays: true`, peels whole days (8h each by default) into a "Xd" prefix,
 * e.g. 1680 -> "3d 4h", 960 -> "2d".
 */
export function durationToString(totalMinutes, options = {}) {
  const { withDays = false, hoursPerDay = 8 } = options;
  const minutes = Math.max(0, Math.round(totalMinutes));

  if (withDays) {
    const minutesPerDay = hoursPerDay * 60;
    const days = Math.floor(minutes / minutesPerDay);
    const remainderMinutes = minutes - days * minutesPerDay;
    const hours = Math.floor(remainderMinutes / 60);
    const mins = remainderMinutes % 60;
    const parts = [];
    if (days > 0) parts.push(`${days}d`);
    if (hours > 0) parts.push(`${hours}h`);
    if (mins > 0) parts.push(`${mins}m`);
    return parts.length === 0 ? '0m' : parts.join(' ');
  }

  const hours = Math.floor(minutes / 60);
  const mins = minutes % 60;

  if (hours === 0) return `${mins}m`;
  if (mins === 0) return `${hours}h`;
  return `${hours}h${mins}m`;
}

/**
 * Compute positive minutes between two HH:MM times; an end at or before the
 * start wraps to the next day (overnight entry). Returns 0 for invalid input.
 */
export function minutesBetweenTimes(startTime, endTime) {
  if (!startTime || !endTime) return 0;

  const [startHours, startMins] = startTime.split(':').map(Number);
  const [endHours, endMins] = endTime.split(':').map(Number);

  const startTotal = startHours * 60 + startMins;
  let endTotal = endHours * 60 + endMins;
  if (endTotal <= startTotal) {
    endTotal += 24 * 60;
  }

  return endTotal - startTotal;
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
