<script>
  import BasePicker from './BasePicker.svelte';
  import User from '@lucide/svelte/icons/user';
  import { createAsyncLoader } from '../composables';
  import { portalCustomers } from '../api/portal.js';
  import { onDestroy, onMount } from 'svelte';

  let {
    value = $bindable(null),
    placeholder = 'Select portal customer',
    showUnassigned = false,
    unassignedLabel = 'None',
    disabled = false,
    customers: providedCustomers = null,
    loading = false,
    onOpen = null,
    autoOpen = false,
    // (value) => string | null. Labels a selected customer id that is not in
    // the loaded options (options load lazily on mount/open).
    resolveMissingLabel = null,
    class: className = '',
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const customers = createAsyncLoader(() => portalCustomers.getAll());
  onMount(() => { if (providedCustomers === null) customers.load(); });
  onDestroy(() => customers.dispose());
  const customerOptions = $derived(providedCustomers ?? customers.data ?? []);
</script>

{#snippet customerRow({ item })}
  <User class="w-4 h-4 flex-shrink-0" />
  <div class="flex flex-col min-w-0">
    <span class="font-medium truncate">{item?.name || ''}</span>
    {#if item?.email}
      <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{item.email}</span>
    {/if}
  </div>
{/snippet}

<BasePicker
  bind:value
  items={customerOptions}
  loading={providedCustomers === null ? customers.loading : loading}
  {placeholder}
  {showUnassigned}
  {unassignedLabel}
  {disabled}
  allowClear={true}
  {autoOpen}
  class={className}
  itemSnippet={customerRow}
  searchFields={['name', 'email', 'customer_organisation_name']}
  getValue={(item) => item?.id}
  getLabel={(item) => item?.name || ''}
  {resolveMissingLabel}
  onOpen={() => onOpen?.()}
  onSelect={(item) => onSelect(item)}
  onCancel={() => onCancel()}
/>
