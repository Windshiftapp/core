<script>
  /**
   * Non-Gregorian calendar field: a trigger plus the Jalali `CalendarGrid`,
   * or the grid rendered inline. Loaded lazily by `DatePicker` so locales that
   * keep the native picker never download the calendar code.
   *
   * Emits Gregorian `YYYY-MM-DD` strings.
   */
  import { createPopover, melt } from '@melt-ui/svelte';
  import { DateFormatter, parseDate } from '@internationalized/date';
  import Calendar from '@lucide/svelte/icons/calendar';
  import X from '@lucide/svelte/icons/x';
  import { untrack } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDate } from '../utils/dateFormatter.js';
  import { serverNow } from '../utils/serverClock.js';
  import { cn } from '../utils/cn.js';
  import { controlSizeClasses } from '../utils/controlSizes.js';
  import CalendarGrid from './CalendarGrid.svelte';

  let {
    value = $bindable(''),
    calendarId = 'persian',
    locale = 'fa',
    min = undefined,
    max = undefined,
    step = undefined,
    disabled = false,
    readonly = false,
    required = false,
    autofocus = false,
    style = '',
    title = undefined,
    ariaDescribedby = undefined,
    name = undefined,
    inputRef = $bindable(null),
    onblur = undefined,
    onfocus = undefined,
    onkeydown = undefined,
    onclick = undefined,
    onkeyup = undefined,
    clearable = true,
    inline = false,
    placeholder = undefined,
    label = '',
    ariaLabel = undefined,
    id = undefined,
    size = 'medium',
    class: className = '',
    dataTestid = undefined,
    onchange = undefined
  } = $props();

  const descriptionId = $props.id();

  const displayFormatter = untrack(
    () =>
      new DateFormatter(locale, {
        calendar: calendarId,
        timeZone: 'UTC',
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

  const controlClass = $derived(
    cn(
      'rounded border transition-colors',
      controlSizeClasses(size),
      inline ? 'w-full' : 'inline-flex items-center justify-between gap-2 text-left',
      className
    )
  );

  function canCommit(nextValue) {
    if (disabled || readonly || (nextValue && ((min && nextValue < min) || (max && nextValue > max)))) return false;
    const dayStep = Number(step);
    return !(nextValue && dayStep > 0 && (Date.parse(nextValue) - Date.parse(min || '1970-01-01')) / 86400000 % dayStep !== 0);
  }

  let validationMessage = $state('');
  $effect(() => {
    void [value, readonly, disabled, min, max, step, required];
    validationMessage = '';
  });
  function commit(nextValue) {
    if (!canCommit(nextValue)) return;
    validationMessage = '';
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
    <span class="date-picker-label text-ds-text">{label}</span>
  {/if}
  {#if name || required}
    <input type="date" {name} {required} {min} {max} {step} {disabled} {readonly} value={value} tabindex="-1" aria-hidden="true" style="position:absolute; width:1px; height:1px; opacity:0;" oninvalid={(event) => { event.preventDefault(); validationMessage = event.currentTarget.validationMessage; inputRef?.focus(); }} />
  {/if}
  {#if required}
    <span id="{descriptionId}-required" class="sr-only">{t('validation.required')}</span>
  {/if}
  {#if validationMessage}
    <span id="{descriptionId}-error" role="alert" class="text-xs text-ds-text-danger">{validationMessage}</span>
  {/if}
  <div class="date-picker-control">
    {#if inline}
      <div
        class="date-picker-inline bg-ds-surface-raised border border-ds-border"
      >
        <CalendarGrid
          {calendarId}
          {locale}
          {value}
          {min}
          {max}
          {readonly}
          {disabled}
          onselect={commit}
          dataTestid={dataTestid ? `${dataTestid}-calendar` : undefined}
        />
      </div>
    {:else}
      <!-- svelte-ignore a11y_autofocus -->
      <button
        use:melt={$trigger}
        type="button"
        {id}
        {disabled}
        {autofocus}
        {title}
        aria-describedby={[ariaDescribedby, required ? `${descriptionId}-required` : '', validationMessage ? `${descriptionId}-error` : ''].filter(Boolean).join(' ') || undefined}
        bind:this={inputRef}
        {onfocus}
        onkeydown={(event) => {
          if ($open && (event.key === 'Enter' || event.key === 'Escape')) return;
          onkeydown?.(event);
        }}
        {onclick}
        {onkeyup}
        onblur={(event) => setTimeout(() => { if (!$open) onblur?.(event); }, 0)}
        aria-label={ariaLabel ?? t('pickers.datePicker.openCalendar')}
        class="bg-ds-background-input border-ds-border {value ? 'text-ds-text' : 'text-ds-text-subtle'} {controlClass}"
        style="{style}"
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
    {/if}
  </div>

  {#if !inline && $open}
    <div
      use:melt={$content}
      class="date-picker-popover bg-ds-surface-raised border border-ds-border"
      data-testid={dataTestid ? `${dataTestid}-popover` : undefined}
    >
      <CalendarGrid
        {calendarId}
        {locale}
        {value}
        {min}
        {max}
        {readonly}
        {disabled}
        onselect={commit}
        dataTestid={dataTestid ? `${dataTestid}-calendar` : undefined}
      />
      <div class="date-picker-footer border-t border-ds-border">
        <button
          type="button"
          class="date-picker-footer-button"
          disabled={!canCommit(formatDate(serverNow()))}
          onclick={() => commit(parseDate(formatDate(serverNow())).toString())}
          data-testid={dataTestid ? `${dataTestid}-today` : undefined}
        >
          {t('common.today')}
        </button>
        {#if clearable && value && !readonly && !disabled}
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

  .date-picker-inline {
    border-radius: 0.5rem;
    box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
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
