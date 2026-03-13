<script>
  import ItemPicker from './ItemPicker.svelte';
  import { User } from 'lucide-svelte';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { onMount } from 'svelte';

  let {
    value = $bindable(null),
    placeholder = 'Select portal customer',
    showUnassigned = false,
    unassignedLabel = 'None',
    disabled = false,
    class: className = '',
    children = null,
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const customers = createAsyncLoader(() => api.portalCustomers.getAll());
  onMount(() => customers.load());

  const config = {
    icon: {
      type: 'component',
      source: () => User
    },
    primary: { text: (item) => item.name || '' },
    secondary: { text: (item) => item.email || '' },
    searchFields: ['name', 'email', 'customer_organisation_name'],
    getValue: (item) => item?.id,
    getLabel: (item) => item?.name || ''
  };
</script>

<ItemPicker
  bind:value
  items={customers.data || []}
  {config}
  {placeholder}
  {showUnassigned}
  {unassignedLabel}
  {disabled}
  loading={customers.loading}
  class={className}
  {children}
  onSelect={(item) => onSelect(item)}
  onCancel={() => onCancel()}
/>
