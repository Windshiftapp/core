import { createCalendar, parseDate, toCalendar } from '@internationalized/date';

function key(date) {
  return [
    date.getFullYear(),
    String(date.getMonth() + 1).padStart(2, '0'),
    String(date.getDate()).padStart(2, '0'),
  ].join('-');
}
export function calendarDate(date, calendar) {
  return toCalendar(parseDate(key(date)), createCalendar(calendar));
}
export function calendarMonthStart(date, calendar, offset = 0) {
  const [year, month, day] = calendarDate(date, calendar)
    .set({ day: 1 })
    .add({ months: offset })
    .toString()
    .split('-')
    .map(Number);
  return new Date(year, month - 1, day);
}
export function civilDays(a, b) {
  const utc = (date) => Date.UTC(date.getFullYear(), date.getMonth(), date.getDate());
  return (utc(a) - utc(b)) / 86400000;
}
export function calendarMonthPosition(reference, date, calendar) {
  const ref = calendarDate(reference, calendar),
    current = calendarDate(date, calendar);
  let offset = current.month - ref.month;
  for (let year = ref.year; year < current.year; year++)
    offset += ref.calendar.getMonthsInYear(ref.set({ year }));
  for (let year = current.year; year < ref.year; year++)
    offset -= ref.calendar.getMonthsInYear(ref.set({ year }));
  const first = calendarMonthStart(date, calendar),
    next = calendarMonthStart(date, calendar, 1);
  return offset + civilDays(date, first) / civilDays(next, first);
}
export function calendarDateAtPosition(reference, position, calendar, snapDays = 1) {
  const offset = Math.floor(position),
    date = calendarMonthStart(reference, calendar, offset),
    next = calendarMonthStart(date, calendar, 1);
  date.setDate(
    date.getDate() +
      Math.floor(((position - offset) * civilDays(next, date) + 1e-8) / snapDays) * snapDays
  );
  return date;
}
