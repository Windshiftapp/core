<script>
  import { Grip, Briefcase, Plus } from '@lucide/svelte';
  import { homepageStore } from '../../stores';
  import { workspaceIconMap } from '../../utils/icons.js';
  import { navigate } from '../../router.js';

  let workspaces = $derived(homepageStore.accessibleWorkspaces.filter(ws => ws.active));
  let canCreate = $derived(homepageStore.canCreateWorkspaces);

  function iconFor(name) {
    return workspaceIconMap[name] || Briefcase;
  }

  function go(workspace) {
    navigate(`/workspaces/${workspace.id}`);
  }
</script>

{#if workspaces.length === 0}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <Grip class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">No workspaces yet</p>
    {#if canCreate}
      <button
        class="mt-3 inline-flex items-center gap-1 text-xs font-medium px-2 py-1 rounded"
        style="color: var(--ds-link);"
        onclick={() => navigate('/workspaces/new')}
      >
        <Plus class="w-3.5 h-3.5" /> Create one
      </button>
    {/if}
  </div>
{:else}
  <div class="grid grid-cols-2 gap-2">
    {#each workspaces.slice(0, 8) as workspace (workspace.id)}
      {@const Icon = iconFor(workspace.icon)}
      <button
        class="flex items-center gap-2 p-2 rounded border transition-colors text-left"
        style="border-color: var(--ds-border); background-color: var(--ds-surface);"
        onmouseenter={(e) => (e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)')}
        onmouseleave={(e) => (e.currentTarget.style.backgroundColor = 'var(--ds-surface)')}
        onclick={() => go(workspace)}
      >
        <div
          class="w-7 h-7 rounded flex items-center justify-center flex-shrink-0"
          style={`background-color: ${workspace.color || 'var(--color-blue-500)'}; color: white;`}
        >
          <Icon class="w-3.5 h-3.5" />
        </div>
        <span class="text-xs font-medium truncate" style="color: var(--ds-text);">
          {workspace.name || workspace.workspace_name || 'Workspace'}
        </span>
      </button>
    {/each}
  </div>
{/if}
