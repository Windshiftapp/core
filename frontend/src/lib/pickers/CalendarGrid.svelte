<script>
  /**
   * Headless calendar grid built on Melt UI's `createCalendar`, which operates
   * on `@internationalized/date` values. Passing a value in a non-Gregorian
   * calendar (e.g. `persian`) makes the grid render that calendar, with the
   * locale's week start, month names, and numerals.
   *
   * Emits Gregorian `YYYY-MM-DD` strings so callers keep the storage/API
   * format unchanged; only the presentation is calendar-specific.
   */
  import { createCalendar, melt } from '@melt-ui/svelte';
  import {
    createCalendar as createIntlCalendar,
    getDayOfWeek,
    parseDate,
    startOfWeek,
    today,
    toCalendar
  } from '@internationalized/date';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import { untrack } from 'svelte';
  import { i18n, t } from '../stores/i18n.svelte.js';

  let {
    calendarId = 'gregory',
    locale = 'en',
    value = '',
    min = undefined,
    max = undefined,
    dataTestid = undefined,
    onselect = undefined
  } = $props();

  const isRTL = $derived(i18n.isRTL);

  // The calendar is instantiated once per mount; the parent remounts this
  // component when the calendar changes, so these props are read as initial
  // values on purpose.
  const { intlCalendar, todayValue, selected, minValue, maxValue, weekStartsOn, dayNumber } = untrack(
    () => {
      const intlCalendar = createIntlCalendar(/** @type {any} */ (calendarId));
      const toDateValue = (ymd) => {
        if (!ymd) return undefined;
        try {
          return toCalendar(parseDate(ymd.slice(0, 10)), intlCalendar);
        } catch {
          return undefined;
        }
      };
      const todayValue = toCalendar(today('UTC'), intlCalendar);
      return {
        intlCalendar,
        todayValue,
        selected: toDateValue(value),
        minValue: toDateValue(min),
        maxValue: toDateValue(max),
        // Saturday for fa/ar, Monday for most of Europe, Sunday for en-US.
        weekStartsOn: getDayOfWeek(startOfWeek(todayValue, locale), locale),
        dayNumber: new Intl.NumberFormat(locale, { useGrouping: false })
      };
    }
  );

  const {
    elements: { calendar, heading, grid, cell, prevButton, nextButton },
    states: { months, weekdays, headingValue }
  } = untrack(() =>
    createCalendar({
      locale,
      defaultPlaceholder: selected ?? todayValue,
      defaultValue: selected,
      weekStartsOn: /** @type {0 | 1 | 2 | 3 | 4 | 5 | 6} */ (weekStartsOn),
      minValue,
      maxValue,
      weekdayFormat: 'short',
      calendarLabel: t('pickers.datePicker.selectDate'),
      onValueChange: ({ next }) => {
        if (next) onselect?.(next.toString());
        return next;
      }
    })
  );

  let prevButtonEl = $state(null);
  let nextButtonEl = $state(null);

  // Melt hardcodes English labels on the nav buttons; override them after mount.
  $effect(() => {
    prevButtonEl?.setAttribute('aria-label', t('pickers.datePicker.previousMonth'));
  });
  $effect(() => {
    nextButtonEl?.setAttribute('aria-label', t('pickers.datePicker.nextMonth'));
  });
</script>

<div
  use:melt={$calendar}
  dir={isRTL ? 'rtl' : 'ltr'}
  class="calendar"
  data-testid={dataTestid}
>
  <div class="calendar-header">
    <button
      use:melt={$prevButton}
      bind:this={prevButtonEl}
      class="nav-button"
      data-testid={dataTestid ? `${dataTestid}-prev` : undefined}
    >
      {#if isRTL}
        <ChevronRight size={16} />
      {:else}
        <ChevronLeft size={16} />
      {/if}
    </button>
    <div use:melt={$heading} class="calendar-heading">{$headingValue}</div>
    <button
      use:melt={$nextButton}
      bind:this={nextButtonEl}
      class="nav-button"
      data-testid={dataTestid ? `${dataTestid}-next` : undefined}
    >
      {#if isRTL}
        <ChevronLeft size={16} />
      {:else}
        <ChevronRight size={16} />
      {/if}
    </button>
  </div>

  {#each $months as month}
    <table use:melt={$grid} class="calendar-grid">
      <thead aria-hidden="true">
        <tr>
          {#each $weekdays as weekday}
            <th class="weekday" scope="col">{weekday}</th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each month.weeks as week}
          <tr>
            {#each week as day}
              <td
                use:melt={$cell(day, month.value)}
                class="day-cell"
                data-testid={dataTestid ? `${dataTestid}-day-${day.toString()}` : undefined}
              >
                <span class="day-inner">{dayNumber.format(day.day)}</span>
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  {/each}
</div>

<style>
  .calendar {
    width: 17.5rem;
    padding: 0.5rem;
    color: var(--ds-text);
  }

  .calendar-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.25rem;
    margin-bottom: 0.5rem;
  }

  .calendar-heading {
    flex: 1;
    text-align: center;
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--ds-text);
  }

  .nav-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.75rem;
    height: 1.75rem;
    border-radius: 0.375rem;
    color: var(--ds-text-subtle);
    background: transparent;
    border: 1px solid transparent;
    cursor: pointer;
    transition: background-color 120ms ease, color 120ms ease;
  }

  .nav-button:hover:not([data-disabled]) {
    background-color: var(--ds-surface-hovered);
    color: var(--ds-text);
  }

  .nav-button[data-disabled] {
    opacity: 0.4;
    cursor: default;
  }

  .calendar-grid {
    width: 100%;
    border-collapse: collapse;
  }

  .weekday {
    padding: 0.25rem 0;
    font-size: 0.6875rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: var(--ds-text-subtle);
    text-align: center;
  }

  .day-cell {
    padding: 1px;
    text-align: center;
    cursor: pointer;
  }

  .day-inner {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    border-radius: 9999px;
    font-size: 0.8125rem;
    color: var(--ds-text);
    transition: background-color 120ms ease, color 120ms ease;
  }

  .day-cell:hover:not([data-disabled]) .day-inner {
    background-color: var(--ds-surface-hovered);
  }

  .day-cell[data-today]:not([data-selected]) .day-inner {
    box-shadow: inset 0 0 0 1px var(--ds-border-bold);
  }

  .day-cell[data-selected] .day-inner {
    background-color: var(--ds-interactive);
    color: var(--ds-text-inverse);
    font-weight: 600;
  }

  .day-cell[data-outside-month] .day-inner {
    color: var(--ds-text-subtlest);
  }

  .day-cell[data-disabled] {
    cursor: default;
  }

  .day-cell[data-disabled] .day-inner {
    color: var(--ds-text-disabled);
  }

  .day-cell[data-focused] .day-inner {
    outline: 2px solid var(--ds-border-focused);
    outline-offset: 1px;
  }
</style>
