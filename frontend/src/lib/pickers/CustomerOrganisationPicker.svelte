<script>
  import ItemPicker from './ItemPicker.svelte';
  import { Building2 } from 'lucide-svelte';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { onMount } from 'svelte';

  let {
    value = $bindable(null),
    placeholder = 'Select organisation',
    showUnassigned = false,
    unassignedLabel = 'None',
    disabled = false,
    class: className = '',
    children = null,
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const organisations = createAsyncLoader(() => api.customerOrganisations.getAll());
  onMount(() => organisations.load());

  const config = {
    icon: {
      type: 'component',
      source: () => Building2
    },
    primary: { text: (item) => item.name || '' },
    secondary: { text: (item) => item.email || item.description || '' },
    searchFields: ['name', 'email', 'description'],
    getValue: (item) => item?.id,
    getLabel: (item) => item?.name || ''
  };
</script>

<ItemPicker
  bind:value
  items={organisations.data || []}
  {config}
  {placeholder}
  {showUnassigned}
  {unassignedLabel}
  {disabled}
  loading={organisations.loading}
  class={className}
  {children}
  onSelect={(item) => onSelect(item)}
  onCancel={() => onCancel()}
/>
