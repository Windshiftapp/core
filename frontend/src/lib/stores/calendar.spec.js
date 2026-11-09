// @vitest-environment jsdom
import { createCalendar, parseDate, toCalendar } from '@internationalized/date';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { userPreferences } from '../api/users.js';
import Input from '../components/Input.svelte';
import ProfileRegionalSettingsTab from '../profile/ProfileRegionalSettingsTab.svelte';
import { collectionPreferences } from './collectionPreferences.svelte.js';
import { i18n } from './i18n.svelte.js';

vi.mock('../api/users.js', () => ({
  userPreferences: { get: vi.fn(), update: vi.fn() },
  updateUserRegionalSettings: vi.fn(),
}));
vi.mock('./auth.svelte.js', () => ({
  authStore: { currentUser: { id: 7 }, patchCurrentUser: vi.fn() },
}));

import {
  calendarDateAtPosition,
  calendarMonthPosition,
  calendarMonthStart,
  civilDays,
} from '../features/collections/roadmapCalendar.js';
import CalendarDateField from '../pickers/CalendarDateField.svelte';
import {
  getCalendarPreference,
  resolveCalendar,
  setCalendarPreference,
} from './calendar.svelte.js';

afterEach(() => setCalendarPreference('auto'));
describe('calendar preference', () => {
  it('uses locale defaults and explicit account choices independently', () => {
    expect(resolveCalendar('fa-IR')).toBe('persian');
    expect(resolveCalendar('en')).toBe('gregory');
    setCalendarPreference('gregory');
    expect(resolveCalendar('fa')).toBe('gregory');
    expect(resolveCalendar('fa', 'persian')).toBe('persian');
    setCalendarPreference('unknown');
    expect(getCalendarPreference()).toBe('auto');
  });
  it('round trips Nowruz and the leap day', () => {
    for (const [iso, year, month, day] of [
      ['2024-03-20', 1403, 1, 1],
      ['2025-03-20', 1403, 12, 30],
      ['2025-03-21', 1404, 1, 1],
    ]) {
      const date = toCalendar(parseDate(String(iso)), createCalendar('persian'));
      expect([date.year, date.month, date.day]).toEqual([year, month, day]);
      expect(date.toString()).toBe(iso);
    }
  });
});

describe('Persian calendar field', () => {
  afterEach(cleanup);
  it('selects an ISO date from the Persian grid', async () => {
    const onchange = vi.fn();
    render(CalendarDateField, {
      props: {
        value: '2025-03-20',
        calendarId: 'persian',
        locale: 'fa',
        inline: true,
        onchange,
        dataTestid: 'date',
      },
    });
    expect(screen.getByTestId('date-calendar').querySelector('thead th')?.textContent).toBe('شنبه');
    await fireEvent.click(screen.getByTestId('date-calendar-day-2025-03-19'));
    expect(onchange).toHaveBeenCalledWith('2025-03-19');
  });
});

describe('calendar field constraints', () => {
  afterEach(cleanup);
  it('does not commit dates outside the allowed range', async () => {
    const onchange = vi.fn();
    render(CalendarDateField, {
      props: {
        value: '2025-03-20',
        max: '2025-03-18',
        calendarId: 'persian',
        locale: 'fa',
        inline: true,
        onchange,
        dataTestid: 'limited',
      },
    });
    await fireEvent.click(screen.getByTestId('limited-calendar-day-2025-03-19'));
    expect(onchange).not.toHaveBeenCalled();
  });
  it('reflects an externally changed value without firing a change callback', async () => {
    const onchange = vi.fn();
    const view = render(CalendarDateField, {
      props: {
        value: '2025-03-20',
        calendarId: 'persian',
        locale: 'fa',
        inline: true,
        onchange,
        dataTestid: 'controlled',
      },
    });
    await view.rerender({ value: '2025-03-19' });
    expect(
      screen.getByTestId('controlled-calendar-day-2025-03-19').getAttribute('aria-selected')
    ).toBe('true');
    expect(onchange).not.toHaveBeenCalled();
  });
});

