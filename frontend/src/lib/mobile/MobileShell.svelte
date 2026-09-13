<script>
  import { onMount } from 'svelte';
  import { Plus } from '@lucide/svelte';
  import { currentRoute, navigate } from '../router.js';
  import { timerStore } from '../stores/timerStore.svelte.js';
  import { workspacesStore, aiStore, homepageStore } from '../stores';
  import { startNotificationPoller, stopNotificationPoller } from '../stores/notifications.js';
  import { resetAuthenticatedShellState } from '../services/authenticatedShellBootstrap.js';
  import { registerMobileServiceWorker } from './pushClient.js';
  import MobileNav from './MobileNav.svelte';
  import GlobalConfirmDialog from '../dialogs/GlobalConfirmDialog.svelte';
  import MyWorkView from './MyWorkView.svelte';
  import PersonalView from './PersonalView.svelte';
  import TimerView from './TimerView.svelte';
  import NotificationsView from './NotificationsView.svelte';
  import MobileItemDetail from './MobileItemDetail.svelte';
  import SearchView from './SearchView.svelte';
  import MobileChatView from './MobileChatView.svelte';
  import IosInstallSheet from './IosInstallSheet.svelte';
  import MobilePagesView from './MobilePagesView.svelte';
  import MobilePageDetail from './MobilePageDetail.svelte';
  import MobileCommandPalette from './MobileCommandPalette.svelte';
  import MobileCreatePage from './MobileCreatePage.svelte';
  import MobileItemEditPage from './MobileItemEditPage.svelte';
  import ToastContainer from '../features/notifications/ToastContainer.svelte';

  const view = $derived($currentRoute.view);
  const TAB_VIEWS = ['mobile-my-work', 'mobile-personal', 'mobile-timer', 'mobile-notifications'];
  const isTabView = $derived(TAB_VIEWS.includes(view));
  // Full-screen "pushed" views (own back/cancel affordance) hide the bottom nav.
  const showNav = $derived(
    view !== 'mobile-item-detail' &&
      view !== 'mobile-search' &&
      view !== 'mobile-chat' &&
      view !== 'mobile-page-detail' &&
      view !== 'mobile-create' &&
      view !== 'mobile-item-edit',
  );
  // The Personal tab creates personal tasks; every other tab uses the full
  // work-item form. The Pages tab gets no FAB at all — pages are created from
  // the desktop editor today.
  const showFab = $derived(isTabView && view !== 'mobile-pages');
  function openCreate() {
    // Dedicated create page (/m/new), not a dialog — composition gets a real
    // route so back gestures and deep links behave like native apps.
    navigate(view === 'mobile-personal' ? '/m/new?mode=personal' : '/m/new');
  }

  onMount(() => {
    // Reuse the same singletons the desktop shell drives, so the active timer
    // and notification inbox stay live on the phone surface too.
    timerStore.initialize();
    resetAuthenticatedShellState();
    startNotificationPoller();
    registerMobileServiceWorker();
    // MainApp normally loads these; the mobile shell bypasses MainApp, so load
    // them here (stores guard re-loads) for the create dialog's workspace list
    // and the AI-chat availability gate.
    workspacesStore.load();
    aiStore.load();

    return () => {
      stopNotificationPoller();
      homepageStore.reset();
      resetAuthenticatedShellState();
    };
  });
</script>

<div class="mobile-shell" data-testid="mobile-shell">
  <main class="mobile-scroll" class:no-nav={!showNav}>
    {#if view === 'mobile-my-work'}
      <MyWorkView />
    {:else if view === 'mobile-personal'}
      <PersonalView />
    {:else if view === 'mobile-pages'}
      <MobilePagesView />
    {:else if view === 'mobile-page-detail'}
      <MobilePageDetail
        workspaceId={Number($currentRoute.params.workspaceId)}
        pageId={Number($currentRoute.params.pageId)}
      />
    {:else if view === 'mobile-timer'}
      <TimerView />
    {:else if view === 'mobile-notifications'}
      <NotificationsView />
    {:else if view === 'mobile-search'}
      <SearchView />
    {:else if view === 'mobile-chat'}
      <MobileChatView />
    {:else if view === 'mobile-item-detail'}
      <MobileItemDetail itemId={Number($currentRoute.params.id)} />
    {:else if view === 'mobile-create'}
      <MobileCreatePage />
    {:else if view === 'mobile-item-edit'}
      <MobileItemEditPage />
    {/if}
  </main>

  {#if showFab}
    <button class="fab" onclick={openCreate} data-testid="mobile-create-fab" aria-label="Create item" type="button">
      <Plus size={26} />
    </button>
  {/if}

  {#if showNav}
    <MobileNav />
  {/if}
</div>

<!-- Global confirm host (the mobile shell bypasses MainApp, which normally
     mounts this) so confirm() dialogs render on the mobile surface. -->
<GlobalConfirmDialog />

<!-- iOS "Add to Home Screen" instructions (opened from the user menu via the
     install helper's store; no-op until triggered). -->
<IosInstallSheet />
<!-- Command palette sheet. Self-mounted from the shared mobilePalette store,
     so any header button can open it without prop plumbing. -->
<MobileCommandPalette />
<ToastContainer />

<style>
  .mobile-shell {
    display: flex;
    flex-direction: column;
    height: 100dvh;
    width: 100%;
    background-color: var(--ds-surface);
    color: var(--ds-text);
    overflow: hidden;
  }

  .mobile-scroll {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
    /* Clear the fixed bottom nav + iPhone home indicator. */
    padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 4rem);
  }
  /* Pushed full-screen views (chat/search/detail) have no bottom nav; the chat
     composer manages its own safe-area padding. */
  .mobile-scroll.no-nav { padding-bottom: 0; }

  .fab {
    position: fixed;
    right: 1rem;
    /* Sit just above the bottom nav + home indicator. */
    bottom: calc(env(safe-area-inset-bottom, 0px) + 4.5rem);
    z-index: 40;
    width: 52px;
    height: 52px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    border-radius: var(--radius-full, 9999px);
    background-color: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
    box-shadow: var(--shadow-float, 0 6px 16px rgba(0, 0, 0, 0.28));
    cursor: pointer;
  }
  .fab:active { background-color: var(--ds-interactive-pressed, var(--ds-interactive-hovered, var(--ds-interactive))); }
</style>
