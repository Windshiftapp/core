<script>
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { onMount, createEventDispatcher } from 'svelte';
  import { Shield } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  let {
    value = $bindable(null),
    placeholder = '',
    label = '',
    disabled = false,
    class: className = ''
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectRole'));

  const roles = createAsyncLoader(() => api.get('/workspace-roles'));

  onMount(() => roles.load());

  function handleSelect(event) {
    dispatch('select', event.detail);
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

<BasePicker
  bind:value
  items={roles.data}
  loading={roles.loading}
  error={roles.error}
  placeholder={resolvedPlaceholder}
  {label}
  {disabled}
  class={className}
  searchFields={['name', 'description']}
  getValue={(role) => role?.id}
  getLabel={(role) => role?.name ?? ''}
  on:select={handleSelect}
  on:cancel={handleCancel}
>
  {#snippet itemSnippet({ item: role, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <!-- Role Icon -->
      <div class="flex-shrink-0">
        <div class="w-8 h-8 rounded-full flex items-center justify-center" style="background: var(--ds-interactive); color: var(--ds-text-inverse);">
          <Shield class="w-4 h-4" />
        </div>
      </div>

      <!-- Role Info -->
      <div class="flex flex-col min-w-0">
        <span class="font-medium truncate">{role.name}</span>
        {#if role.description}
          <span class="text-sm truncate" style="color: var(--ds-text-subtle);">{role.description}</span>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>
