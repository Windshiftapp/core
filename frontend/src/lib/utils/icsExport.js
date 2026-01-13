/**
 * ICS Calendar Export Utility
 * Generates RFC 5545 compliant ICS files
 */

/**
 * Escape special characters for ICS format
 */
function escapeICS(text) {
  if (!text) return '';
  return text
    .replace(/\\/g, '\\\\')
    .replace(/;/g, '\\;')
    .replace(/,/g, '\\,')
    .replace(/\n/g, '\\n');
}

/**
 * Format a Date object to ICS datetime format (YYYYMMDDTHHMMSS)
 */
function formatICSDate(date) {
  return date.getFullYear().toString() +
    (date.getMonth() + 1).toString().padStart(2, '0') +
    date.getDate().toString().padStart(2, '0') + 'T' +
    date.getHours().toString().padStart(2, '0') +
    date.getMinutes().toString().padStart(2, '0') + '00';
}

/**
 * Create an ICS event object
 * @param {Object} options
 * @param {string} options.uid - Unique identifier
 * @param {Date} options.start - Start datetime
 * @param {Date} options.end - End datetime
 * @param {string} options.title - Event title
 * @param {string} [options.description] - Event description
 * @param {string} [options.url] - URL link
 * @param {string} [options.location] - Event location
 */
export function createEvent({ uid, start, end, title, description, url, location }) {
  const lines = [
    'BEGIN:VEVENT',
    `UID:${uid}`,
    `DTSTART:${formatICSDate(start)}`,
    `DTEND:${formatICSDate(end)}`,
    `SUMMARY:${escapeICS(title)}`
  ];

  if (description) {
    lines.push(`DESCRIPTION:${escapeICS(description)}`);
  }
  if (url) {
    lines.push(`URL:${url}`);
  }
  if (location) {
    lines.push(`LOCATION:${escapeICS(location)}`);
  }

  lines.push('END:VEVENT');
  return lines.join('\r\n');
}

/**
 * Generate a complete ICS calendar file content
 * @param {Array} events - Array of event strings from createEvent()
 * @param {string} [calendarName] - Optional calendar name
 */
export function generateICSContent(events, calendarName = 'Windshift Calendar') {
  const header = [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'PRODID:-//Windshift//Calendar//EN',
    'CALSCALE:GREGORIAN',
    'METHOD:PUBLISH',
    `X-WR-CALNAME:${escapeICS(calendarName)}`
  ];

  const footer = ['END:VCALENDAR'];

  return [...header, ...events, ...footer].join('\r\n');
}

/**
 * Download ICS content as a file
 * @param {string} content - ICS file content
 * @param {string} filename - Download filename (should end in .ics)
 */
export function downloadICS(content, filename) {
  const blob = new Blob([content], { type: 'text/calendar;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

/**
 * Convert scheduled work items to ICS events and download
 * @param {Array} tasks - Array of scheduled tasks
 * @param {string} baseUrl - Base URL for item links
 * @param {string} filename - Download filename
 */
export function exportTasksToICS(tasks, baseUrl, filename) {
  const events = tasks
    .filter(task => task.scheduledDate && task.scheduledTime)
    .map(task => {
      const [year, month, day] = task.scheduledDate.split('-');
      const [hours, minutes] = task.scheduledTime.split(':');
      const start = new Date(year, month - 1, day, hours, minutes);
      const end = new Date(start.getTime() + (task.durationMinutes || 60) * 60000);

      const itemUrl = `${baseUrl}/workspaces/${task.workspace_id}/items/${task.id}`;
      const description = task.description
        ? `${task.description}\n\nView in Windshift: ${itemUrl}`
        : `View in Windshift: ${itemUrl}`;

      return createEvent({
        uid: `${task.id}-${task.scheduledDate}@windshift`,
        start,
        end,
        title: task.title,
        description,
        url: itemUrl
      });
    });

  const content = generateICSContent(events);
  downloadICS(content, filename);
}
