import { getDayOfWeek, parseDate, startOfWeek } from '@internationalized/date';

let preference = $state('auto');
let weekStartPreference = $state('auto');
export const WEEKDAYS = [
  'sunday',
  'monday',
  'tuesday',
  'wednesday',
  'thursday',
  'friday',
  'saturday',
];
export function getWeekStartPreference() {
  return weekStartPreference;
}
export function setWeekStartPreference(value) {
  weekStartPreference = WEEKDAYS.includes(value) ? value : 'auto';
}
export function resolveWeekStart(locale) {
  const explicit = WEEKDAYS.indexOf(weekStartPreference);
  if (explicit !== -1) return explicit;
  return getDayOfWeek(startOfWeek(parseDate('2025-01-05'), locale || 'en'), 'en-US');
}
export function startOfPreferredWeek(date, locale) {
  const start = new Date(date);
  start.setHours(0, 0, 0, 0);
  start.setDate(start.getDate() - ((start.getDay() - resolveWeekStart(locale) + 7) % 7));
  return start;
}

export const CALENDARS = ['auto', 'gregory', 'persian'];
export function getCalendarPreference() {
  return preference;
}
export function setCalendarPreference(value) {
  preference = CALENDARS.includes(value) ? value : 'auto';
}
export function resolveCalendar(locale, override) {
  const selected = override || preference;
  return selected === 'auto'
    ? /^fa(?:-|$)/i.test(locale || '')
      ? 'persian'
      : 'gregory'
    : selected;
}
