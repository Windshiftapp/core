<script>
  import { onMount } from 'svelte';
  import { currentRoute, initRouter } from './lib/router.js';
  import { authStore } from './lib/stores';
  import { moduleSettings } from './lib/stores/moduleSettings.js';
  import { api } from './lib/api.js';
  import { APP_NAME } from './lib/constants.js';
  import { themeStore } from './lib/stores/theme.svelte.js';
  import { i18n, SUPPORTED_LOCALES } from './lib/stores/i18n.svelte.js';
  import LoginDialog from './lib/dialogs/LoginDialog.svelte';
  import WelcomeAssistant from './lib/pages/WelcomeAssistant.svelte';
  import Portal from './lib/layout/Portal.svelte';
  import MainApp from './lib/pages/MainApp.svelte';

  let showLoginDialog = $state(false);
  let setupCompleted = $state(false);
  let setupLoading = $state(true);
  let appInitialized = $state(false);
  let showWelcomeAssistant = $state(false);

  onMount(async () => {
    initRouter();

    // Initialize i18n (loads user's preferred locale)
    await i18n.init();

    // Check setup status first
    await checkSetupStatus();
    setupLoading = false;

    // Only initialize auth if setup is completed
    if (setupCompleted) {
      await authStore.init();
    }

    // Load theme early (works with or without authentication)
    await loadAndApplyTheme();

    // Only load data if setup is not completed OR user is authenticated
    if (!setupCompleted) {
      // Load basic data for setup flow
      moduleSettings.load();
      appInitialized = true;
    } else if ($authStore.isAuthenticated) {
      // For authenticated users, let MainApp handle the data loading
      appInitialized = true;
    } else {
      // Setup completed but not authenticated - app is ready for login
      appInitialized = true;
    }
  });

  // Show login dialog when setup is completed but user is not authenticated and app is initialized
  // But NOT for portal routes (they are public)
  const shouldShowLoginDialog = $derived(
    setupCompleted &&
      !$authStore.isAuthenticated &&
      !$authStore.loading &&
      appInitialized &&
      $currentRoute.view !== 'portal'
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

  async function checkSetupStatus() {
    try {
      const status = await api.setup.getStatus();
      setupCompleted = status.setup_completed;
      if (!status.setup_completed) {
        showWelcomeAssistant = true;
      }
    } catch (error) {
      console.error('Failed to check setup status:', error);
      // Assume setup is completed if we can't check
      setupCompleted = true;
    }
  }

  async function loadAndApplyTheme() {
    // Initialize theme store (sets up system preference detection)
    themeStore.init();

    try {
      const activeTheme = await api.themes.getActive();
      // Store the active theme in the theme store
      themeStore.setActiveTheme(activeTheme);
      applyNavColors(activeTheme);
    } catch (error) {
      // 401 is expected when not logged in - don't spam console
      if (!error.message?.includes('AUTHENTICATION_REQUIRED')) {
        console.error('Failed to load active theme:', error);
      }
      // Apply default theme if loading fails
      const defaultTheme = {
        nav_background_color_light: '#ffffff',
        nav_text_color_light: '#374151',
        nav_background_color_dark: '#1f2937',
        nav_text_color_dark: '#f3f4f6'
      };
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

<div class="min-h-screen flex flex-col" style="background-color: var(--ds-surface);">
  <!-- Show loading screen during initial setup check -->
  {#if setupLoading}
    <div class="min-h-screen flex items-center justify-center w-full">
      <div class="text-center">
        <div class="w-16 h-16 mx-auto mb-4">
          <img src="/windshift-3.svg" alt={APP_NAME} class="w-16 h-16 animate-pulse" />
        </div>
        <p class="text-gray-600">Loading...</p>
      </div>
    </div>
  <!-- Portal route - public, no authentication required -->
  {:else if $currentRoute.view === 'portal'}
    <Portal />
  <!-- Empty background during setup - WelcomeAssistant modal will show on top -->
  {:else if !setupCompleted && appInitialized}
    <div class="flex-1"></div>
  <!-- Show main app when user is authenticated -->
  {:else if $authStore.isAuthenticated && appInitialized}
    <MainApp />
  {:else}
    <!-- Show loading or login screen while waiting for auth -->
    <div class="flex-1 flex items-center justify-center">
      {#if $authStore.loading}
        <div class="text-center">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto mb-4"></div>
          <p class="text-gray-600">Loading...</p>
        </div>
      {:else if showLoginDialog}
        <!-- Login dialog will show, but we can show a minimal background -->
        <div class="text-center">
          <img src="/windshift-3.svg" alt="Windshift" class="w-16 h-16 mx-auto mb-4 opacity-50" />
          <h1 class="text-2xl font-bold text-gray-400 mb-2">Windshift</h1>
          <p class="text-gray-500">Work Management</p>
        </div>
      {/if}
    </div>
  {/if}
</div>

<!-- Welcome Assistant -->
<WelcomeAssistant
  bind:isOpen={showWelcomeAssistant}
  onsetup-completed={() => moduleSettings.reload()}
/>

<!-- Login Dialog -->
<LoginDialog
  bind:isOpen={showLoginDialog}
  onsuccess={() => {
    showLoginDialog = false;
  }}
/>

<style>
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

  :global(.themed-nav .nav-button.nav-button-selected) {
    background-color: var(--ds-surface-pressed);
  }

  :global(.themed-nav .nav-button.nav-button-selected:hover) {
    background-color: var(--ds-surface-pressed);
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
