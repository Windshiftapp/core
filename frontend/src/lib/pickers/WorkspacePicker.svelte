<script>
  import { BasePicker } from '.';
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Briefcase, Package } from 'lucide-svelte';
  import { workspaceIconMap } from '../utils/icons.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable([]),
    placeholder = '',
    label = '',
    disabled = false,
    class: className = '',
    onChange = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectWorkspaces'));

  let workspaces = $state([]);
  let loading = $state(false);
  let error = $state(null);

  onMount(async () => {
    await loadWorkspaces();
  });

  async function loadWorkspaces() {
    if (loading) return;

    try {
      loading = true;
      error = null;
      const allWorkspaces = await api.workspaces.getAll() || [];
      // Filter out personal workspaces for dropdown
      workspaces = allWorkspaces.filter(w => !w.is_personal);
    } catch (err) {
      console.error('Failed to load workspaces:', err);
      error = err.message || 'Failed to load workspaces';
      workspaces = [];
    } finally {
      loading = false;
    }
  }

  function getIconComponent(iconName) {
    if (iconName && workspaceIconMap[iconName]) {
      return workspaceIconMap[iconName];
    }
    return Briefcase;
  }
</script>

<BasePicker
  bind:value
  items={workspaces}
  {loading}
  {error}
  placeholder={resolvedPlaceholder}
  {label}
  {disabled}
  class={className}
  multiple={true}
  searchFields={['name', 'key', 'description']}
  getValue={(workspace) => workspace?.id}
  getLabel={(workspace) => workspace?.name ?? ''}
  onChange={(value) => onChange(value)}
  onCancel={() => onCancel()}
>
  {#snippet chipSnippet({ item: workspace })}
    <!-- Workspace Icon/Avatar -->
    <div class="w-3.5 h-3.5 rounded flex items-center justify-center text-white flex-shrink-0 overflow-hidden"
         style="background-color: {workspace.color || '#3b82f6'};">
      {#if workspace.avatar || workspace.image}
        <img src={workspace.avatar || workspace.image} alt="" class="w-full h-full object-cover" />
      {:else}
        <svelte:component this={getIconComponent(workspace.icon)} class="w-2 h-2" />
      {/if}
    </div>
    <span class="font-medium">{workspace.key}</span>
  {/snippet}

  {#snippet itemSnippet({ item: workspace, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <!-- Workspace Icon/Avatar -->
      <div class="flex-shrink-0">
        <div class="w-6 h-6 rounded-md flex items-center justify-center text-white text-xs font-medium overflow-hidden"
             style="background-color: {workspace.color || '#3b82f6'};">
          {#if workspace.avatar || workspace.image}
            <img src={workspace.avatar || workspace.image} alt="" class="w-full h-full object-cover" />
          {:else}
            <svelte:component this={getIconComponent(workspace.icon)} class="w-3 h-3" />
          {/if}
        </div>
      </div>

      <!-- Workspace Info -->
      <div class="flex flex-col min-w-0">
        <div class="flex items-center gap-2">
          <span class="font-medium text-xs px-1.5 py-0.5 rounded"
                style="background-color: var(--ds-surface); color: var(--ds-text-subtle);">
            {workspace.key}
          </span>
          <span class="font-medium truncate">{workspace.name}</span>
        </div>
        {#if workspace.description}
          <span class="text-sm truncate" style="color: var(--ds-text-subtle);">{workspace.description}</span>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>
