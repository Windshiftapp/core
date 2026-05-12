import { afterEach, describe, expect, test, vi } from 'vitest';

// Mock serverClock so every "now" reference in the module under test is
// deterministic. The module reads serverNow() inside the formatter
// functions, so the mock is consulted on each call — perfect for
// time-arithmetic assertions.
vi.mock('./serverClock.js', () => ({
  serverNow: vi.fn(() => new Date('2026-05-12T12:00:00Z')),
}));

// The i18n store falls back to 'en' when locale is unset, but we lock it
// down explicitly so locale-dependent assertions (e.g. month names) are
// stable across CI environments.
vi.mock('../stores/i18n.svelte.js', () => ({
  i18n: { locale: 'en-US' },
}));

import {
  daysUntil,
  formatCustomFieldDate,
  formatDate,
  formatDateShort,
  formatDateSimple,
  formatDateTimeLocale,
  formatDateWithOptions,
  formatDueDate,
  formatHistoryTimestamp,
  formatRelativeCompact,
  formatRelativeTime,
  getDaysOverdue,
  getDueBadgeClass,
  getUserTimezone,
  isToday,
} from './dateFormatter.js';
import { serverNow } from './serverClock.js';

afterEach(() => {
  vi.mocked(serverNow).mockReturnValue(new Date('2026-05-12T12:00:00Z'));
});

describe('formatDate', () => {
  test('returns empty string for falsy input', () => {
    expect(formatDate('')).toBe('');
    expect(formatDate(null)).toBe('');
    expect(formatDate(undefined)).toBe('');
  });

  test('returns YYYY-MM-DD for an ISO timestamp', () => {
    // Note: formatDate uses local-time getters, so the result depends on
    // the runtime timezone. Use a midday UTC string so all common test
    // timezones (UTC, US, EU) land on the same calendar day.
    expect(formatDate('2026-05-12T12:00:00Z')).toBe('2026-05-12');
  });

  test('pads single-digit month and day', () => {
    expect(formatDate('2026-01-05T12:00:00Z')).toBe('2026-01-05');
  });
});

describe('formatCustomFieldDate', () => {
  // Critical invariant: a stored YYYY-MM-DD must render as the same
  // calendar day regardless of the viewer's timezone. This is the bug
  // formatCustomFieldDate exists to prevent (date-only fields drifting
  // by ±1 day when a UTC-stamped date hits a non-UTC client).
  test('returns empty string for falsy input', () => {
    expect(formatCustomFieldDate('')).toBe('');
    expect(formatCustomFieldDate(null)).toBe('');
  });

  test('formats a plain YYYY-MM-DD string', () => {
    // en-US default for 'Jan 15, 2026' is the short month + day + year.
    expect(formatCustomFieldDate('2026-01-15')).toBe('Jan 15, 2026');
  });

  test('preserves the calendar day on midnight-UTC ISO inputs', () => {
    // 2026-01-15T00:00:00Z viewed from anywhere west of UTC would
    // otherwise render as Jan 14 — the helper must clamp to UTC.
    expect(formatCustomFieldDate('2026-01-15T00:00:00Z')).toBe('Jan 15, 2026');
  });

  test('rejects malformed inputs', () => {
    expect(formatCustomFieldDate('not-a-date')).toBe('');
    expect(formatCustomFieldDate('2026/01/15')).toBe('');
  });
});

describe('formatDateShort / formatDateTimeLocale / formatDateSimple / formatDateWithOptions', () => {
  test('formatDateShort returns short month + day + year', () => {
    expect(formatDateShort('2026-05-12T12:00:00Z')).toBe('May 12, 2026');
  });

  test('formatDateTimeLocale includes hours and minutes', () => {
    const result = formatDateTimeLocale('2026-05-12T12:00:00Z');
    expect(result).toMatch(/May 12, 2026/);
    // hour/minute formatting depends on the host locale's 24h vs 12h
    // convention; just assert the digits are present.
    expect(result).toMatch(/\d{1,2}:\d{2}/);
  });

  test('formatDateSimple uses the locale default', () => {
    // en-US default is M/D/YYYY.
    expect(formatDateSimple('2026-05-12T12:00:00Z')).toBe('5/12/2026');
  });

  test('formatDateWithOptions threads custom Intl options', () => {
    expect(formatDateWithOptions('2026-05-12T12:00:00Z', { year: 'numeric', month: 'long' })).toBe(
      'May 2026'
    );
  });

  test('all formatters return empty string for falsy input', () => {
    expect(formatDateShort('')).toBe('');
    expect(formatDateTimeLocale(null)).toBe('');
    expect(formatDateSimple(undefined)).toBe('');
    expect(formatDateWithOptions('')).toBe('');
  });
});

