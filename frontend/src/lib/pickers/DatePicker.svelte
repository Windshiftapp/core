<script>
  /**
   * Shared Melt calendar field for every calendar system.
   * Values remain Gregorian YYYY-MM-DD strings; inline mode omits the popover.
   */
  import { getAppLocale } from '../stores/appLocale.svelte.js';
  import { resolveCalendar } from '../stores/calendar.svelte.js';
  import CalendarDateField from './CalendarDateField.svelte';

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
    ariaInvalid = undefined,
    ariaRequired = undefined,
    ariaLabelledby = undefined,
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
  const resolvedAriaLabel = $derived(ariaLabel ?? (label || undefined));

</script>

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
    {autoOpen}
    {title}
    {ariaDescribedby}
    {ariaInvalid}
    {ariaRequired}
    {ariaLabelledby}
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
    ariaLabel={resolvedAriaLabel}
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
