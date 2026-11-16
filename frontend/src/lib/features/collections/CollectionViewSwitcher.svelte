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
  import { isSystemAdmin } from '../../stores/permissions.svelte.js';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import BoardSharingEditor from './BoardSharingEditor.svelte';
  let showSharing = $state(false);
  let savedSlug = $state(undefined);

  // Props
  let {
    workspaceId,
    collectionId = null,
    activeView = 'board',
    publicSlug = null,
  } = $props();

  $effect(() => { collectionId; savedSlug = undefined; showSharing = false; });
  const visibleSlug = $derived(savedSlug === undefined ? publicSlug : savedSlug);

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
        data-testid="switcher-view-configure"
        onclick={activeView !== 'configure' ? goToConfigure : undefined}
      >
        <Settings class="w-4 h-4" />
        {t('collections.configure')}
      </button>
    {/if}

    {#if collectionId && (isCollectionCreator || $isSystemAdmin)}
      <button type="button" class={segment} data-testid="board-share" onclick={() => showSharing = true}><Globe class="w-4 h-4" />Share board</button>
    {/if}
    {#if visibleSlug}
      <a
        href="/board/{visibleSlug}"
        target="_blank"
        rel="noopener noreferrer"
        class={segment}
        title="Open shared board"
      >
        <Globe class="w-4 h-4" />
        Open shared board
      </a>
    {/if}
  {/snippet}
</ButtonGroup>

{#if showSharing}
  <Modal isOpen={true} onclose={() => showSharing = false} maxWidth="max-w-xl" closeOnBackdropClick={false}>
    <ModalHeader title="Share board" onclose={() => showSharing = false} />
    <div class="px-6 py-5"><BoardSharingEditor {collectionId} oncancel={() => showSharing = false} onsaved={(value, close) => { savedSlug = value.enabled && value.standalone_enabled && !value.revoked_at ? value.slug : null; if (close) showSharing = false; }} /></div>
  </Modal>
{/if}
