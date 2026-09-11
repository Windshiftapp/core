<script>
  import Button from '../components/Button.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import Spinner from '../components/Spinner.svelte';
  import { desktopBridge } from '../desktop/bridge.svelte.js';
  import GlobalConfirmDialog from '../dialogs/GlobalConfirmDialog.svelte';
  import ToastContainer from '../features/notifications/ToastContainer.svelte';
  import FloatingTimer from '../features/time/FloatingTimer.svelte';
  import { aiStore } from '../stores/aiStore.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { getMainAppLazyState } from './mainAppRoutes.js';

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
</script>

{#if commandPaletteState.loading && showCommandPalette}
  <ModalBackdrop
    show={true}
    opacity={0.4}
    blur={8}
    extraFilter="saturate(120%)"
    zIndex={60}
    align="top"
    paddingTop="pt-[20vh]"
    closeOnClick={false}
    closeOnEscape={false}
    transition={false}
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
  <ModalBackdrop show={true} opacity={0.4} zIndex={60} closeOnClick={false} onclose={onclosecommand}>
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
  <ModalBackdrop show={true} opacity={0.4} closeOnClick={false} closeOnEscape={false} transition={false}>
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
  <ModalBackdrop show={true} opacity={0.4} closeOnClick={false} onclose={onclosecreate}>
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

{#if aiStore.chatAvailable && showChatPanel && chatPanelState.component}
  {@const ChatPanelComponent = chatPanelState.component}
  <ChatPanelComponent bind:isOpen={showChatPanel} onclose={onclosechat} />
{/if}

<ToastContainer />
