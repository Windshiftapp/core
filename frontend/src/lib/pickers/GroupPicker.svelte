<script>
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { onMount, createEventDispatcher } from 'svelte';
  import { Users } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  let {
    value = $bindable(null),
    placeholder = '',
    label = '',
    disabled = false,
    class: className = ''
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectGroup'));

  const groups = createAsyncLoader(() => api.get('/groups'));

  onMount(() => groups.load());

  function handleSelect(event) {
    dispatch('select', event.detail);
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

<BasePicker
  bind:value
  items={groups.data}
  loading={groups.loading}
  error={groups.error}
  placeholder={resolvedPlaceholder}
  {label}
  {disabled}
  class={className}
  searchFields={['group_name', 'name', 'description']}
  getValue={(group) => group?.id}
  getLabel={(group) => group?.group_name || group?.name || ''}
  on:select={handleSelect}
  on:cancel={handleCancel}
>
  {#snippet itemSnippet({ item: group, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <!-- Group Icon -->
      <div class="flex-shrink-0">
        <div class="w-8 h-8 rounded-full bg-green-500 flex items-center justify-center text-white">
          <Users class="w-4 h-4" />
        </div>
      </div>

      <!-- Group Info -->
      <div class="flex flex-col min-w-0">
        <span class="font-medium truncate">{group.group_name || group.name}</span>
        {#if group.description}
          <span class="text-sm truncate" style="color: var(--ds-text-subtle);">{group.description}</span>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>
