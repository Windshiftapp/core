<script>
  import { onMount } from 'svelte';
  import { currentRoute, initRouter, isMobileRoute, navigate } from './lib/router.js';
  import { authStore } from './lib/stores';
  import { moduleSettings } from './lib/stores/moduleSettings.js';
  import { api } from './lib/api.js';
  import { APP_NAME } from './lib/constants.js';
  import { themeStore } from './lib/stores/theme.svelte.js';
  import { i18n, SUPPORTED_LOCALES, t } from './lib/stores/i18n.svelte.js';
  import { safeLoginReturnPath } from './lib/utils/loginReturnPath.js';
  import BrandedLoader from './lib/components/BrandedLoader.svelte';
  import LazyRootDialog from './lib/components/LazyRootDialog.svelte';
  import LazyRootView from './lib/components/LazyRootView.svelte';

  // Root surfaces are intentionally loaded only after startup has resolved the
  // route/auth boundary. Keep these as direct import() calls so Vite can split
  // public, login/setup, mobile, print, and desktop code into separate chunks.
  const ROOT_VIEW_LOADERS = {
    login: () => import('./lib/dialogs/LoginDialog.svelte'),
    welcome: () => import('./lib/pages/WelcomeAssistant.svelte'),
    portal: () => import('./lib/layout/Portal.svelte'),
    publicForm: () => import('./lib/features/forms/PublicFormPage.svelte'),
    setPassword: () => import('./lib/pages/SetPassword.svelte'),
    desktop: () => import('./lib/pages/MainApp.svelte'),
    mobile: () => import('./lib/mobile/MobileShell.svelte'),
    publicBoard: () => import('./lib/pages/PublicBoard.svelte'),
    pagePrint: () => import('./lib/features/pages/PagePrintView.svelte'),
    timeReportPrint: () => import('./lib/features/time/TimeReportPrintView.svelte'),
    testRunSummaryPrint: () => import('./lib/features/testing/TestRunSummaryPrintView.svelte'),
  };

  let showLoginDialog = $state(false);
  let setupCompleted = $state(false);
  let setupLoading = $state(true);
  let appInitialized = $state(false);
  let showWelcomeAssistant = $state(false);
  let startupError = $state('');
  let startupSlow = $state(false);
  let startupAttempt = 0;
  let themeAudience = null;
  let themeLoadGeneration = 0;

  const BOOTSTRAP_TIMEOUT_MS = 10_000;
  const SLOW_START_MS = 4_000;

  async function withBootstrapDeadline(promise) {
    let timeout;
    const deadline = new Promise((_, reject) => {
      timeout = window.setTimeout(() => {
        reject(
          Object.assign(new Error('The server took too long to respond.'), {
            code: 'REQUEST_TIMEOUT',
          })
        );
      }, BOOTSTRAP_TIMEOUT_MS);
    });
    try {
      return await Promise.race([promise, deadline]);
    } finally {
      window.clearTimeout(timeout);
    }
  }

  onMount(() => {
    initRouter();
    themeStore.init();
    void initializeApp();
  });

  async function initializeApp() {
    const attempt = ++startupAttempt;
    setupLoading = true;
    appInitialized = false;
    startupError = '';
    startupSlow = false;
    showLoginDialog = false;

    const slowTimer = window.setTimeout(() => {
      if (attempt === startupAttempt) startupSlow = true;
    }, SLOW_START_MS);

    try {
      // Initialize i18n (loads user's preferred locale)
      await withBootstrapDeadline(i18n.init());

      // Check setup status first
      await checkSetupStatus();
      if (attempt !== startupAttempt) return;

      // Only initialize auth if setup is completed. A transport error is not a
      // logout: authStore returns it so the startup UI can offer a retry.
      if (setupCompleted) {
        const authResult = await authStore.init({ timeout: BOOTSTRAP_TIMEOUT_MS });
        if (authResult?.status === 'error') throw authResult.error;
      }
      if (attempt !== startupAttempt) return;

      setupLoading = false;

      // The shell is ready once setup and session state are known. MainApp and
      // MobileShell own their subsequent data loading.
      appInitialized = true;
      if (!setupCompleted) {
        moduleSettings.load();
      } else if ($authStore.isAuthenticated) {
        // Phone viewport landing on the desktop root → mobile surface. Runs here
        // for SSO returns, reload logins, and existing sessions too.
        maybeRedirectToMobile();
      }
    } catch (error) {
      if (attempt !== startupAttempt) return;
      console.error('Failed to initialize Windshift:', error);
      setupLoading = false;
      appInitialized = false;
      startupError =
        error?.code === 'REQUEST_TIMEOUT'
          ? t('errors.TIMEOUT')
          : t('errors.NETWORK_ERROR');
    } finally {
      window.clearTimeout(slowTimer);
    }
  }

  // Show login dialog when setup is completed but user is not authenticated and app is initialized
  // But NOT for portal routes (they are public)
  const shouldShowLoginDialog = $derived(
    setupCompleted &&
      !$authStore.isAuthenticated &&
      !$authStore.loading &&
      appInitialized &&
      $currentRoute.view !== 'portal' &&
      $currentRoute.view !== 'public-form' &&
      $currentRoute.view !== 'set-password' &&
      $currentRoute.view !== 'public-board'
  );

  $effect(() => {
    if (shouldShowLoginDialog) {
      showLoginDialog = true;
    }
  });

  // Handle authentication state changes
  $effect(() => {
    if ($authStore.isAuthenticated && setupCompleted) {
      showLoginDialog = false;
      appInitialized = true;
    }
  });

  // Theme loading follows the authenticated audience rather than only the
  // initial bootstrap. This covers interactive login/logout without delaying
  // shell rendering or requiring a page reload to pick up the active theme.
  $effect(() => {
    if (!appInitialized) return;

    const nextAudience = $authStore.isAuthenticated
      ? `user:${$authStore.currentUser?.id ?? 'authenticated'}`
      : 'public';
    if (nextAudience === themeAudience) return;

    themeAudience = nextAudience;
    const generation = ++themeLoadGeneration;
    void loadAndApplyTheme($authStore.isAuthenticated, generation);
  });

  // Sync i18n locale with user's saved language preference
  $effect(() => {
    if ($authStore.isAuthenticated && authStore.currentUser?.language) {
      const userLang = authStore.currentUser.language;
      if (SUPPORTED_LOCALES.some(l => l.code === userLang) && i18n.locale !== userLang) {
        i18n.setLocale(userLang);
      }
    }
  });

  // Update document direction when locale changes (for RTL support)
  $effect(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.dir = i18n.direction;
      document.documentElement.lang = i18n.locale;
    }
  });

  // Lock <html>/<body> to the visible viewport while the mobile PWA shell is
  // active, so the page can't scroll or rubber-band behind the notch / dynamic
  // browser chrome. Driven by route (the same signal that selects MobileShell)
  // rather than a viewport media query, so it also covers iPads/tablets whose
  // width exceeds the phone breakpoint but still render the mobile surface.
  // app.css scopes the lock to html.mobile-shell-active.
  $effect(() => {
    if (typeof document === 'undefined') return;
    document.documentElement.classList.toggle(
      'mobile-shell-active',
      isMobileRoute($currentRoute.view)
    );
  });

  // After an interactive login on a phone-sized viewport, send the user to the
  // mobile surface — unless they've opted into the desktop site, or they logged
  // in on a deep link (only redirect from the default landing page). Installed
  // PWAs already open at /m via the manifest start_url, so this only affects
  // plain browsers.
  function maybeRedirectToMobile() {
    try {
      if (localStorage.getItem('windshift-prefer-desktop') === 'true') return;
    } catch {
      // localStorage unavailable — fall through and use the viewport check.
    }
    const onLanding = $currentRoute.view === 'homepage' || $currentRoute.path === '/';
    if (onLanding && window.matchMedia?.('(max-width: 768px)').matches) {
      navigate('/m');
    }
  }

  async function checkSetupStatus() {
    // Always ask the backend. setup_completed is cheap to fetch and the
    // /setup/status rate-limit burst comfortably covers normal reloads. We
    // deliberately do NOT cache the result client-side: a cached "completed"
    // flag survives backend/DB swaps under the same origin (e.g. dev worktrees
    // behind the vite proxy), making a fresh, unconfigured instance look
    // already set up and silently skipping the setup wizard.
    const status = await api.setup.getStatus({ timeout: BOOTSTRAP_TIMEOUT_MS });
    setupCompleted = status.setup_completed;
    if (!status.setup_completed) {
      showWelcomeAssistant = true;
    }
  }

  async function loadAndApplyTheme(isAuthenticated, generation) {
    const defaultTheme = {
      nav_background_color_light: '#ffffff',
      nav_text_color_light: '#374151',
      nav_background_color_dark: '#1f2937',
      nav_text_color_dark: '#f3f4f6'
    };

    // Public pages have no authenticated theme to load. Applying defaults
    // locally avoids a guaranteed 401 request on portals, public forms, and
    // the login shell.
    if (!isAuthenticated) {
      if (generation !== themeLoadGeneration) return;
      themeStore.setActiveTheme(defaultTheme);
      applyNavColors(defaultTheme);
      return;
    }

    try {
      const activeTheme = await api.themes.getActive({ timeout: BOOTSTRAP_TIMEOUT_MS });
      if (generation !== themeLoadGeneration) return;
      // Store the active theme in the theme store
      themeStore.setActiveTheme(activeTheme);
      applyNavColors(activeTheme);
    } catch (error) {
      if (generation !== themeLoadGeneration) return;
      // 401 is expected when not logged in - don't spam console
      if (error.status !== 401) {
        console.error('Failed to load active theme:', error);
      }
      // Apply default theme if loading fails
      themeStore.setActiveTheme(defaultTheme);
      applyNavColors(defaultTheme);
    }
  }

  function applyNavColors(theme) {
    if (!theme) return;

    const root = document.documentElement;
    const isDark = themeStore.resolvedTheme === 'dark';

    root.style.setProperty(
      '--nav-bg-color',
      isDark ? theme.nav_background_color_dark : theme.nav_background_color_light
    );
    root.style.setProperty(
      '--nav-text-color',
      isDark ? theme.nav_text_color_dark : theme.nav_text_color_light
    );
  }
