<script>
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { viewSettingsStore } from '../../stores/viewSettings.svelte.js';
  import { collectionStore } from '../../stores/collectionContext.svelte.js';
  import { authStore } from '../../stores/auth.svelte.js';
  import SquareKanban from '@lucide/svelte/icons/square-kanban';
  import Inbox from '@lucide/svelte/icons/inbox';
  import Settings from '@lucide/svelte/icons/settings';
  import Globe from '@lucide/svelte/icons/globe';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';

  // Props
  let {
    workspaceId,
    collectionId = null,
    activeView = 'board',
    publicSlug = null,
  } = $props();

  // Keep the enabled-views lookup warm; board/backlog hide when disabled.
  $effect(() => {
    if (workspaceId || collectionId) {
      viewSettingsStore.load(workspaceId, collectionId);
    }
  });
  const boardEnabled = $derived(viewSettingsStore.enabledViewIds(workspaceId, collectionId).includes('board'));
  const backlogEnabled = $derived(viewSettingsStore.enabledViewIds(workspaceId, collectionId).includes('backlog'));

  // Views follow collection-editing rights: the workspace-default context is
  // workspace-admin territory, while a collection's views belong to whoever
  // can edit the collection (its creator) — mirroring the backend gates.
  const isCollectionCreator = $derived(Boolean(
    collectionStore.boardCollection &&
      authStore.currentUser?.id != null &&
      String(collectionStore.boardCollection.created_by) === String(authStore.currentUser.id)
  ));
  let canConfigure = $derived(
    collectionId ? isCollectionCreator : workspacePermissions.canAdminWorkspace(workspaceId)
  );

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

  function goToNavConfig() {
    const url = workspaceId
      ? `/workspaces/${workspaceId}/collections/${collectionId}/nav-config`
      : `/collections/${collectionId}/nav-config`;
    navigate(url);
  }
</script>

<div class="flex rounded p-1" style={containerStyle} data-testid="board-view-switcher">
  <!-- Board Button -->
  {#if boardEnabled}
  <button
    class="view-btn px-3 py-1.5 text-sm font-medium rounded transition-colors"
    class:shadow-sm={activeView === 'board'}
    class:active={activeView === 'board'}
    data-testid="switcher-view-board"
    onclick={activeView !== 'board' ? goToBoard : undefined}
  >
    <div class="flex items-center gap-2">
      <SquareKanban class="w-4 h-4" />
      {t('collections.board')}
    </div>
  </button>
  {/if}

  <!-- Backlog Button -->
  {#if backlogEnabled}
  <button
    class="view-btn px-3 py-1.5 text-sm font-medium rounded transition-colors"
    class:shadow-sm={activeView === 'backlog'}
    class:active={activeView === 'backlog'}
    data-testid="switcher-view-backlog"
    onclick={activeView !== 'backlog' ? goToBacklog : undefined}
  >
    <div class="flex items-center gap-2">
      <Inbox class="w-4 h-4" />
      {t('collections.backlog')}
    </div>
  </button>
  {/if}

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

  <!-- Navigation visibility entry point for collection scopes. -->
  {#if collectionId && canConfigure}
    <button
      class="view-btn px-2 py-1.5 text-sm font-medium rounded transition-colors"
      title={t('navConfig.configureTitle')}
      aria-label={t('navConfig.configureTitle')}
      data-testid="collection-nav-config-button"
      onclick={goToNavConfig}
    >
      <div class="flex items-center gap-2">
        <Settings class="w-4 h-4" />
        <span class="sr-only">{t('navConfig.configureTitle')}</span>
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
