<script>
  /**
   * Calendar-aware date field.
   *
   * The bound `value` is always a Gregorian `YYYY-MM-DD` string, matching
   * `<input type="date">` and the timestamp APIs.
   *
   * Two rendering modes, chosen from the resolved calendar:
   * - Gregorian locales keep a native `<input type="date">`, which has the best
   *   keyboard and OS-picker behaviour.
   * - Non-Gregorian locales (Persian for `fa`) load `CalendarDateField` lazily,
   *   so the Jalali calendar and `@internationalized/date` never reach locales
   *   that keep the native picker.
   *
   * `inline` renders the control directly instead of inside its own popover,
   * for use inside an existing popover (e.g. the create-modal date chips).
   */
  import Calendar from '@lucide/svelte/icons/calendar';
  import { t } from '../stores/i18n.svelte.js';
  import { getAppLocale } from '../stores/appLocale.svelte.js';
  import { resolveCalendar } from '../stores/calendar.svelte.js';
  import { cn } from '../utils/cn.js';
  import { controlSizeClasses } from '../utils/controlSizes.js';
  import { LazyComponentLoader } from '../utils/lazyComponentLoader.svelte.js';

  let {
    value = $bindable(''),
    calendar = undefined,
    locale = undefined,
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
    clearable = true,
    inline = false,
    autoOpen = false,
    placeholder = undefined,
    label = '',
    ariaLabel = undefined,
    id = undefined,
    name = undefined,
    size = 'medium',
    class: className = '',
    dataTestid = undefined,
    inputRef = $bindable(null),
    onchange = undefined,
    onkeydown = undefined,
    onblur = undefined,
    onfocus = undefined,
    onclick = undefined,
    onkeyup = undefined
  } = $props();

  const resolvedLocale = $derived(locale || getAppLocale());
  const calendarId = $derived(
    resolveCalendar(resolvedLocale, calendar)
  );
  const isNative = $derived(calendarId === 'gregory');
  const resolvedAriaLabel = $derived(ariaLabel ?? (label || undefined));

  const controlClass = $derived(
    cn(
      'rounded border transition-colors',
      controlSizeClasses(size),
      inline ? 'w-full' : 'inline-flex items-center justify-between gap-2 text-left',
      className
    )
  );

  // Load the non-Gregorian field only when the locale needs it.
  const calendarLoader = new LazyComponentLoader({ calendar: () => import('./CalendarDateField.svelte') });
  let CalendarDateField = $derived(calendarLoader.getComponent('calendar'));
  $effect(() => {
    if (!isNative) void calendarLoader.load('calendar');
  });

  function handleNativeInput(event) {
    const next = event.currentTarget.value;
    if (next === value) return;
    value = next;
    onchange?.(value, event);
  }

  // Open the OS picker as soon as the input mounts, so a chip click still sets
  // a date in one interaction.
  function autoOpenPicker(node) {
    if (!autoOpen) return;
    node.focus();
    try {
      node.showPicker?.();
    } catch {
      // Keep the focused input available for manual entry.
    }
  }
</script>

{#if isNative}
  {#if label}
    <span class="date-picker-label text-ds-text">{label}</span>
  {/if}
  <!-- svelte-ignore a11y_autofocus -->
  <input
    type="date"
    {id}
    {name}
    {min}
    {max}
    {step}
    {disabled}
    {required}
    {autofocus}
    {title}
    aria-describedby={ariaDescribedby}
    readonly={readonly}
    aria-label={resolvedAriaLabel}
    {placeholder}
    class="bg-ds-background-input border-ds-border text-ds-text {controlClass}"
    style="{style}"
    data-testid={dataTestid}
    value={value}
    bind:this={inputRef}
    oninput={handleNativeInput}
    onchange={handleNativeInput}
    {onkeydown}
    {onblur}
    {onfocus}
    {onclick}
    {onkeyup}
    use:autoOpenPicker
  />
{:else if CalendarDateField}
  {#key `${calendarId}:${resolvedLocale}`}
  <CalendarDateField
    value={value}
    {calendarId}
    locale={resolvedLocale}
    {min}
    {max}
    {step}
    {disabled}
    {readonly}
    {required}
    {autofocus}
    {title}
    {ariaDescribedby}
    {style}
    {onclick}
    {onkeyup}
    {name}
    bind:inputRef
    {onblur}
    {onfocus}
    {onkeydown}
    {clearable}
    {inline}
    {placeholder}
    {label}
    {ariaLabel}
    {id}
    {size}
    class={className}
    {dataTestid}
    onchange={(next) => {
      value = next;
      onchange?.(next);
    }}
  />
  {/key}
{:else if calendarLoader.getError('calendar')}
  <button type="button" class={controlClass} onclick={() => calendarLoader.retry('calendar')}>
    {t('errors.failedToLoad')} · {t('common.retry')}
  </button>
{:else}
  <button
    type="button"
    class="bg-ds-background-input border-ds-border text-ds-text-subtle {controlClass}"
    {disabled}
    data-testid={dataTestid ? `${dataTestid}-loading` : undefined}
  >
    <span class="date-picker-icon"><Calendar size={15} /></span>
    <span class="date-picker-text">{placeholder ?? t('pickers.datePicker.selectDate')}</span>
  </button>
{/if}

<style>
  .date-picker-label {
    display: block;
    font-size: 0.8125rem;
    font-weight: 500;
    margin-bottom: 0.25rem;
  }

  :global(.date-picker-icon) {
    display: inline-flex;
    align-items: center;
    color: var(--ds-text-subtle);
    flex-shrink: 0;
  }

  :global(.date-picker-text) {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
