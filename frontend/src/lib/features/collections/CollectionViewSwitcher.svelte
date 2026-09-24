<script>
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { SquareKanban, Inbox, Settings, Globe } from '@lucide/svelte';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';

  // Props
  let {
    workspaceId,
    collectionId = null,
    activeView = 'board',
    publicSlug = null,
  } = $props();

  // Configure view writes the workspace-default board configuration, which
  // the backend gates to `workspace.admin` — hide the entry point for
  // non-admins so they don't reach a page they can't save.
  let canConfigure = $derived(workspacePermissions.canAdminWorkspace(workspaceId));

  // Styles use --ctx-* CSS vars cascaded from parent collection view
  const containerStyle = 'background-color: var(--ctx-surface, var(--ds-background-neutral)); backdrop-filter: var(--ctx-backdrop, none);';

  // Navigation functions
  function goToBoard() {
    const url = workspaceId
      ? (collectionId ? `/workspaces/${workspaceId}/collections/${collectionId}/board` : `/workspaces/${workspaceId}/board`)
      : `/collections/${collectionId}/board`;
    navigate(url);
  }

  function goToBacklog() {
    const url = workspaceId
      ? (collectionId ? `/workspaces/${workspaceId}/collections/${collectionId}/backlog` : `/workspaces/${workspaceId}/backlog`)
      : `/collections/${collectionId}/backlog`;
    navigate(url);
  }

  function goToConfigure() {
    const url = workspaceId
      ? (collectionId ? `/workspaces/${workspaceId}/collections/${collectionId}/board/configure` : `/workspaces/${workspaceId}/board/configure`)
      : `/collections/${collectionId}/board/configure`;
    navigate(url);
  }
</script>

<div class="flex rounded p-1" style={containerStyle} data-testid="board-view-switcher">
  <!-- Board Button -->
  <button
    class="view-btn px-3 py-1.5 text-sm font-medium rounded transition-colors"
    class:shadow-sm={activeView === 'board'}
    class:active={activeView === 'board'}
    onclick={activeView !== 'board' ? goToBoard : undefined}
  >
    <div class="flex items-center gap-2">
      <SquareKanban class="w-4 h-4" />
      {t('collections.board')}
    </div>
  </button>

  <!-- Backlog Button -->
  <button
    class="view-btn px-3 py-1.5 text-sm font-medium rounded transition-colors"
    class:shadow-sm={activeView === 'backlog'}
    class:active={activeView === 'backlog'}
    onclick={activeView !== 'backlog' ? goToBacklog : undefined}
  >
    <div class="flex items-center gap-2">
      <Inbox class="w-4 h-4" />
      {t('collections.backlog')}
    </div>
  </button>

  <!-- Configure Button -->
  {#if canConfigure}
    <button
      class="view-btn px-3 py-1.5 text-sm font-medium rounded transition-colors"
      class:shadow-sm={activeView === 'configure'}
      class:active={activeView === 'configure'}
      onclick={activeView !== 'configure' ? goToConfigure : undefined}
    >
      <div class="flex items-center gap-2">
        <Settings class="w-4 h-4" />
        {t('collections.configure')}
      </div>
    </button>
  {/if}

  {#if publicSlug}
    <a
      href="/board/{publicSlug}"
      target="_blank"
      rel="noopener noreferrer"
      class="view-btn px-3 py-1.5 text-sm font-medium rounded transition-colors"
      title={t('collections.publicBoard')}
      onclick={(e) => { e.stopPropagation(); window.open(`/board/${publicSlug}`, '_blank'); e.preventDefault(); }}
    >
      <div class="flex items-center gap-2">
        <Globe class="w-4 h-4" />
        {t('collections.publicBoard')}
      </div>
    </a>
  {/if}
</div>

<style>
  .view-btn {
    color: var(--ds-text);
  }

  .view-btn:hover:not(.active) {
    background-color: var(--ctx-surface, var(--ds-background-neutral-hovered));
  }

  .view-btn.active {
    background-color: var(--ctx-surface-raised, var(--ds-surface-raised));
  }
</style>
