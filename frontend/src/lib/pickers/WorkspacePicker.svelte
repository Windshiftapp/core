<script>
  import { BasePicker } from '.';
  import { onMount } from 'svelte';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import Briefcase from '@lucide/svelte/icons/briefcase';
  import Package from '@lucide/svelte/icons/package';
  import { workspaceIconMap } from '../utils/icons.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable([]),
    placeholder = '',
    label = '',
    disabled = false,
    multiple = true,
    allowClear = false,
    items = null,
    class: className = '',
    onChange = () => {},
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectWorkspaces'));

  let loadedWorkspaces = $state([]);
  let searchedWorkspaces = $state(null);
  // Server results replace the cached page while a search is active; before
  // the first response (and once the query clears) the cached page applies
  // with instant local filtering.
  let workspaces = $derived(items ?? (searchedWorkspaces ?? loadedWorkspaces));
  let loading = $state(false);
  let error = $state(null);

  // Every workspace we have ever seen, so a selected chip stays resolvable
  // after the option list is replaced by a server search and then cleared.
  let knownById = $state(new Map());

  function rememberWorkspaces(list) {
    if (!list || list.length === 0) return;
    const next = new Map(knownById);
    let changed = false;
    for (const w of list) {
      if (!w || w.id == null || next.has(w.id)) continue;
      next.set(w.id, w);
      changed = true;
    }
    if (changed) knownById = next;
  }

  function resolveMissingWorkspaceLabel(id) {
    const ws = knownById.get(id);
    if (ws) return ws.name || ws.key || `#${id}`;
    return `#${id}`;
  }

  onMount(async () => {
    if (items === null) {
      await loadWorkspaces();
    }
  });

  async function loadWorkspaces() {
    if (loading) return;

    try {
      loading = true;
      error = null;
      const allWorkspaces = (await workspacesStore.load()) || [];
      // Filter out personal workspaces for dropdown
      loadedWorkspaces = allWorkspaces.filter(w => !w.is_personal);
      rememberWorkspaces(loadedWorkspaces);
    } catch (err) {
      console.error('Failed to load workspaces:', err);
      error = err.message || 'Failed to load workspaces';
      loadedWorkspaces = [];
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

  let workspaceSearchToken = 0;

  async function handleSearchChange(query) {
    const trimmed = query.trim();
    const token = ++workspaceSearchToken;
    if (!trimmed) {
      searchedWorkspaces = null;
      return;
    }
    const result = await workspacesStore.searchWorkspaces(trimmed, { limit: 100 });
    if (token !== workspaceSearchToken) return;
    searchedWorkspaces = (result.workspaces || []).filter(w => !w.is_personal);
    rememberWorkspaces(searchedWorkspaces);
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
  multiple={multiple}
  {allowClear}
  serverSearch={searchedWorkspaces !== null}
  resolveMissingLabel={resolveMissingWorkspaceLabel}
  onSearchChange={handleSearchChange}
  searchFields={['name', 'key', 'description']}
  getValue={(workspace) => workspace?.id}
  getLabel={(workspace) => workspace?.name ?? ''}
  onChange={(value) => onChange(value)}
  onSelect={(item) => onSelect(item)}
  onCancel={() => onCancel()}
>
  {#snippet chipSnippet({ item: workspace })}
    <!-- Workspace Icon/Avatar -->
    <div class="w-3.5 h-3.5 rounded flex items-center justify-center text-white flex-shrink-0 overflow-hidden"
         style="background-color: {workspace.color || '#3b82f6'};">
      {#if workspace.avatar || workspace.image}
        <img src={workspace.avatar || workspace.image} alt="" class="w-full h-full object-cover" />
      {:else}
        {@const WsIcon = getIconComponent(workspace.icon)}
        <WsIcon class="w-2 h-2" />
      {/if}
    </div>
    <span class="font-medium">{workspace.key ?? workspace.label ?? workspace.name ?? ''}</span>
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
            {@const WsIcon = getIconComponent(workspace.icon)}
            <WsIcon class="w-3 h-3" />
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
