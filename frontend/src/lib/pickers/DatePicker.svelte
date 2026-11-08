<script>
  /**
   * Calendar-aware date picker: a popover trigger plus `CalendarGrid`.
   *
   * The bound `value` is always a Gregorian `YYYY-MM-DD` string, matching
   * `<input type="date">` and the timestamp APIs, so it is a drop-in
   * replacement. The displayed calendar follows the UI locale by default
   * (Persian for `fa`) and can be overridden with the `calendar` prop.
   */
  import { createPopover, melt } from '@melt-ui/svelte';
  import { DateFormatter, parseDate, today } from '@internationalized/date';
  import Calendar from '@lucide/svelte/icons/calendar';
  import X from '@lucide/svelte/icons/x';
  import { i18n, t } from '../stores/i18n.svelte.js';
  import { cn } from '../utils/cn.js';
  import { controlSizeClasses } from '../utils/controlSizes.js';
  import CalendarGrid from './CalendarGrid.svelte';

  let {
    value = $bindable(''),
    calendar = undefined,
    locale = undefined,
    min = undefined,
    max = undefined,
    disabled = false,
    readonly = false,
    clearable = true,
    placeholder = undefined,
    label = '',
    ariaLabel = undefined,
    id = undefined,
    name = undefined,
    size = 'medium',
    class: className = '',
    dataTestid = undefined,
    onchange = undefined
  } = $props();

  const PERSIAN_LOCALES = new Set(['fa']);

  const resolvedLocale = $derived(locale || i18n.locale || 'en');
  const calendarId = $derived(
    calendar || (PERSIAN_LOCALES.has(resolvedLocale) ? 'persian' : 'gregory')
  );

  const displayFormatter = $derived(
    new DateFormatter(resolvedLocale, {
      calendar: calendarId,
      day: 'numeric',
      month: 'long',
      year: 'numeric'
    })
  );

  const displayText = $derived.by(() => {
    if (!value) return placeholder ?? t('pickers.datePicker.selectDate');
    try {
      return displayFormatter.format(parseDate(value.slice(0, 10)).toDate('UTC'));
    } catch {
      return value;
    }
  });

  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    positioning: { placement: 'bottom-start', gutter: 4 },
    portal: 'body',
    forceVisible: true
  });

  const triggerClass = $derived(
    cn(
      'inline-flex items-center justify-between gap-2 rounded border transition-colors text-left',
      controlSizeClasses(size),
      className
    )
  );

  function commit(nextValue) {
    value = nextValue;
    onchange?.(nextValue);
    $open = false;
  }

  function handleClear(event) {
    event?.stopPropagation();
    commit('');
  }
</script>

<div class="date-picker" data-testid={dataTestid ? `${dataTestid}-root` : undefined}>
  {#if label}
    <span class="date-picker-label" style="color: var(--ds-text);">{label}</span>
  {/if}
  <div class="date-picker-control">
    <button
      use:melt={$trigger}
      type="button"
      {id}
      {disabled}
      aria-label={ariaLabel ?? (label || t('pickers.datePicker.openCalendar'))}
      class={triggerClass}
      style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: {value
        ? 'var(--ds-text)'
        : 'var(--ds-text-subtle)'};"
      data-testid={dataTestid}
    >
      <span class="date-picker-icon">
        <Calendar size={15} />
      </span>
      <span class="date-picker-text">{displayText}</span>
    </button>
    {#if clearable && value && !disabled && !readonly}
      <button
        type="button"
        class="date-picker-clear"
        onclick={handleClear}
        aria-label={t('common.clear')}
        data-testid={dataTestid ? `${dataTestid}-clear` : undefined}
      >
        <X size={14} />
      </button>
    {/if}
  </div>

  {#if $open}
    <div
      use:melt={$content}
      class="date-picker-popover"
      data-testid={dataTestid ? `${dataTestid}-popover` : undefined}
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      {#key calendarId}
        <CalendarGrid
          {calendarId}
          locale={resolvedLocale}
          {value}
          {min}
          {max}
          onselect={commit}
          dataTestid={dataTestid ? `${dataTestid}-calendar` : undefined}
        />
      {/key}
      <div class="date-picker-footer" style="border-top: 1px solid var(--ds-border);">
        <button
          type="button"
          class="date-picker-footer-button"
          onclick={() => commit(today('UTC').toString())}
          data-testid={dataTestid ? `${dataTestid}-today` : undefined}
        >
          {t('common.today')}
        </button>
        {#if clearable && value}
          <button
            type="button"
            class="date-picker-footer-button"
            onclick={handleClear}
            data-testid={dataTestid ? `${dataTestid}-footer-clear` : undefined}
          >
            {t('common.clear')}
          </button>
        {/if}
      </div>
    </div>
  {/if}

  {#if name}
    <input type="hidden" {name} value={value} />
  {/if}
</div>

<style>
  .date-picker {
    display: inline-flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .date-picker-label {
    font-size: 0.8125rem;
    font-weight: 500;
  }

  .date-picker-control {
    position: relative;
    display: inline-flex;
    align-items: center;
    width: 100%;
  }

  .date-picker-control :global(.date-picker-icon) {
    display: inline-flex;
    align-items: center;
    color: var(--ds-text-subtle);
    flex-shrink: 0;
  }

  .date-picker-control :global(.date-picker-text) {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .date-picker-clear {
    position: absolute;
    inset-inline-end: 0.5rem;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.25rem;
    height: 1.25rem;
    border-radius: 9999px;
    border: none;
    background: transparent;
    color: var(--ds-text-subtle);
    cursor: pointer;
  }

  .date-picker-clear:hover {
    background-color: var(--ds-surface-hovered);
    color: var(--ds-text);
  }

  .date-picker-popover {
    z-index: 70;
    border-radius: 0.5rem;
    box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
  }

  .date-picker-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.5rem;
  }

  .date-picker-footer-button {
    padding: 0.25rem 0.5rem;
    border-radius: 0.375rem;
    border: none;
    background: transparent;
    font-size: 0.8125rem;
    color: var(--ds-interactive);
    cursor: pointer;
  }

  .date-picker-footer-button:hover {
    background-color: var(--ds-surface-hovered);
  }
</style>
