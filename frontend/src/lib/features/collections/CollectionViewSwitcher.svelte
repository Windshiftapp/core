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
  import ButtonGroup from '../../components/ButtonGroup.svelte';
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

<ButtonGroup testid="board-view-switcher">
  {#snippet children(segment)}
    <!-- Board Button -->
    {#if boardEnabled}
      <button
        class={segment}
        data-active={activeView === 'board'}
        aria-current={activeView === 'board' ? 'page' : undefined}
        data-testid="switcher-view-board"
        onclick={activeView !== 'board' ? goToBoard : undefined}
      >
        <SquareKanban class="w-4 h-4" />
        {t('collections.board')}
      </button>
    {/if}

    <!-- Backlog Button -->
    {#if backlogEnabled}
      <button
        class={segment}
        data-active={activeView === 'backlog'}
        aria-current={activeView === 'backlog' ? 'page' : undefined}
        data-testid="switcher-view-backlog"
        onclick={activeView !== 'backlog' ? goToBacklog : undefined}
      >
        <Inbox class="w-4 h-4" />
        {t('collections.backlog')}
      </button>
    {/if}

    <!-- Configure Button -->
    {#if canConfigure}
      <button
        class={segment}
        data-active={activeView === 'configure'}
        aria-current={activeView === 'configure' ? 'page' : undefined}
        onclick={activeView !== 'configure' ? goToConfigure : undefined}
      >
        <Settings class="w-4 h-4" />
        {t('collections.configure')}
      </button>
    {/if}

    {#if publicSlug}
      <a
        href="/board/{publicSlug}"
        target="_blank"
        rel="noopener noreferrer"
        class={segment}
        title={t('collections.publicBoard')}
        onclick={(e) => { e.stopPropagation(); window.open(`/board/${publicSlug}`, '_blank'); e.preventDefault(); }}
      >
        <Globe class="w-4 h-4" />
        {t('collections.publicBoard')}
      </a>
    {/if}
  {/snippet}
</ButtonGroup>