describe('formatRelativeTime', () => {
  test('returns empty string for falsy input', () => {
    expect(formatRelativeTime('')).toBe('');
    expect(formatRelativeTime(null)).toBe('');
  });

  test('"just now" within 60 seconds', () => {
    expect(formatRelativeTime('2026-05-12T11:59:30Z')).toBe('just now');
  });

  test('pluralizes minutes/hours/days correctly', () => {
    expect(formatRelativeTime('2026-05-12T11:59:00Z')).toBe('1 minute ago');
    expect(formatRelativeTime('2026-05-12T11:58:00Z')).toBe('2 minutes ago');
    expect(formatRelativeTime('2026-05-12T11:00:00Z')).toBe('1 hour ago');
    expect(formatRelativeTime('2026-05-12T10:00:00Z')).toBe('2 hours ago');
    expect(formatRelativeTime('2026-05-11T12:00:00Z')).toBe('1 day ago');
    expect(formatRelativeTime('2026-05-10T12:00:00Z')).toBe('2 days ago');
  });

  test('weeks bucket between 7 and 30 days', () => {
    expect(formatRelativeTime('2026-05-05T12:00:00Z')).toBe('1 week ago');
    expect(formatRelativeTime('2026-04-21T12:00:00Z')).toBe('3 weeks ago');
  });

  test('months bucket between 30 and 365 days', () => {
    expect(formatRelativeTime('2026-04-01T12:00:00Z')).toBe('1 month ago');
    expect(formatRelativeTime('2025-12-12T12:00:00Z')).toBe('5 months ago');
  });

  test('years bucket beyond 365 days', () => {
    expect(formatRelativeTime('2025-05-12T12:00:00Z')).toBe('1 year ago');
    expect(formatRelativeTime('2024-05-12T12:00:00Z')).toBe('2 years ago');
  });
});

describe('formatRelativeCompact', () => {
  test('returns Unknown for falsy input', () => {
    expect(formatRelativeCompact(null)).toBe('Unknown');
    expect(formatRelativeCompact('')).toBe('Unknown');
  });

  test('"Just now" within 1 minute, then minute/hour buckets', () => {
    expect(formatRelativeCompact('2026-05-12T11:59:50Z')).toBe('Just now');
    expect(formatRelativeCompact('2026-05-12T11:55:00Z')).toBe('5m ago');
    expect(formatRelativeCompact('2026-05-12T10:00:00Z')).toBe('2h ago');
  });

  test('"Yesterday" for ~1 day, day buckets up to 7 days', () => {
    expect(formatRelativeCompact('2026-05-11T12:00:00Z')).toBe('Yesterday');
    expect(formatRelativeCompact('2026-05-09T12:00:00Z')).toBe('3d ago');
  });

  test('falls back to short date past 7 days', () => {
    expect(formatRelativeCompact('2026-04-01T12:00:00Z')).toBe('Apr 1');
  });
});

describe('formatDueDate', () => {
  test('returns "No due date" for falsy input', () => {
    expect(formatDueDate(null)).toBe('No due date');
    expect(formatDueDate('')).toBe('No due date');
  });

  test('today / tomorrow / yesterday wording', () => {
    expect(formatDueDate('2026-05-12T12:00:00Z')).toBe('Due today');
    expect(formatDueDate('2026-05-13T12:00:00Z')).toBe('Due tomorrow');
    expect(formatDueDate('2026-05-11T12:00:00Z')).toBe('Due yesterday');
  });

  test('"Due in N days" for 2-7 days out', () => {
    expect(formatDueDate('2026-05-15T12:00:00Z')).toBe('Due in 3 days');
    expect(formatDueDate('2026-05-19T12:00:00Z')).toBe('Due in 7 days');
  });

  test('falls back to a short date past 7 days', () => {
    expect(formatDueDate('2026-06-01T12:00:00Z')).toBe('Jun 1');
  });

  test('"Overdue by N days" past yesterday', () => {
    expect(formatDueDate('2026-05-09T12:00:00Z')).toBe('Overdue by 3 days');
  });
});

