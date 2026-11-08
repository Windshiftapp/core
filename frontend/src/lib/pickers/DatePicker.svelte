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
  import { cn } from '../utils/cn.js';
  import { controlSizeClasses } from '../utils/controlSizes.js';

  let {
    value = $bindable(''),
    calendar = undefined,
    locale = undefined,
    min = undefined,
    max = undefined,
    disabled = false,
    readonly = false,
    required = false,
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
    onfocus = undefined
  } = $props();

  const PERSIAN_LOCALES = new Set(['fa']);

  const resolvedLocale = $derived(locale || getAppLocale());
  const calendarId = $derived(
    calendar || (PERSIAN_LOCALES.has(resolvedLocale) ? 'persian' : 'gregory')
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
  let CalendarDateField = $state(null);
  $effect(() => {
    if (isNative || CalendarDateField) return;
    let cancelled = false;
    import('./CalendarDateField.svelte').then((module) => {
      if (!cancelled) CalendarDateField = module.default;
    });
    return () => {
      cancelled = true;
    };
  });

  function handleNativeInput(event) {
    value = event.currentTarget.value;
    onchange?.(value);
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
    <span class="date-picker-label" style="color: var(--ds-text);">{label}</span>
  {/if}
  <input
    type="date"
    {id}
    {name}
    {min}
    {max}
    {disabled}
    {required}
    readonly={readonly}
    aria-label={resolvedAriaLabel}
    {placeholder}
    class={controlClass}
    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
    data-testid={dataTestid}
    value={value}
    bind:this={inputRef}
    oninput={handleNativeInput}
    onchange={handleNativeInput}
    {onkeydown}
    {onblur}
    {onfocus}
    use:autoOpenPicker
  />
{:else if CalendarDateField}
  <CalendarDateField
    value={value}
    {calendarId}
    locale={resolvedLocale}
    {min}
    {max}
    {disabled}
    {readonly}
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
{:else}
  <button
    type="button"
    class={controlClass}
    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text-subtle);"
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
