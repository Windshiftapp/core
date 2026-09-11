<script>
  import { onMount } from 'svelte';
  import { currentRoute } from '../router.js';
  import { currentWorkspace, workspacesStore } from '../stores';
  import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';
  import { CheckSquare } from '@lucide/svelte';
  import TodoList from '../features/items/TodoList.svelte';
  import PageHeader from '../layout/PageHeader.svelte';

  let { workspaceId } = $props();

  let workspace = $state(null);
  let loading = $state(true);

  // Derived workspace ID: use personal workspace from store if on personal route
  let effectiveWorkspaceId = $derived(
    $currentRoute.path?.startsWith('/personal')
      ? $workspacesStore.personalWorkspace?.id
      : workspaceId
  );

  onMount(async () => {
    // Wait for personal workspace to load if on personal route
    if ($currentRoute.path?.startsWith('/personal')) {
      await workspacesStore.loadPersonalWorkspace();
    }

    if (effectiveWorkspaceId) {
      await loadWorkspace();
    }
    loading = false;
  });

  async function loadWorkspace() {
    try {
      await workspaceDataStore.initialize(effectiveWorkspaceId);
      workspace = workspaceDataStore.workspace;
      currentWorkspace.hydrate(workspace);
    } catch (error) {
      console.error('Failed to load workspace:', error);
    }
  }
</script>

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">Loading...</div>
  </div>
{:else if workspace}
  {#if workspace.is_personal}
    <!-- Personal Todo Workspace - Simplified Interface -->
    <div class="p-6" style="background-color: var(--ds-surface); min-height: 100vh;">
      <PageHeader
        icon={CheckSquare}
        title={workspace.name}
        subtitle="Personal task management"
      >
        {#snippet actions()}
          <span class="inline-flex items-center px-2 py-1 rounded text-xs font-medium bg-orange-100 text-orange-800">
            Personal
          </span>
        {/snippet}
      </PageHeader>

      <!-- Todo List Interface -->
      <TodoList workspaceId={effectiveWorkspaceId} />
    </div>
  {/if}
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      Workspace not found.
    </div>
  </div>
{/if}