describe('getDueBadgeClass', () => {
  test('neutral palette for no due date', () => {
    expect(getDueBadgeClass(null)).toContain('gray');
    expect(getDueBadgeClass('')).toContain('gray');
  });

  test('rose palette when overdue', () => {
    expect(getDueBadgeClass('2026-05-11T12:00:00Z')).toContain('rose');
  });

  test('amber palette when due within 2 days', () => {
    expect(getDueBadgeClass('2026-05-13T12:00:00Z')).toContain('amber');
    expect(getDueBadgeClass('2026-05-12T12:00:00Z')).toContain('amber');
  });

  test('blue palette when due further out', () => {
    expect(getDueBadgeClass('2026-06-12T12:00:00Z')).toContain('blue');
  });
});

describe('daysUntil', () => {
  const labels = {
    overdue: (n) => `${n} overdue`,
    today: 'today',
    oneDay: 'tomorrow',
    remaining: (n) => `${n} left`,
  };

  test('returns null when targetDate is falsy', () => {
    expect(daysUntil(null, labels)).toBeNull();
    expect(daysUntil('', labels)).toBeNull();
  });

  test('classifies overdue / today / tomorrow / remaining', () => {
    expect(daysUntil('2026-05-12T12:00:00Z', labels)).toEqual({
      text: 'today',
      overdue: false,
    });
    expect(daysUntil('2026-05-13T12:00:00Z', labels)).toEqual({
      text: 'tomorrow',
      overdue: false,
    });
    expect(daysUntil('2026-05-17T12:00:00Z', labels)).toEqual({
      text: '5 left',
      overdue: false,
    });
    expect(daysUntil('2026-05-10T12:00:00Z', labels)).toEqual({
      text: '2 overdue',
      overdue: true,
    });
  });
});

describe('getDaysOverdue', () => {
  test('returns 0 for falsy input', () => {
    expect(getDaysOverdue(null)).toBe(0);
    expect(getDaysOverdue('')).toBe(0);
  });

  test('returns floor((now - due) / day)', () => {
    expect(getDaysOverdue('2026-05-09T12:00:00Z')).toBe(3);
    expect(getDaysOverdue('2026-05-12T11:00:00Z')).toBe(0); // 1h overdue → 0 full days
    expect(getDaysOverdue('2026-05-15T12:00:00Z')).toBe(-3);
  });
});

describe('isToday', () => {
  test('true for the same calendar day as serverNow', () => {
    expect(isToday('2026-05-12T03:00:00Z')).toBe(true);
  });

  test('false for a different day', () => {
    expect(isToday('2026-05-11T12:00:00Z')).toBe(false);
    expect(isToday('2026-05-13T12:00:00Z')).toBe(false);
  });

  test('returns false on bad input rather than throwing', () => {
    expect(isToday('')).toBe(false);
    expect(isToday(null)).toBe(false);
  });
});

describe('getUserTimezone', () => {
  test('prefers currentUser.timezone when set', () => {
    expect(getUserTimezone({ timezone: 'Europe/Berlin' })).toBe('Europe/Berlin');
  });

  test('falls back to the browser timezone when user has no timezone', () => {
    const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    expect(getUserTimezone({})).toBe(browserTz);
    expect(getUserTimezone(null)).toBe(browserTz);
  });

  test('returns UTC when both user and browser fail', () => {
    // Force the browser-timezone path to throw so the UTC fallback fires.
    const original = Intl.DateTimeFormat;
    vi.stubGlobal('Intl', {
      ...Intl,
      DateTimeFormat: () => {
        throw new Error('no timezone');
      },
    });
    try {
      expect(getUserTimezone({})).toBe('UTC');
    } finally {
      vi.stubGlobal('Intl', { ...Intl, DateTimeFormat: original });
    }
  });
});

describe('formatHistoryTimestamp', () => {
  test('returns empty string for falsy input', () => {
    expect(formatHistoryTimestamp(null)).toBe('');
    expect(formatHistoryTimestamp('')).toBe('');
  });

  test('formats date + time + tz abbreviation', () => {
    // Output composition: "Mon Day, Year at H:MM AM/PM TZ"
    const result = formatHistoryTimestamp('2026-05-12T15:30:00Z', 'UTC');
    expect(result).toMatch(/May 12, 2026/);
    expect(result).toMatch(/at\s+3:30/);
    expect(result).toMatch(/UTC$/);
  });

  test('renders timestamps in the supplied IANA zone', () => {
    // 15:30 UTC is 11:30 EDT on this date — verify the zone shift lands.
    const result = formatHistoryTimestamp('2026-05-12T15:30:00Z', 'America/New_York');
    expect(result).toMatch(/11:30/);
    // Timezone abbreviation may render as "EDT" or "GMT-4" depending on
    // the host's CLDR data; accept either.
    expect(result).toMatch(/(EDT|GMT[+-]\d)/);
  });
});
