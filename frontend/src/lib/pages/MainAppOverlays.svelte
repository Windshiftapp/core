<script>
  import Button from '../components/Button.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import Spinner from '../components/Spinner.svelte';
  import { desktopBridge } from '../desktop/bridge.svelte.js';
  import GlobalConfirmDialog from '../dialogs/GlobalConfirmDialog.svelte';
  import ToastContainer from '../features/notifications/ToastContainer.svelte';
  import FloatingTimer from '../features/time/FloatingTimer.svelte';
  import TimeLogHost from '../features/time/TimeLogHost.svelte';
  import { aiStore } from '../stores/aiStore.svelte.js';
  import { itemModalStore } from '../stores/itemModalStore.svelte.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { surfaceError } from '../stores/toasts.svelte.js';
  import { currentRoute } from '../router.js';
  import { getMainAppLazyState, isPersonalTaskWorkspace } from './mainAppRoutes.js';

  let {
    lazyComponents,
    showCommandPalette = $bindable(false),
    showCreateModal = $bindable(false),
    showChatPanel = $bindable(false),
    createModalInitialType = 'work-item',
    createModalWorkspaceId = null,
    createModalParentContext = null,
    createModalSkipNavigate = false,
    onclosecreate,
    onclosecommand = () => {},
    onclosechat = () => {},
  } = $props();

  let PomodoroSettingsModalComponent = $state(null);
  let AboutModalComponent = $state(null);
  let ItemDetailModalComponent = $state(null);
  let PersonalTaskDetailModalComponent = $state(null);
  let desktopModalLoading = $state(false);

  const commandPaletteState = $derived.by(() =>
    getMainAppLazyState(lazyComponents, 'command-palette')
  );
  const createModalState = $derived.by(() =>
    getMainAppLazyState(lazyComponents, 'create-modal')
  );
  const chatPanelState = $derived.by(() => getMainAppLazyState(lazyComponents, 'chat-panel'));

  async function loadDesktopModal(modal) {
    if (desktopModalLoading) return;
    if (modal === 'pomodoro-settings' && PomodoroSettingsModalComponent) return;
    if (modal === 'about' && AboutModalComponent) return;

    desktopModalLoading = true;
    try {
      if (modal === 'pomodoro-settings') {
        const module = await import('../dialogs/PomodoroSettingsModal.svelte');
        PomodoroSettingsModalComponent = module.default;
      } else if (modal === 'about') {
        const module = await import('../dialogs/AboutModal.svelte');
        AboutModalComponent = module.default;
      }
    } catch (error) {
      console.error('Failed to load desktop modal:', error);
    } finally {
      desktopModalLoading = false;
    }
  }

  $effect(() => {
    if (showCommandPalette) void lazyComponents.load('command-palette');
    if (showCreateModal) void lazyComponents.load('create-modal');
    if (showChatPanel) void lazyComponents.load('chat-panel');
  });

  $effect(() => {
    if (desktopBridge.modal) void loadDesktopModal(desktopBridge.modal);
  });

  async function loadItemDetailModal() {
    if (ItemDetailModalComponent) return;
    try {
      const module = await import('../features/items/ItemDetail.svelte');
      ItemDetailModalComponent = module.default;
    } catch (error) {
      console.error('Failed to load item detail modal:', error);
      // Avoid leaving the loading backdrop stuck if the chunk cannot load.
      itemModalStore.close();
    }
  }

  async function loadPersonalTaskDetailModal() {
    if (PersonalTaskDetailModalComponent) return;
    try {
      const module = await import('../features/personal/PersonalTaskDetail.svelte');
      PersonalTaskDetailModalComponent = module.default;
    } catch (error) {
      surfaceError(error, { context: 'loadPersonalTaskDetailModal' });
      itemModalStore.close();
    }
  }

  // The modal host owns the task/work-item split, mirroring the route-level
  // resolveEffectiveMainAppView. Callers only report the item and its
  // workspace, so no opener has to know that personal-workspace items are
  // tasks.
  const personalWorkspaceId = $derived($workspacesStore.personalWorkspace?.id ?? null);
  const modalOpensPersonalTask = $derived(
    itemModalStore.isOpen &&
      isPersonalTaskWorkspace(itemModalStore.workspaceId, personalWorkspaceId)
  );
  const modalComponentReady = $derived(
    modalOpensPersonalTask
      ? Boolean(PersonalTaskDetailModalComponent)
      : Boolean(ItemDetailModalComponent)
  );

  // The personal workspace loads on demand. Until it arrives a non-null
  // workspace id cannot be classified, so hold the backdrop instead of
  // rendering the work-item detail and swapping it out mid-load.
  let personalWorkspaceChecked = $state(false);
  let personalWorkspaceCheckInFlight = false;

  $effect(() => {
    if (!itemModalStore.isOpen) {
      personalWorkspaceChecked = false;
      return;
    }
    if (itemModalStore.workspaceId == null || personalWorkspaceId != null) return;
    if (personalWorkspaceCheckInFlight) return;
    personalWorkspaceCheckInFlight = true;
    void workspacesStore.loadPersonalWorkspace().finally(() => {
      personalWorkspaceCheckInFlight = false;
      personalWorkspaceChecked = true;
    });
  });

  const modalWorkspaceUnresolved = $derived(
    itemModalStore.isOpen &&
      itemModalStore.workspaceId != null &&
      personalWorkspaceId == null &&
      !personalWorkspaceChecked
  );

  $effect(() => {
    if (!itemModalStore.isOpen || modalWorkspaceUnresolved) return;
    if (modalOpensPersonalTask) void loadPersonalTaskDetailModal();
    else void loadItemDetailModal();
  });

  // The host outlives the view that opened it; close on route changes so the
  // modal cannot float above an unrelated destination.
  let lastRoutePath = null;
  $effect(() => {
    const path = $currentRoute.path;
    if (lastRoutePath !== null && path !== lastRoutePath) itemModalStore.close();
    lastRoutePath = path;
  });