describe('calendar form behavior', () => {
  afterEach(cleanup);
  it('participates in required form validation and respects read-only', async () => {
    const view = render(CalendarDateField, {
      props: { value: '', calendarId: 'persian', locale: 'fa', name: 'date', required: true },
    });
    const form = document.createElement('form');
    document.body.append(form);
    form.append(view.container);
    expect(form.checkValidity()).toBe(false);
    await view.rerender({ readonly: true });
    expect(form.checkValidity()).toBe(true);
    form.remove();
  });
});

describe('date input callbacks', () => {
  afterEach(cleanup);
  it('provides the ISO value through both event properties', async () => {
    setCalendarPreference('persian');
    const oninput = vi.fn();
    const onchange = vi.fn();
    render(Input, {
      props: { type: 'date', value: '2025-03-20', dataTestid: 'field', oninput, onchange },
    });
    await fireEvent.click(await screen.findByTestId('field'));
    await fireEvent.click(await screen.findByTestId('field-calendar-day-2025-03-19'));
    expect(oninput).toHaveBeenCalledTimes(1);
    expect(onchange).toHaveBeenCalledTimes(1);
    expect(onchange.mock.calls[0][0].target.value).toBe('2025-03-19');
    expect(onchange.mock.calls[0][0].currentTarget.value).toBe('2025-03-19');
  });
});

describe('calendar popup focus', () => {
  afterEach(cleanup);
  it('does not report a blur while focus moves into the calendar', async () => {
    setCalendarPreference('persian');
    const onblur = vi.fn();
    render(Input, {
      props: { type: 'date', value: '2025-03-20', dataTestid: 'focus-date', onblur },
    });
    const button = await screen.findByTestId('focus-date');
    await fireEvent.focus(button);
    await fireEvent.click(button);
    await screen.findByTestId('focus-date-calendar');
    await fireEvent.blur(button);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(onblur).not.toHaveBeenCalled();
  });
});

describe('calendar roadmap positions', () => {
  it.each(['gregory', 'persian', 'hebrew'])(
    'round trips civil days across month and year boundaries: %s',
    (calendar) => {
      const reference = calendarMonthStart(new Date(2025, 2, 20), calendar);
      for (const date of [
        new Date(2024, 11, 31),
        new Date(2025, 2, 20),
        new Date(2025, 2, 21),
        new Date(2025, 10, 2),
        new Date(2026, 0, 1),
      ]) {
        const position = calendarMonthPosition(reference, date, calendar);
        expect(civilDays(calendarDateAtPosition(reference, position, calendar), date)).toBe(0);
      }
    }
  );
});

describe('account preference lifecycle', () => {
  afterEach(() => {
    collectionPreferences.reset();
    vi.clearAllMocks();
  });
  it('ignores a preference response from a logged-out account', async () => {
    /** @type {(value: any) => void} */
    let resolveOld = () => {};
    vi.mocked(userPreferences.get).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOld = resolve;
      })
    );
    const pending = collectionPreferences.ensureLoaded(1);
    collectionPreferences.reset();
    resolveOld({ calendar: 'persian' });
    await pending;
    expect(getCalendarPreference()).toBe('auto');
    vi.mocked(userPreferences.get).mockResolvedValueOnce({ calendar: 'gregory' });
    await collectionPreferences.ensureLoaded(2);
    expect(getCalendarPreference()).toBe('gregory');
  });
});

describe('regional settings calendar loading', () => {
  afterEach(() => {
    cleanup();
    collectionPreferences.reset();
    vi.clearAllMocks();
  });
  it('offers a retry when the calendar preference failed to load', async () => {
    await i18n.setLocale('en');
    collectionPreferences.reset();
    vi.mocked(userPreferences.get).mockRejectedValueOnce(new Error('offline'));
    await collectionPreferences.ensureLoaded(7);
    render(ProfileRegionalSettingsTab, {
      props: { user: { id: 7, timezone: 'UTC', language: 'en' }, userId: 7 },
    });
    vi.mocked(userPreferences.get).mockResolvedValueOnce({ calendar: 'persian' });
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(getCalendarPreference()).toBe('persian'));
  });
});
