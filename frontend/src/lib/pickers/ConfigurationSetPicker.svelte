<script>
  import BasePicker from './BasePicker.svelte';
  import { Settings } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    items = [],
    placeholder = '',
    disabled = false,
    class: className = '',
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.defaultConfiguration'));

  function isDefaultSystemConfiguration(item) {
    return Boolean(item?.builtin_key);
  }

  function getConfigurationDisplayValue(item, field) {
    return isDefaultSystemConfiguration(item)
      ? t(`settings.configSets.defaults.configuration.${field}`)
      : (item?.[field] || '');
  }
</script>

{#snippet configSetRow({ item })}
  <Settings class="w-4 h-4 flex-shrink-0" />
  <div class="flex flex-col min-w-0">
    <span class="font-medium truncate">{getConfigurationDisplayValue(item, 'name')}</span>
    {#if getConfigurationDisplayValue(item, 'description')}
      <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{getConfigurationDisplayValue(item, 'description')}</span>
    {/if}
  </div>
{/snippet}

<BasePicker
  bind:value
  {items}
  placeholder={resolvedPlaceholder}
  showUnassigned={true}
  unassignedLabel={t('pickers.defaultConfiguration')}
  {disabled}
  allowClear={true}
  class={className}
  itemSnippet={configSetRow}
  searchFields={['name', 'description']}
  getValue={(item) => item?.id}
  getLabel={(item) => getConfigurationDisplayValue(item, 'name')}
  onSelect={(item) => onSelect(item)}
  onCancel={() => onCancel()}
/>
