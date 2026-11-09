let preference = $state('auto');
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