</script>

<a class="skip-link" href="#app-main">{t('common.skipToMainContent')}</a>
<div
  id="app-main"
  tabindex="-1"
  class="flex flex-col {isMobileRoute($currentRoute.view) ? 'h-dvh overflow-hidden' : 'min-h-screen'}"
  style="background-color: var(--ds-surface);"
>
  {#if startupError}
    <div class="min-h-screen flex items-center justify-center w-full px-6" data-testid="startup-error">
      <div class="text-center max-w-sm">
        <img src="windshift-3.svg" alt={APP_NAME} width="64" height="64" class="w-16 h-16 mx-auto mb-4 opacity-75" />
        <h1 class="text-xl font-semibold mb-2">{t('errors.failedToLoad')} Windshift</h1>
        <p class="text-gray-600 mb-1">{startupError}</p>
        <p class="text-sm text-gray-500 mb-5">{t('errors.NETWORK_ERROR')}</p>
        <button
          type="button"
          class="min-h-11 px-5 py-2 rounded-lg bg-blue-600 text-white font-semibold hover:bg-blue-700"
          onclick={() => initializeApp()}
          data-testid="startup-retry"
        >{t('common.retry')}</button>
      </div>
    </div>
  <!-- Show loading screen during initial setup/session checks -->
  {:else if setupLoading}
    <BrandedLoader
      label={t('common.loading')}
      detail={startupSlow ? t('common.loadingSlow') : ''}
    />
  <!-- Public board route - no authentication required -->
  {:else if $currentRoute.view === 'public-board'}
    <LazyRootView
      loader={ROOT_VIEW_LOADERS.publicBoard}
      label="public board"
      componentProps={{ slug: $currentRoute.params.slug }}
    />
  <!-- Portal route - public, no authentication required -->
  {:else if $currentRoute.view === 'portal'}
    <LazyRootView loader={ROOT_VIEW_LOADERS.portal} label="portal" />
  <!-- Public form route - no authentication required -->
  {:else if $currentRoute.view === 'public-form'}
    <LazyRootView loader={ROOT_VIEW_LOADERS.publicForm} label="form" />
  <!-- Set password route - public with token -->
  {:else if $currentRoute.view === 'set-password'}
    <LazyRootView loader={ROOT_VIEW_LOADERS.setPassword} label="password setup" />
  <!-- Empty background during setup - WelcomeAssistant modal will show on top -->
  {:else if !setupCompleted && appInitialized}
    <div class="flex-1"></div>
  <!-- Chrome-free print/PDF view for a single page (authenticated, no app shell) -->
  {:else if $authStore.isAuthenticated && appInitialized && $currentRoute.view === 'page-print'}
    <LazyRootView
      loader={ROOT_VIEW_LOADERS.pagePrint}
      label="print view"
      componentProps={{
        workspaceId: Number($currentRoute.params.id),
        pageId: Number($currentRoute.params.pageId),
      }}
    />
  {:else if $authStore.isAuthenticated && appInitialized && $currentRoute.view === 'time-report-print'}
    <LazyRootView loader={ROOT_VIEW_LOADERS.timeReportPrint} label="time report print view" />
  {:else if $authStore.isAuthenticated && appInitialized && $currentRoute.view === 'test-run-summary-print'}
    <LazyRootView
      loader={ROOT_VIEW_LOADERS.testRunSummaryPrint}
      label="test run summary print view"
      componentProps={{
        workspaceId: Number($currentRoute.params.id),
        runId: Number($currentRoute.params.runId),
      }}
    />
  <!-- Mobile PWA surface (phone-focused shell, bypasses desktop MainApp chrome) -->
  {:else if $authStore.isAuthenticated && appInitialized && isMobileRoute($currentRoute.view)}
    <LazyRootView loader={ROOT_VIEW_LOADERS.mobile} label="mobile workspace" />
  <!-- Show main app when user is authenticated -->
  {:else if $authStore.isAuthenticated && appInitialized}
    <LazyRootView loader={ROOT_VIEW_LOADERS.desktop} label="workspace" />
  {:else}
    <!-- Show loading or login screen while waiting for auth -->
    <div class="flex-1 flex items-center justify-center">
      {#if $authStore.loading}
        <BrandedLoader fullViewport={false} />
      {:else if showLoginDialog}
        <!-- Login dialog will show, but we can show a minimal background -->
        <div class="text-center">
          <img src="windshift-3.svg" alt="Windshift" width="64" height="64" class="w-16 h-16 mx-auto mb-4 opacity-50" />
          <h1 class="text-2xl font-bold text-gray-400 mb-2">Windshift</h1>
          <p class="text-gray-500">{t('footer.platformName')}</p>
        </div>
      {/if}
    </div>
  {/if}
</div>

<!-- Setup and login dialogs are split too; public routes never fetch them. -->
{#if showWelcomeAssistant}
  <LazyRootDialog
    loader={ROOT_VIEW_LOADERS.welcome}
    label="setup assistant"
    bind:isOpen={showWelcomeAssistant}
    componentProps={{ 'onsetup-completed': () => moduleSettings.reload() }}
  />
{/if}

{#if showLoginDialog}
  <LazyRootDialog
    loader={ROOT_VIEW_LOADERS.login}
    label="sign in"
    bind:isOpen={showLoginDialog}
    componentProps={{
      onsuccess: () => {
        showLoginDialog = false;
        const returnTo = safeLoginReturnPath(window.location.search);
        if (returnTo) {
          navigate(returnTo);
        } else {
          maybeRedirectToMobile();
        }
      },
    }}
  />
{/if}

<style>
  .skip-link {
    position: fixed;
    top: 0.75rem;
    left: 0.75rem;
    z-index: 1000;
    padding: 0.625rem 0.875rem;
    border-radius: 0.375rem;
    background: var(--ds-surface-raised);
    color: var(--ds-text);
    box-shadow: 0 4px 12px rgb(0 0 0 / 20%);
    transform: translateY(-200%);
    transition: transform 120ms ease;
  }

  .skip-link:focus-visible {
    transform: translateY(0);
  }

  /* Global CSS custom properties for theming - uses design tokens */
  :global(html) {
    --nav-bg-color: var(--ds-surface-raised);
    --nav-text-color: var(--ds-text);
  }

  /* Themed navigation styles */
  :global(.themed-nav) {
    background-color: var(--nav-bg-color);
    color: var(--nav-text-color);
  }

  /* Ensure child elements inherit the theme colors */
  :global(.themed-nav *) {
    color: inherit;
  }

  /* Override any specific text colors for navigation elements */
  :global(.themed-nav a),
  :global(.themed-nav button) {
    color: var(--nav-text-color);
  }

  /* Theme-aware navigation button classes */
  :global(.themed-nav .nav-button) {
    color: var(--nav-text-color);
    transition: all 0.2s ease;
  }

  :global(.themed-nav .nav-button:hover) {
    background-color: var(--ds-background-neutral-hovered);
  }

  :global(.themed-nav .nav-button.nav-button-emphasized) {
    background-color: color-mix(in srgb, var(--ds-interactive) 8%, transparent);
  }

  /* Exception: Primary buttons should keep their original colors and hover behavior */
  :global(.themed-nav .bg-primary) {
    color: var(--ds-text-inverse) !important;
    background-color: var(--ds-interactive) !important;
  }

  :global(.themed-nav .bg-primary:hover) {
    background-color: var(--ds-interactive-hovered) !important;
  }
</style>
