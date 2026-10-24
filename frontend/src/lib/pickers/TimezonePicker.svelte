<script>
  import { BasePicker } from '.';
  import { i18n, t } from '../stores/i18n.svelte.js';
  import { listIanaTimezones } from '../utils/timeUtils.js';

  let {
    value = $bindable(null),
    id = undefined,
    disabled = false,
    placeholder = '',
    class: className = '',
    allowClear = false,
    showSelectedInTrigger = true,
    inputTestid = undefined,
    searchTestid = undefined,
    optionTestid = null,
    menuTestid = undefined,
    scrollTestid = undefined,
    onSelect = () => {},
    onCancel = () => {},
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('users.timezone'));

  // Offer every IANA zone the runtime knows. Keep a stored value visible even
  // when it is an alias or no longer canonical (e.g. legacy "Local").
  const baseTimezoneIds = listIanaTimezones();

  function timezoneLabel(timezone) {
    try {
      const formatter = new Intl.DateTimeFormat(i18n.locale, {
        timeZone: timezone,
        timeZoneName: timezone === 'UTC' ? 'long' : 'longGeneric',
      });
      const name = formatter.formatToParts(new Date()).find((part) => part.type === 'timeZoneName')?.value;
      if (!name) return timezone;
      return timezone === 'UTC' ? `UTC (${name})` : `${name} (${timezone})`;
    } catch {
      return timezone;
    }
  }

  // Labels are locale-dependent, so rebuild them on locale change only. The
  // selected value must not invalidate the whole list on every pick.
  const baseTimezones = $derived(
    baseTimezoneIds.map((tz) => ({ value: tz, label: timezoneLabel(tz) }))
  );
  const timezones = $derived.by(() => {
    if (value && !baseTimezoneIds.includes(value)) {
      return [{ value, label: timezoneLabel(value) }, ...baseTimezones];
    }
    return baseTimezones;
  });
</script>

<BasePicker
  bind:value
  {id}
  items={timezones}
  {disabled}
  {allowClear}
  {showSelectedInTrigger}
  placeholder={resolvedPlaceholder}
  class={className}
  searchFields={['value', 'label']}
  getValue={(item) => item.value}
  getLabel={(item) => item.label}
  {inputTestid}
  {searchTestid}
  {optionTestid}
  {menuTestid}
  {scrollTestid}
  onSelect={(item) => onSelect(item)}
  onCancel={() => onCancel()}
/>
