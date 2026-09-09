<script>
  import { slide } from 'svelte/transition';
  import { Menu } from '@lucide/svelte';
  import { useEventListener } from 'runed';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import EmailVerificationBanner from '../features/notifications/EmailVerificationBanner.svelte';
  import CollectionNavigation from '../features/collections/CollectionNavigation.svelte';
  import Footer from '../layout/Footer.svelte';
  import MainSidebar from '../layout/MainSidebar.svelte';
  import {
    GLOBAL_COLLECTION_VIEWS,
    currentRoute,
    isWorkspaceRoute,
    navigate,
  } from '../router.js';
  import { hydrateCurrentWorkspaceFromSharedData } from '../services/authenticatedShellUI.js';
  import {
    authStore,
    currentWorkspace,
    uiStore,
    workspaceDataStore,
    workspacesStore,
  } from '../stores';
  import { aiStore } from '../stores/aiStore.svelte.js';
  import { terminalStore } from '../stores/terminalStore.svelte.js';
  import { clearWorkspaceGradient } from '../stores/workspaceGradient.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import { LazyComponentLoader } from '../utils/lazyComponentLoader.svelte.js';
  import { hasSessionExpired, reloadIfBuildChanged } from '../utils/lazyLoadRecovery.js';
  import WorkspaceNavigation from '../workspaces/WorkspaceNavigation.svelte';
  import WorkspaceBreadcrumbs from '../workspaces/WorkspaceBreadcrumbs.svelte';
  import MainAppOverlays from './MainAppOverlays.svelte';
  import MainRouteContent from './MainRouteContent.svelte';
  import { useMainAppLifecycle } from './useMainAppLifecycle.js';
  import {
    CREATE_MODAL_WORKSPACE_VIEWS,
    MAIN_APP_COMPONENT_LOADERS,
    MAIN_APP_TEST_VIEWS,
    resolveEffectiveMainAppView,
  } from './mainAppRoutes.js';
  import {
    getMainAppWorkspaceRedirect,
    resolveMainAppWorkspaceContext,
  } from './mainAppWorkspaceContext.js';

  let showCommandPalette = $state(false);
  let showCreateModal = $state(false);
  let showChatPanel = $state(false);
  let activeSidebarSurface = $state(null);
  let createModalInitialType = $state('work-item');
  let createModalSkipNavigate = $state(false);
  let createModalWorkspaceId = $state(null);
  let createModalParentContext = $state(null);
  let showEmailVerificationBanner = $state(false);
  let mobileWorkspaceNavOpen = $state(false);

  let terminalState = $derived($terminalStore);
  let TerminalPanelComponent = $state(null);
  let terminalLoading = $state(false);
  let isResizingTerminal = $state(false);
  let resizeStartX = $state(0);
  let resizeStartPercent = $state(50);

  let sessionRevalidationPromise = null;

  const lazyComponents = new LazyComponentLoader(MAIN_APP_COMPONENT_LOADERS, {
    onError: (view, error) => {
      console.error(`Failed to load component for ${view}:`, error);
      void recoverFromLazyLoadFailure();
    },
  });

  async function loadTerminalPanel() {
    if (TerminalPanelComponent || terminalLoading) return;
    terminalLoading = true;
    try {
      const module = await import('../features/terminal/TerminalPanel.svelte');
      TerminalPanelComponent = module.default;
    } catch (error) {
      console.error('Failed to load terminal panel:', error);
    } finally {
      terminalLoading = false;
    }
  }

  function toggleTerminal() {
    closeAllSidebarSurfaces();
    terminalStore.toggle();
    if (!TerminalPanelComponent) void loadTerminalPanel();
  }

  function handleTerminalResizeStart(event) {
    event.preventDefault();
    resizeStartX = event.clientX;
    resizeStartPercent = terminalState.splitPercent;
    isResizingTerminal = true;
  }

  function onTerminalMouseMove(event) {
    const container = document.querySelector('.main-split-container');
    if (!container) return;
    const rect = container.getBoundingClientRect();
    const deltaX = event.clientX - resizeStartX;
    terminalStore.setSplitPercent(resizeStartPercent + (deltaX / rect.width) * 100);
  }

  function onTerminalMouseUp() {
    isResizingTerminal = false;
  }

  useEventListener(() => (isResizingTerminal ? document : null), 'mousemove', onTerminalMouseMove);
  useEventListener(() => (isResizingTerminal ? document : null), 'mouseup', onTerminalMouseUp);

  $effect(() => {
    if (!isResizingTerminal) return;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    return () => {
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
  });

  async function recoverFromLazyLoadFailure() {
    if (!authStore.isAuthenticated) return;

    sessionRevalidationPromise ??= hasSessionExpired(api.auth.getCurrentUser);
    const sessionExpired = await sessionRevalidationPromise;
    sessionRevalidationPromise = null;

    if (sessionExpired) {
      authStore.clearAuth();
      showCommandPalette = false;
      closeCreateModal();
      showChatPanel = false;
      return;
    }

    await reloadIfBuildChanged();
  }

  function closeCreateModal() {
    showCreateModal = false;
    if (activeSidebarSurface === 'create') activeSidebarSurface = null;
    createModalInitialType = 'work-item';
    createModalSkipNavigate = false;
    createModalWorkspaceId = null;
    createModalParentContext = null;
  }

  function closeAllSidebarSurfaces() {
    activeSidebarSurface = null;
    showCommandPalette = false;
    showCreateModal = false;
    showChatPanel = false;
    createModalInitialType = 'work-item';
    createModalSkipNavigate = false;
    createModalWorkspaceId = null;
    createModalParentContext = null;
  }

  function activateSidebarSurface(surface) {
    const shouldClose = activeSidebarSurface === surface;
    closeAllSidebarSurfaces();
    if (shouldClose) return;

    activeSidebarSurface = surface;
    showCommandPalette = surface === 'search';
    showCreateModal = surface === 'create';
    showChatPanel = surface === 'chat';
  }

  function handleSidebarSurfaceChange(surface) {
    if (surface === null) {
      if (['workspaces', 'notifications', 'profile'].includes(activeSidebarSurface)) {
        activeSidebarSurface = null;
      }
      return;
    }
    activateSidebarSurface(surface);
  }

  function closeCommandPalette() {
    showCommandPalette = false;
    if (activeSidebarSurface === 'search') activeSidebarSurface = null;
  }

  function closeChatPanel() {
    showChatPanel = false;
    if (activeSidebarSurface === 'chat') activeSidebarSurface = null;
  }

  function showCreateDropdown() {
    closeAllSidebarSurfaces();
    createModalWorkspaceId = null;
    createModalParentContext = null;
    const currentWorkspaceId = $currentRoute.params?.id;
    if (currentWorkspaceId && CREATE_MODAL_WORKSPACE_VIEWS.has($currentRoute.view)) {
      createModalWorkspaceId = Number.parseInt(currentWorkspaceId, 10);
    }
    activeSidebarSurface = 'create';
    showCreateModal = true;
  }

  function showCreateModalFromEvent(detail) {
    closeAllSidebarSurfaces();
    if (detail.type) createModalInitialType = detail.type;
    createModalSkipNavigate = detail.skipNavigate || false;
    createModalParentContext = detail.parentContext ?? null;
    createModalWorkspaceId = detail.workspaceId
      ? Number.parseInt(String(detail.workspaceId), 10)
      : null;
    activeSidebarSurface = 'create';
    showCreateModal = true;
  }

  useMainAppLifecycle({
    onEmailVerificationChange: (show) => showEmailVerificationBanner = show,
    onShowCommandPalette: () => activateSidebarSurface('search'),
    onShowCreateModal: showCreateModalFromEvent,
  });

  const effectiveView = $derived(
    resolveEffectiveMainAppView($currentRoute, $workspacesStore.personalWorkspace?.id)
  );
  // Route-only predicate for surfaces that frame workspace content. The
  // breadcrumb bar must not wait for workspace hydration: mounting it late
  // shifts the whole app content down and breaks scroll positions.
  const onWorkspaceSurfaceRoute = $derived(
    $currentRoute.view !== 'workspaces' &&
      (isWorkspaceRoute($currentRoute.view) ||
        effectiveView === 'personal-task-detail' ||
        MAIN_APP_TEST_VIEWS.has($currentRoute.view))
  );
  const showWorkspaceNav = $derived(
    !$uiStore.reviewFullscreen && !!$currentWorkspace && onWorkspaceSurfaceRoute
  );
  const showWorkspaceBreadcrumbs = $derived(
    !$uiStore.reviewFullscreen &&
      (onWorkspaceSurfaceRoute || ['homepage', 'workspaces'].includes($currentRoute.view))
  );
  const showCollectionNav = $derived(
    !$uiStore.reviewFullscreen && GLOBAL_COLLECTION_VIEWS.has($currentRoute.view)
  );

  let previousRoutePath = $state(null);
  $effect(() => {
    const routePath = $currentRoute.path;
    if (previousRoutePath !== null && previousRoutePath !== routePath) {
      closeAllSidebarSurfaces();
    }
    previousRoutePath = routePath;
  });

  $effect(() => {
    const route = $currentRoute;
    const context = resolveMainAppWorkspaceContext(
      route,
      $workspacesStore.personalWorkspace?.id
    );

    if (context.kind === 'workspace') {
      void hydrateCurrentWorkspaceFromSharedData(context.workspaceId);
    } else if (context.kind === 'personal-pending') {
      void workspacesStore.loadPersonalWorkspace();
    } else if (context.kind === 'global-collection') {
      if ($currentWorkspace) currentWorkspace.clear();
      workspaceDataStore.initializeGlobal();
      clearWorkspaceGradient();
    } else {
      currentWorkspace.clear();
      workspaceDataStore.reset();
      clearWorkspaceGradient();
    }
  });

  $effect(() => {
    const redirect = getMainAppWorkspaceRedirect($currentRoute, $currentWorkspace);
    if (redirect) navigate(redirect, { replace: true });
  });

  $effect(() => {
    if (terminalState.visible && !TerminalPanelComponent) void loadTerminalPanel();
  });
</script>

<div class="desktop-app-shell h-dvh min-h-0 overflow-hidden flex flex-col" style="background-color: var(--ds-surface);">
  <EmailVerificationBanner
    show={showEmailVerificationBanner}
    ondismiss={() => showEmailVerificationBanner = false}
  />

  {#if !$uiStore.reviewFullscreen}
    <MainSidebar
      onShowCommandPalette={() => activateSidebarSurface('search')}
      onShowCreateModal={showCreateDropdown}
      onShowChatPanel={() => activateSidebarSurface('chat')}
      onToggleTerminal={toggleTerminal}
      activeSurface={activeSidebarSurface}
      onSurfaceChange={handleSidebarSurfaceChange}
    />
  {/if}

  <Button class="sr-only" onclick={() => activateSidebarSurface('search')} hotkeyConfig={{ key: toHotkeyString('global', 'commandPalette') }}>Command Palette</Button>
  <Button class="sr-only" onclick={showCreateDropdown} hotkeyConfig={{ key: toHotkeyString('global', 'create') }}>Create</Button>
  {#if aiStore.chatAvailable}
    <Button class="sr-only" onclick={() => activateSidebarSurface('chat')} hotkeyConfig={{ key: toHotkeyString('global', 'aiChat') }}>AI Chat</Button>
  {/if}
  <Button class="sr-only" onclick={toggleTerminal} hotkeyConfig={{ key: 'Mod+`' }}>Toggle Terminal</Button>

  <div
    class="authenticated-content flex flex-col flex-1 min-h-0 overflow-hidden transition-[margin] duration-200 ease-[cubic-bezier(0.25,1,0.5,1)] motion-reduce:transition-none"
    class:has-mobile-context-nav={showWorkspaceNav}
    style={!$uiStore.reviewFullscreen ? `margin-left: ${$uiStore.navExpanded ? '200px' : '64px'}` : ''}
  >
    {#if showWorkspaceBreadcrumbs}
      <WorkspaceBreadcrumbs
        workspace={showWorkspaceNav ? $currentWorkspace : null}
        onOpen={closeAllSidebarSurfaces}
      />
    {/if}
    <div class="authenticated-body flex flex-1 min-h-0 overflow-hidden">
      {#if showWorkspaceNav}
        <Button
          class="mobile-workspace-nav-trigger"
          variant="default"
          size="small"
          icon={Menu}
          title={mobileWorkspaceNavOpen ? 'Close workspace navigation' : 'Open workspace navigation'}
          dataTestid="mobile-workspace-nav-trigger"
          onclick={() => mobileWorkspaceNavOpen = !mobileWorkspaceNavOpen}
        >
          Workspace
        </Button>
        {#if mobileWorkspaceNavOpen}
          <button
            type="button"
            class="mobile-workspace-nav-backdrop"
            aria-label="Close workspace navigation"
            data-testid="mobile-workspace-nav-backdrop"
            onclick={() => mobileWorkspaceNavOpen = false}
          ></button>
        {/if}
        <div
          class="workspace-context-nav h-full min-h-0"
          class:mobile-open={mobileWorkspaceNavOpen}
          out:slide={{ duration: 200, axis: 'x' }}
        >
          <WorkspaceNavigation
            workspaceId={$currentRoute.path?.startsWith('/personal')
              ? $workspacesStore.personalWorkspace?.id
              : $currentRoute.params.id}
          />
        </div>
      {:else if showCollectionNav}
        <div class="h-full min-h-0" out:slide={{ duration: 200, axis: 'x' }}>
          <CollectionNavigation collectionId={$currentRoute.params.id} />
        </div>
      {/if}

      <div class="flex-1 flex min-w-0 min-h-0 overflow-hidden main-split-container">
        <div
          class="flex flex-col min-w-0 min-h-0"
          style={terminalState.visible
            ? `width: ${terminalState.splitPercent}%; flex-shrink: 0;`
            : 'flex: 1;'}
        >
          <main data-testid="main-content-scroll" class="flex-1 min-h-0 overflow-y-auto overscroll-contain">
            <MainRouteContent view={effectiveView} route={$currentRoute} {lazyComponents} />
          </main>
        </div>

        {#if terminalState.visible}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="terminal-resize-handle w-1 cursor-col-resize hover:bg-blue-500/40 active:bg-blue-500/60 transition-colors flex-shrink-0"
            style="background-color: var(--ds-border);"
            onmousedown={handleTerminalResizeStart}
          ></div>
          <div
            class="flex flex-col min-w-0"
            style="width: {100 - terminalState.splitPercent}%; flex-shrink: 0;"
          >
            {#if TerminalPanelComponent}
              <TerminalPanelComponent />
            {:else if terminalLoading}
              <div class="flex items-center justify-center h-full" style="background-color: #1a1b26;">
                <Spinner />
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  </div>

  <footer
    class="authenticated-footer flex-shrink-0 transition-[margin] duration-200 ease-[cubic-bezier(0.25,1,0.5,1)] motion-reduce:transition-none"
    style={!$uiStore.reviewFullscreen
      ? `margin-left: ${$uiStore.navExpanded ? '200px' : '64px'}`
      : ''}
  >
    <Footer />
  </footer>
</div>

<MainAppOverlays
  {lazyComponents}
  bind:showCommandPalette
  bind:showCreateModal
  bind:showChatPanel
  {createModalInitialType}
  {createModalWorkspaceId}
  {createModalParentContext}
  {createModalSkipNavigate}
  onclosecreate={closeCreateModal}
  onclosecommand={closeCommandPalette}
  onclosechat={closeChatPanel}
/>

<style>
  :global(html) {
    --nav-bg-color: var(--ds-surface-raised);
    --nav-text-color: var(--ds-text);
  }

  :global(.mobile-workspace-nav-trigger),
  .mobile-workspace-nav-backdrop {
    display: none;
  }

  @media (max-width: 767px) {
    .authenticated-content {
      margin-left: 4rem !important;
      min-width: 0;
    }

    .authenticated-content.has-mobile-context-nav .authenticated-body {
      padding-top: 3.5rem;
    }

    .workspace-context-nav {
      position: fixed;
      z-index: 45;
      top: 3rem;
      bottom: 0;
      left: 4rem;
      transform: translateX(-100%);
      transition: transform var(--duration-normal, 200ms) var(--ease-smooth, ease);
    }

    .workspace-context-nav.mobile-open {
      transform: translateX(0);
    }

    :global(.mobile-workspace-nav-trigger) {
      display: inline-flex;
      position: fixed;
      z-index: 50;
      top: 3.75rem;
      left: 4.75rem;
    }

    .mobile-workspace-nav-backdrop {
      display: block;
      position: fixed;
      z-index: 30;
      inset: 3rem 0 0 4rem;
      border: 0;
      background: color-mix(in srgb, var(--ds-blanket, #091e42) 54%, transparent);
    }

    .authenticated-footer {
      margin-left: 4rem !important;
      transform: none !important;
    }
  }

  :global(.themed-nav) {
    background-color: var(--nav-bg-color);
    color: var(--nav-text-color);
  }

  :global(.themed-nav *) {
    color: inherit;
  }

  :global(.themed-nav a),
  :global(.themed-nav button) {
    color: var(--nav-text-color);
  }

  :global(.themed-nav .nav-button) {
    color: var(--nav-text-color);
    position: relative;
    transition:
      background-color var(--duration-normal, 200ms) var(--ease-smooth, ease),
      box-shadow var(--duration-normal, 200ms) var(--ease-smooth, ease);
  }

  :global(.themed-nav .nav-button:hover) {
    background-color: var(--ds-background-neutral-hovered);
  }

  :global(.themed-nav .nav-button.nav-button-accent) {
    color: var(--ds-text-link);
    background-color: color-mix(in srgb, var(--ds-interactive-subtle) 72%, transparent);
  }

  :global(.themed-nav .nav-button.nav-button-accent:hover),
  :global(.themed-nav .nav-button.nav-button-accent.nav-button-selected) {
    color: var(--ds-text-link-hovered);
    background-color: var(--ds-interactive-subtle);
  }

  :global(.themed-nav .nav-shortcut) {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    gap: 0.1875rem;
  }

  :global(.themed-nav .nav-shortcut-key) {
    display: inline-flex;
    min-width: 1.5rem;
    height: 1.5rem;
    padding-inline: 0.3125rem;
    align-items: center;
    justify-content: center;
    border: 1px solid var(--ds-border);
    border-radius: 0.25rem;
    color: var(--ds-text-subtle);
    background: color-mix(in srgb, var(--ds-surface) 65%, transparent);
    box-shadow: 0 1px 0 var(--ds-border-bold);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-weight: 600;
    font-size: 0.6875rem;
    line-height: 1rem;
    text-align: center;
  }

  :global(.themed-nav .nav-button:focus-visible) {
    outline: 2px solid var(--ds-border-focused);
    outline-offset: 2px;
  }

  :global(.themed-nav .bg-primary) {
    color: var(--ds-text-inverse) !important;
    background-color: var(--ds-interactive) !important;
    transition:
      background-color var(--duration-normal, 200ms) var(--ease-smooth, ease),
      transform var(--duration-fast, 100ms) var(--ease-spring, cubic-bezier(0.34, 1.56, 0.64, 1)),
      box-shadow var(--duration-normal, 200ms) var(--ease-smooth, ease);
  }

  :global(.themed-nav .bg-primary:hover) {
    background-color: var(--ds-interactive-hovered) !important;
    transform: scale(1.05);
    box-shadow: var(--ds-glow-primary);
  }

  :global(.themed-nav .bg-primary:active) {
    transform: scale(0.95);
  }

  @media (prefers-reduced-motion: reduce) {
    :global(.themed-nav .nav-button),
    :global(.themed-nav .bg-primary) {
      transition: none;
    }

    :global(.themed-nav .bg-primary:hover),
    :global(.themed-nav .bg-primary:active) {
      transform: none;
    }
  }

  .terminal-resize-handle {
    position: relative;
    z-index: 10;
  }

  .terminal-resize-handle::before {
    content: '';
    position: absolute;
    top: 0;
    bottom: 0;
    left: -3px;
    right: -3px;
  }
</style>
