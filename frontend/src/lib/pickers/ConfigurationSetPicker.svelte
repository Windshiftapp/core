<script>
  import ItemPicker from './ItemPicker.svelte';
  import { Settings } from 'lucide-svelte';
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

  const config = {
    icon: {
      type: 'component',
      source: () => Settings
    },
    primary: { text: (item) => item.name || '' },
    secondary: { text: (item) => item.description || '' },
    searchFields: ['name', 'description'],
    getValue: (item) => item?.id,
    getLabel: (item) => item?.name || ''
  };
</script>

<ItemPicker
  bind:value
  {items}
  {config}
  placeholder={resolvedPlaceholder}
  showUnassigned={true}
  unassignedLabel={t('pickers.defaultConfiguration')}
  {disabled}
  allowClear={true}
  class={className}
  onSelect={(item) => onSelect(item)}
  onCancel={() => onCancel()}
/>
