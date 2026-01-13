<script>
  import { BasePicker } from '../pickers';
  import { createEventDispatcher } from 'svelte';
  import { Building } from 'lucide-svelte';

  const dispatch = createEventDispatcher();

  let {
    value = $bindable(null),
    workspaces = [],
    placeholder = 'Select workspace...',
    disabled = false,
    allowClear = false,
    loading = false,
    class: className = ''
  } = $props();

  function handleSelect(event) {
    dispatch('select', event.detail);
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

<BasePicker
  bind:value
  items={workspaces}
  {loading}
  {placeholder}
  {disabled}
  {allowClear}
  class={className}
  searchFields={['name', 'key', 'description']}
  getValue={(workspace) => workspace?.id}
  getLabel={(workspace) => workspace?.name ?? ''}
  on:select={handleSelect}
  on:cancel={handleCancel}
>
  {#snippet itemSnippet({ item: workspace, isSelected })}
    <div class="flex items-start gap-3 flex-1 min-w-0">
      <!-- Workspace avatar or icon -->
      {#if workspace.avatar_url}
        <img
          src={workspace.avatar_url}
          alt={workspace.name}
          class="w-8 h-8 rounded flex-shrink-0 mt-0.5"
        />
      {:else}
        <div
          class="w-8 h-8 rounded flex items-center justify-center flex-shrink-0"
          style="background-color: {workspace.color || '#E0E7FF'};"
        >
          <Building size={16} style="color: {workspace.color ? '#fff' : 'var(--ds-text-subtle)'};" />
        </div>
      {/if}

      <!-- Workspace info -->
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2 mb-1">
          <span class="font-medium truncate">{workspace.name}</span>
          {#if workspace.key}
            <span
              class="text-xs px-1.5 py-0.5 rounded flex-shrink-0"
              style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
            >
              {workspace.key}
            </span>
          {/if}
        </div>

        {#if workspace.description}
          <div class="text-xs line-clamp-2" style="color: var(--ds-text-subtle);">
            {workspace.description}
          </div>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>

<style>
  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
</style>
