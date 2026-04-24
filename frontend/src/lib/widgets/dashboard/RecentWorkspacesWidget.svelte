<script>
  import { Briefcase } from 'lucide-svelte';
  import { homepageStore } from '../../stores';
  import { workspaceIconMap } from '../../utils/icons.js';
  import { navigate } from '../../router.js';

  let workspaces = $derived(homepageStore.recentWorkspaces);
  let loading = $derived(homepageStore.loading);

  function iconFor(name) {
    return workspaceIconMap[name] || Briefcase;
  }

  function go(workspace) {
    navigate(`/workspaces/${workspace.workspace_id}`);
  }
</script>

{#if loading && workspaces.length === 0}
  <div class="space-y-2 animate-pulse">
    {#each Array(3) as _}
      <div class="h-11 rounded" style="background-color: var(--ds-background-neutral);"></div>
    {/each}
  </div>
{:else if workspaces.length === 0}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <Briefcase class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">No recent workspaces</p>
  </div>
{:else}
  <ul class="flex flex-col gap-1.5">
    {#each workspaces as ws (ws.workspace_id)}
      {@const Icon = iconFor(ws.icon)}
      <li>
        <button
          class="w-full flex items-center gap-3 p-2 rounded border transition-colors text-left"
          style="border-color: var(--ds-border); background-color: var(--ds-surface);"
          onmouseenter={(e) => (e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)')}
          onmouseleave={(e) => (e.currentTarget.style.backgroundColor = 'var(--ds-surface)')}
          onclick={() => go(ws)}
        >
          <div
            class="w-8 h-8 rounded flex items-center justify-center flex-shrink-0"
            style={`background-color: ${ws.color || 'var(--color-blue-500)'}; color: white;`}
          >
            <svelte:component this={Icon} class="w-4 h-4" />
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-sm font-medium truncate" style="color: var(--ds-text);">
              {ws.workspace_name}
            </p>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              {ws.workspace_key}
              {#if ws.last_visited}
                · visited {homepageStore.formatRelativeTime(ws.last_visited)}
              {/if}
            </p>
          </div>
        </button>
      </li>
    {/each}
  </ul>
{/if}