</script>

{#if commandPaletteState.loading && showCommandPalette}
  <ModalBackdrop
    show={true}
    ariaLabel={t('nav.loadingSearch')}
    opacity={0.4}
    blur={8}
    extraFilter="saturate(120%)"
    zIndex={60}
    align="top"
    paddingTop="pt-[20vh]"
    closeOnClick={false}
    closeOnEscape={false}
  >
    <div class="rounded-lg p-6" style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);">
      <Spinner class="mx-auto mb-4" />
      <p>{t('nav.loadingSearch')}</p>
    </div>
  </ModalBackdrop>
{:else if commandPaletteState.component && showCommandPalette}
  {@const CommandPalette = commandPaletteState.component}
  <CommandPalette bind:isOpen={showCommandPalette} onclose={onclosecommand} />
{:else if commandPaletteState.error && showCommandPalette}
  <!-- shortcut-guard-exempt: retrying a failed lazy import is a recovery action, not a form submission. -->
  <ModalBackdrop show={true} ariaLabel="Failed to load Search" opacity={0.4} zIndex={60} closeOnClick={false} onclose={onclosecommand}>
    <div class="rounded-lg p-6 text-center" role="alert" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
      <p class="font-semibold">Failed to load Search</p>
      <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">Check your connection, then try again.</p>
      <div class="mt-4 flex justify-center gap-2">
        <Button variant="secondary" onclick={onclosecommand}>{t('common.close')}</Button>
        <Button variant="primary" onclick={() => lazyComponents.retry('command-palette')}>{t('nav.retry')}</Button>
      </div>
    </div>
  </ModalBackdrop>
{/if}

{#if createModalState.loading && showCreateModal}
  <ModalBackdrop show={true} ariaLabel={t('nav.loadingCreateForm')} opacity={0.4} closeOnClick={false} closeOnEscape={false}>
    <div class="rounded-lg p-6" style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);">
      <Spinner class="mx-auto mb-4" />
      <p>{t('nav.loadingCreateForm')}</p>
    </div>
  </ModalBackdrop>
{:else if createModalState.component && showCreateModal}
  {@const CreateModal = createModalState.component}
  <CreateModal
    bind:isOpen={showCreateModal}
    initialType={createModalInitialType}
    initialWorkspaceId={createModalWorkspaceId}
    initialParentContext={createModalParentContext}
    skipNavigate={createModalSkipNavigate}
    onclose={onclosecreate}
  />
{:else if createModalState.error && showCreateModal}
  <!-- shortcut-guard-exempt: retrying a failed lazy import is a recovery action, not a form submission. -->
  <ModalBackdrop show={true} ariaLabel="Failed to load Create Form" opacity={0.4} closeOnClick={false} onclose={onclosecreate}>
    <div class="rounded-lg p-6 text-center" role="alert" data-testid="create-modal-load-error" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
      <p class="font-semibold">Failed to load Create Form</p>
      <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">Check your connection, then try again.</p>
      <div class="mt-4 flex justify-center gap-2">
        <Button variant="secondary" onclick={onclosecreate}>{t('common.close')}</Button>
        <Button variant="primary" onclick={() => lazyComponents.retry('create-modal')}>{t('nav.retry')}</Button>
      </div>
    </div>
  </ModalBackdrop>
{/if}

<GlobalConfirmDialog />

{#if desktopBridge.modal === 'pomodoro-settings' && PomodoroSettingsModalComponent}
  <PomodoroSettingsModalComponent show={true} onclose={() => desktopBridge.close()} />
{:else if desktopBridge.modal === 'about' && AboutModalComponent}
  <AboutModalComponent show={true} onclose={() => desktopBridge.close()} />
{/if}

<FloatingTimer />
<TimeLogHost />

{#if itemModalStore.isOpen}
  {#if modalWorkspaceUnresolved || !modalComponentReady}
    <ModalBackdrop show={true} ariaLabel={t('items.workItemDetails')} opacity={0.4} closeOnClick={false} closeOnEscape={false}>
      <div class="rounded-lg p-6" style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);">
        <Spinner class="mx-auto mb-4" />
        <p>{t('common.loading')}</p>
      </div>
    </ModalBackdrop>
  {:else if modalOpensPersonalTask}
    <PersonalTaskDetailModalComponent
      isModal={true}
      itemId={itemModalStore.itemId}
      workspaceId={itemModalStore.workspaceId}
      onclose={() => itemModalStore.close()}
    />
  {:else}
    <ItemDetailModalComponent
      isModal={true}
      itemId={itemModalStore.itemId}
      workspaceId={itemModalStore.workspaceId}
      onclose={() => itemModalStore.close()}
    />
  {/if}
{/if}

{#if aiStore.chatAvailable && showChatPanel && chatPanelState.component}
  {@const ChatPanelComponent = chatPanelState.component}
  <ChatPanelComponent bind:isOpen={showChatPanel} onclose={onclosechat} />
{/if}

<ToastContainer />
