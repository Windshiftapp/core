/**
 * Numeric date/time layouts per locale.
 *
 * Backend-rendered email (internal/i18n) cannot use the browser's Intl, and Go
 * has no locale-aware date formatter, so each supported locale declares its
 * numeric layout here. Layouts are Go reference-time strings
 * (https://pkg.go.dev/time#pkg-constants) and are consumed verbatim by
 * scripts/generate-i18n-catalog.mjs, which emits
 * internal/i18n/date_formats_generated.go.
 *
 * Deliberately numeric: no month or weekday names, so no CLDR symbol tables and
 * no extra dependency. Times use the 24-hour clock where the locale does.
 *
 * When a locale is added under src/lib/locales/<locale>/, add its entry here.
 * frontend/scripts/check-i18n.js fails the build otherwise.
 */

/** @type {Record<string, { date: string, dateTime: string, time: string }>} */
export const DATE_FORMATS = {
  en: { date: '1/2/2006', dateTime: '1/2/2006, 3:04 PM', time: '3:04 PM' },
  de: { date: '02.01.2006', dateTime: '02.01.2006, 15:04', time: '15:04' },
  es: { date: '2/1/2006', dateTime: '2/1/2006, 15:04', time: '15:04' },
  ar: { date: '2/1/2006', dateTime: '2/1/2006, 15:04', time: '15:04' },
  'pt-BR': { date: '2/1/2006', dateTime: '2/1/2006, 15:04', time: '15:04' },
  ru: { date: '02.01.2006', dateTime: '02.01.2006, 15:04', time: '15:04' },
  'zh-CN': { date: '2006/1/2', dateTime: '2006/1/2 15:04', time: '15:04' },
  fr: { date: '02/01/2006', dateTime: '02/01/2006 15:04', time: '15:04' },
  ko: { date: '2006. 1. 2.', dateTime: '2006. 1. 2. 15:04', time: '15:04' },
  it: { date: '02/01/2006', dateTime: '02/01/2006, 15:04', time: '15:04' },
};

/** Keys every locale entry must define. */
export const DATE_FORMAT_KEYS = ['date', 'dateTime', 'time'];
