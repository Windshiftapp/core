<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { currentRoute } from '../router.js';
  import { authStore } from '../stores';
  import { AlertCircle, Menu, ArrowLeft, Palette, Edit3, Sun, Moon, User, LogOut, List } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Components
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import Spinner from '../components/Spinner.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import PortalHeader from '../portal/PortalHeader.svelte';
  import PortalHero from '../portal/PortalHero.svelte';
  import PortalFooter from '../portal/PortalFooter.svelte';
  import PortalMyRequests from '../portal/PortalMyRequests.svelte';
  import PortalSections from '../portal/PortalSections.svelte';
  import PortalCustomizePanel from '../portal/PortalCustomizePanel.svelte';

  // Modals
  import PortalLoginModal from '../portal/PortalLoginModal.svelte';
  import PortalVerifyLink from '../portal/PortalVerifyLink.svelte';
  import RequestTypeFieldsModal from '../dialogs/RequestTypeFieldsModal.svelte';
  import RequestFormModal from '../dialogs/RequestFormModal.svelte';
  import RequestTypeModal from '../dialogs/RequestTypeModal.svelte';

  // Store
  import { portalStore, gradients } from '../stores/portal.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { api } from '../api.js';
  import { navigate } from '../router.js';

  // Modal states (kept local since they are component-specific)
  let showFieldsModal = $state(false);
  let selectedRequestType = $state(null);
  let showRequestFormModal = $state(false);
  let selectedRequestTypeForForm = $state(null);
  let showRequestTypeModal = $state(false);
  let requestTypeModalMode = $state('create');
  let selectedRequestTypeForModal = $state(null);
  let availableItemTypes = $state([]);

  // Compact header state for requests view
  let hoveredMenuItem = $state(null);

  // Compute background style - image takes priority over gradient (same as PortalHero)
  const backgroundStyle = $derived(() => {
    if (portalStore.backgroundImageUrl) {
      return `background: linear-gradient(rgba(0,0,0,0.4), rgba(0,0,0,0.4)), url(${portalStore.backgroundImageUrl}) center/cover no-repeat;`;
    }
    const gradientValue = gradients[portalStore.selectedGradient]?.value;
    if (gradientValue) {
      return `background: ${gradientValue};`;
    }
    return `background: ${gradients[1].value};`;
  });

  // Magic link token from query params
  let verifyToken = $derived($currentRoute.query?.token);

  // Parse view params from URL
  let viewParam = $derived($currentRoute.query?.view);
  let requestIdParam = $derived($currentRoute.query?.id);
  let requestTypeParam = $derived($currentRoute.query?.['request-type']);

  // Track auth check completion to prevent flash of unauthenticated content
  let authCheckComplete = $state(false);

  // Track if we've synced the URL state (to avoid re-syncing on every change)
  let urlStateSynced = $state(false);

  // Track previous auth state to detect login events
  let previousAuthState = $state(false);

  // Derived authentication state - use $ prefix for proper store subscriptions
  let isUserAuthenticated = $derived(
    $authStore.isAuthenticated || $portalAuthStore.isAuthenticated
  );

  onMount(async () => {
    // Initialize auth (non-blocking for portal)
    authStore.init().catch(() => {});

    // Load portal data
    const slug = $currentRoute.params?.slug;
    await portalStore.loadPortal(slug);

    // Check portal customer auth (magic link session) - await to prevent flash
    await portalAuthStore.checkAuth(slug);
    authCheckComplete = true;

    // Apply theme CSS variables
    applyThemeStyles();
  });

  onDestroy(() => {
    portalStore.reset();
    portalAuthStore.reset();
  });

  // Sync portal view state from URL after auth check completes
  $effect(() => {
    if (!authCheckComplete || urlStateSynced) return;

    // Only sync once on initial load
    urlStateSynced = true;

    if (viewParam === 'requests') {
      // Set showMyRequests directly (don't toggle, which would navigate again)
      portalStore.setShowMyRequests(true);

      // If a specific request ID is in URL, load and view it
      if (requestIdParam) {
        portalStore.loadAndViewRequest(requestIdParam);
      }
    } else {
      portalStore.setShowMyRequests(false);

      // Check for request-type param to auto-open form
      if (requestTypeParam) {
        const requestTypeId = parseInt(requestTypeParam, 10);
        if (!isNaN(requestTypeId)) {
          // Wait for request types to load, then open the form
          const checkAndOpenForm = () => {
            const rt = portalStore.requestTypes.find(t => t.id === requestTypeId);
            if (rt) {
              openRequestForm(rt);
              // Clear the query param from URL without reload
              const slug = $currentRoute.params?.slug;
              navigate(`/portal/${slug}`, { replace: true });
            }
          };
          // If request types already loaded, open immediately; otherwise wait
          if (portalStore.requestTypes.length > 0) {
            checkAndOpenForm();
          } else {
            // Poll briefly for request types to load
            const interval = setInterval(() => {
              if (portalStore.requestTypes.length > 0) {
                clearInterval(interval);
                checkAndOpenForm();
              }
            }, 100);
            // Clear interval after 5 seconds to prevent infinite polling
            setTimeout(() => clearInterval(interval), 5000);
          }
        }
      }
    }
  });

  // Watch for auth state changes to reload request types after login
  $effect(() => {
    const currentAuth = $authStore.isAuthenticated || $portalAuthStore.isAuthenticated;

    // Only reload when auth state changes from false to true (login)
    if (authCheckComplete && currentAuth && !previousAuthState && portalStore.currentSlug) {
      portalStore.loadRequestTypes();
      // Also reload My Requests if that view is currently showing
      if (portalStore.showMyRequests) {
        portalStore.loadMyRequests();
      }
    }

    previousAuthState = currentAuth;
  });

  // Apply theme CSS variables when isDarkMode changes
  $effect(() => {
    applyThemeStyles();
  });

  function applyThemeStyles() {
    if (typeof document === 'undefined') return;
    document.documentElement.dataset.colorMode = portalStore.isDarkMode ? 'dark' : 'light';
  }

  // Handle ESC key to close customize panel
  function handleKeydown(event) {
    if (event.key === 'Escape') {
      if (portalStore.showCustomizePanel) {
        portalStore.showCustomizePanel = false;
      }
    }
  }

  // Load item types when opening request type modal
  async function loadItemTypes() {
    try {
      availableItemTypes = await api.itemTypes.getAll();
    } catch (err) {
      console.error('Failed to load item types:', err);
    }
  }

  // Modal handlers
  function openFieldsModal(requestType) {
    selectedRequestType = requestType;
    showFieldsModal = true;
  }

  function handleFieldsSaved() {
    // Reload request types to update field counts
    portalStore.loadRequestTypes();
  }

  async function openRequestTypeModal(mode, requestType = null) {
    requestTypeModalMode = mode;
    selectedRequestTypeForModal = requestType;
    await loadItemTypes();
    await tick();
    showRequestTypeModal = true;
  }

  async function handleRequestTypeSaved() {
    showRequestTypeModal = false;
    selectedRequestTypeForModal = null;
    await tick();
    portalStore.loadRequestTypes();
  }

  function openRequestForm(requestType) {
    if (portalStore.isEditing || (portalStore.showCustomizePanel && portalStore.activeSection === 'request-types')) {
      return;
    }

    // Check if authenticated (either internal or portal customer)
    const isAuthenticated = $authStore.isAuthenticated || $portalAuthStore.isAuthenticated;

    if (!isAuthenticated) {
      // Store request type to open after login
      portalStore.pendingRequestType = requestType;
      portalStore.showLoginDialog = true;
      return;
    }

    selectedRequestTypeForForm = requestType;
    showRequestFormModal = true;
  }

  function handleRequestSubmitted() {
  }

  function handleLoginSuccess() {
    portalStore.showLoginDialog = false;

    // Check if there's a pending request type to open
    if (portalStore.pendingRequestType) {
      selectedRequestTypeForForm = portalStore.pendingRequestType;
      portalStore.pendingRequestType = null;
      showRequestFormModal = true;
    } else if (portalStore.showMyRequests) {
      portalStore.loadMyRequests();
    }
  }

  async function handleVerifySuccess(customer) {
    // Clear token from URL
    const slug = $currentRoute.params?.slug;
    navigate(`/portal/${slug}`);

    // Force re-check auth to ensure UI reflects authenticated state
    await portalAuthStore.checkAuth(slug);

    // Refresh request types after auth change to ensure they're visible
    await portalStore.loadRequestTypes();

    // Reload my requests if that view is showing
    if (portalStore.showMyRequests) {
      portalStore.loadMyRequests();
    }
  }

  function handleVerifyError(message) {
    // Clear token from URL - let user try again
    const slug = $currentRoute.params?.slug;
    navigate(`/portal/${slug}`);
  }

  async function handleLogout() {
    await authStore.logout();
    portalStore.showProfileMenu = false;
  }

  async function handlePortalLogout() {
    const slug = $currentRoute.params?.slug;
    await portalAuthStore.logout(slug);
    portalStore.showProfileMenu = false;
  }
</script>

<!-- Global keydown listener for ESC key -->
<svelte:window onkeydown={handleKeydown} />

<!-- Portal Page - Standalone, no Windshift navigation -->
<div class="min-h-screen flex flex-col" style="background-color: var(--ds-surface, #ffffff);">
  {#if portalStore.loading}
    <!-- Loading State -->
    <div class="flex-1 flex items-center justify-center">
      <div class="text-center">
        <Spinner size="lg" class="mx-auto mb-4" />
        <p style="color: var(--ds-text-subtle);">{t('portal.loading')}</p>
      </div>
    </div>
  {:else if portalStore.error}
    <!-- Error State -->
    <div class="flex-1 flex items-center justify-center px-4">
      <EmptyState
        icon={AlertCircle}
        title={t('portal.notFound')}
        description={portalStore.error}
      />
    </div>
  {:else if portalStore.portalData}
    {#if !authCheckComplete}
      <!-- Auth Check Loading State -->
      <div class="flex-1 flex items-center justify-center">
        <div class="text-center">
          <Spinner size="lg" class="mx-auto mb-4" />
          <p style="color: var(--ds-text-subtle);">{t('portal.checkingAuth')}</p>
        </div>
      </div>
    {:else if isUserAuthenticated}
      <!-- AUTHENTICATED: Show full portal -->
      <div class="flex-1 flex flex-col">
        <!-- Header (hamburger menu + profile) -->
        <PortalHeader />

        <!-- Hero Section (shown in normal portal view) -->
        {#if !portalStore.showMyRequests}
          <PortalHero />
        {:else}
        <!-- Compact Header for Request Views -->
        <div class="hero-gradient {portalStore.isDarkMode ? 'dark-mode' : ''} {portalStore.backgroundImageUrl ? 'has-image' : ''} border-b border-white/20" style="{backgroundStyle()}">
          <div class="hero-content max-w-7xl mx-auto px-6 py-4">
            <div class="flex items-center justify-between">
              <!-- Left: Logo + Portal Name (clickable to go back to portal home) -->
              <button
                onclick={() => portalStore.toggleMyRequests()}
                class="flex items-center gap-3 hover:opacity-80 transition-opacity"
                title="Back to portal home"
              >
                {#if portalStore.effectiveLogoUrl}
                  <img
                    src={portalStore.effectiveLogoUrl}
                    alt="Portal logo"
                    class="h-10 max-w-[120px] object-contain"
                  />
                {/if}
                <h1 class="text-xl font-semibold text-white">
                  {portalStore.editableTitle || portalStore.portalData?.name || 'Portal'}
                </h1>
              </button>

              <!-- Right: Navigation Buttons -->
              <div class="flex items-center gap-2">
                <!-- Hamburger Menu Button -->
                <div class="relative">
                  <button
                    onclick={() => portalStore.showMainMenu = !portalStore.showMainMenu}
                    class="glass-btn w-10 h-10 rounded flex items-center justify-center text-white transition-all shadow-lg"
                    title={t('portal.menu')}
                  >
                    <Menu class="w-5 h-5" />
                  </button>

                  <!-- Dropdown Menu -->
                  {#if portalStore.showMainMenu}
                    <button
                      type="button"
                      class="fixed inset-0 z-[-1] cursor-default"
                      onclick={() => portalStore.showMainMenu = false}
                      aria-label="Close menu"
                    ></button>

                    <div
                      class="absolute top-12 right-0 min-w-[200px] rounded shadow-2xl border overflow-hidden"
                      style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
                    >
                      <button
                        onclick={() => { window.location.href = '/'; portalStore.showMainMenu = false; }}
                        class="w-full px-4 py-3 flex items-center gap-3 transition-colors hover:bg-black/5 text-left"
                        style="color: var(--ds-text);"
                      >
                        <ArrowLeft class="w-5 h-5" />
                        <span class="font-medium">{t('portal.backToApp')}</span>
                      </button>

                      {#if $authStore.isAuthenticated}
                        {#if !portalStore.isEditing}
                          <button
                            onclick={() => { portalStore.toggleEditing(); portalStore.showMainMenu = false; }}
                            class="w-full px-4 py-3 flex items-center gap-3 transition-colors hover:bg-black/5 text-left"
                            style="color: var(--ds-text);"
                          >
                            <Palette class="w-5 h-5" />
                            <span class="font-medium">{t('common.edit')}</span>
                          </button>
                        {/if}
                      {/if}

                      <button
                        onclick={() => { portalStore.toggleTheme(); portalStore.showMainMenu = false; }}
                        class="w-full px-4 py-3 flex items-center gap-3 transition-colors hover:bg-black/5 text-left"
                        style="color: var(--ds-text);"
                      >
                        {#if portalStore.isDarkMode}
                          <Sun class="w-5 h-5" />
                          <span class="font-medium">{t('portal.lightMode')}</span>
                        {:else}
                          <Moon class="w-5 h-5" />
                          <span class="font-medium">{t('portal.darkMode')}</span>
                        {/if}
                      </button>
                    </div>
                  {/if}
                </div>

                <!-- Profile Button -->
                <div class="relative">
                  <button
                    onclick={() => portalStore.showProfileMenu = !portalStore.showProfileMenu}
                    class="glass-btn w-10 h-10 rounded flex items-center justify-center text-white transition-all shadow-lg"
                    title={t('common.profile')}
                  >
                    <User class="w-5 h-5" />
                  </button>

                  {#if portalStore.showProfileMenu}
                    <div
                      class="absolute top-14 right-0 w-64 rounded shadow-2xl border overflow-hidden"
                      style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
                    >
                      {#if $portalAuthStore.isAuthenticated && $portalAuthStore.customer}
                        <!-- Portal Customer Info (Magic Link Auth) -->
                        <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
                          <div class="flex items-center gap-3">
                            <div class="w-10 h-10 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
                              <User class="w-5 h-5" style="color: var(--ds-text);" />
                            </div>
                            <div class="flex-1">
                              <div class="font-medium text-sm" style="color: var(--ds-text);">
                                {$portalAuthStore.customer.name || t('portal.portalCustomer') || 'Portal Customer'}
                              </div>
                              <div class="text-xs" style="color: var(--ds-text-subtle);">{$portalAuthStore.customer.email}</div>
                            </div>
                          </div>
                        </div>

                        <div class="py-1">
                          <button
                            class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                            style="color: var(--ds-text); background-color: {hoveredMenuItem === 'my-requests' ? 'var(--ds-background-neutral)' : 'transparent'};"
                            onmouseenter={() => hoveredMenuItem = 'my-requests'}
                            onmouseleave={() => hoveredMenuItem = null}
                            onclick={() => portalStore.toggleMyRequests()}
                          >
                            <List class="w-4 h-4" />
                            <span class="text-sm">{portalStore.showMyRequests ? t('portal.backToPortal') : t('portal.myRequests')}</span>
                          </button>
                          <button
                            class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                            style="color: {hoveredMenuItem === 'logout' ? '#dc2626' : 'var(--ds-text)'}; background-color: {hoveredMenuItem === 'logout' ? (portalStore.isDarkMode ? 'rgba(220, 38, 38, 0.1)' : '#fee2e2') : 'transparent'};"
                            onmouseenter={() => hoveredMenuItem = 'logout'}
                            onmouseleave={() => hoveredMenuItem = null}
                            onclick={handlePortalLogout}
                          >
                            <LogOut class="w-4 h-4" />
                            <span class="text-sm">{t('auth.signOut')}</span>
                          </button>
                        </div>
                      {:else if $authStore.isAuthenticated && $authStore.currentUser}
                        <!-- Internal Admin User Info -->
                        <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
                          <div class="flex items-center gap-3">
                            {#if $authStore.currentUser.avatar_url}
                              <img src={$authStore.currentUser.avatar_url} alt={$authStore.currentUser.username} class="w-10 h-10 rounded-full" />
                            {:else}
                              <div class="w-10 h-10 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
                                <User class="w-5 h-5" style="color: var(--ds-text);" />
                              </div>
                            {/if}
                            <div class="flex-1">
                              <div class="font-medium text-sm" style="color: var(--ds-text);">
                                {$authStore.currentUser.first_name} {$authStore.currentUser.last_name}
                              </div>
                              <div class="text-xs" style="color: var(--ds-text-subtle);">{$authStore.currentUser.email}</div>
                            </div>
                          </div>
                        </div>

                        <div class="py-1">
                          <button
                            class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                            style="color: var(--ds-text); background-color: {hoveredMenuItem === 'my-requests' ? 'var(--ds-background-neutral)' : 'transparent'};"
                            onmouseenter={() => hoveredMenuItem = 'my-requests'}
                            onmouseleave={() => hoveredMenuItem = null}
                            onclick={() => portalStore.toggleMyRequests()}
                          >
                            <List class="w-4 h-4" />
                            <span class="text-sm">{portalStore.showMyRequests ? t('portal.backToPortal') : t('portal.myRequests')}</span>
                          </button>
                          <button
                            class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                            style="color: {hoveredMenuItem === 'logout' ? '#dc2626' : 'var(--ds-text)'}; background-color: {hoveredMenuItem === 'logout' ? (portalStore.isDarkMode ? 'rgba(220, 38, 38, 0.1)' : '#fee2e2') : 'transparent'};"
                            onmouseenter={() => hoveredMenuItem = 'logout'}
                            onmouseleave={() => hoveredMenuItem = null}
                            onclick={handleLogout}
                          >
                            <LogOut class="w-4 h-4" />
                            <span class="text-sm">{t('auth.signOut')}</span>
                          </button>
                        </div>
                      {:else}
                        <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
                          <div class="flex items-center gap-3">
                            <div class="w-10 h-10 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
                              <User class="w-5 h-5" style="color: var(--ds-text);" />
                            </div>
                            <div class="flex-1">
                              <div class="font-medium text-sm" style="color: var(--ds-text);">{t('portal.guestUser')}</div>
                              <div class="text-xs" style="color: var(--ds-text-subtle);">{t('portal.notSignedIn')}</div>
                            </div>
                          </div>
                        </div>

                        <div class="py-1">
                          <button
                            class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                            style="color: {portalStore.isDarkMode ? '#60a5fa' : '#2563eb'}; background-color: {hoveredMenuItem === 'signin' ? 'var(--ds-background-neutral)' : 'transparent'};"
                            onmouseenter={() => hoveredMenuItem = 'signin'}
                            onmouseleave={() => hoveredMenuItem = null}
                            onclick={() => { portalStore.showLoginDialog = true; portalStore.showProfileMenu = false; }}
                          >
                            <User class="w-4 h-4" />
                            <span class="text-sm font-medium">{t('auth.signIn')}</span>
                          </button>
                        </div>
                      {/if}
                    </div>
                  {/if}
                </div>

                <!-- Back to Portal Button -->
                <button
                  onclick={() => portalStore.toggleMyRequests()}
                  class="glass-btn flex items-center gap-2 px-4 py-2 rounded text-white transition-all shadow-lg"
                >
                  <ArrowLeft class="w-4 h-4" />
                  <span class="font-medium">{t('portal.backToPortal')}</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      {/if}

        <!-- Content Area Below Hero -->
        <div class="flex-1" style="background-color: var(--ds-surface-raised);">
          <div class="max-w-4xl mx-auto px-6 py-16">
            {#if portalStore.showMyRequests}
              <PortalMyRequests />
            {:else}
              <PortalSections onOpenRequestForm={openRequestForm} />
            {/if}
          </div>
        </div>

        <!-- Footer -->
        <PortalFooter />
      </div>

      <!-- Customization Panel (only for authenticated internal users) -->
      <PortalCustomizePanel
        onOpenFieldsModal={openFieldsModal}
        onOpenRequestTypeModal={openRequestTypeModal}
      />

      <!-- Request Type Fields Modal -->
      {#if showFieldsModal && selectedRequestType}
        <RequestTypeFieldsModal
          bind:isOpen={showFieldsModal}
          requestTypeId={selectedRequestType.id}
          requestTypeName={selectedRequestType.name}
          isDarkMode={portalStore.isDarkMode}
          onsaved={handleFieldsSaved}
          onclose={() => showFieldsModal = false}
        />
      {/if}

      <!-- Request Form Modal -->
      {#if showRequestFormModal && selectedRequestTypeForForm && portalStore.portalData}
        <RequestFormModal
          bind:isOpen={showRequestFormModal}
          requestType={selectedRequestTypeForForm}
          portalSlug={portalStore.portalData.slug}
          isDarkMode={portalStore.isDarkMode}
          onsubmitted={handleRequestSubmitted}
          onclose={() => showRequestFormModal = false}
        />
      {/if}

      <!-- Request Type Modal (Create/Edit) -->
      {#if showRequestTypeModal && portalStore.portalData}
        <RequestTypeModal
          isOpen={showRequestTypeModal}
          mode={requestTypeModalMode}
          requestType={selectedRequestTypeForModal}
          channelId={portalStore.portalData.channel_id}
          {availableItemTypes}
          isDarkMode={portalStore.isDarkMode}
          onsaved={handleRequestTypeSaved}
          onclose={() => showRequestTypeModal = false}
        />
      {/if}
    {:else}
      <!-- NOT AUTHENTICATED: Show login prompt only -->
      <div class="flex-1 flex flex-col">
        <PortalHeader />
        <div class="hero-gradient {portalStore.isDarkMode ? 'dark-mode' : ''} {portalStore.backgroundImageUrl ? 'has-image' : ''}" style="{backgroundStyle()}">
          <div class="hero-content max-w-4xl mx-auto px-6 py-20 text-center">
            <h1 class="text-6xl font-bold mb-6 text-white">
              {portalStore.editableTitle}
            </h1>
            {#if portalStore.editableDescription}
              <p class="text-2xl text-white/90 mb-12 max-w-3xl mx-auto">
                {portalStore.editableDescription}
              </p>
            {/if}
            <!-- Login prompt -->
            <div class="max-w-md mx-auto">
              <p class="text-white/80 mb-6">{t('portal.signInToAccess')}</p>
              <button
                onclick={() => portalStore.showLoginDialog = true}
                class="glass-btn px-8 py-3 rounded-xl font-semibold text-lg text-white transition-all duration-200 shadow-xl hover:shadow-2xl hover:scale-[1.02] active:scale-[0.98]"
              >
                {t('auth.signIn')}
              </button>
            </div>
          </div>
        </div>
        <div class="flex-1" style="background-color: var(--ds-surface-raised);"></div>
        <PortalFooter />
      </div>
    {/if}

    <!-- Portal Login Modal (Magic Link) - always accessible -->
    <PortalLoginModal onloginsuccess={handleLoginSuccess} />

    <!-- Magic Link Verification - always accessible -->
    <ModalBackdrop show={!!verifyToken} blur={4} closeOnClick={false} closeOnEscape={false}>
      <div
        class="relative w-full max-w-md rounded-xl shadow-2xl overflow-hidden"
        style="background-color: var(--ds-surface-card);"
      >
        <PortalVerifyLink
          slug={$currentRoute.params?.slug}
          token={verifyToken}
          onSuccess={handleVerifySuccess}
          onError={handleVerifyError}
        />
      </div>
    </ModalBackdrop>
  {/if}
</div>

<style>
  /* Hero gradient background */
  .hero-gradient {
    width: 100%;
    position: relative;
  }

  /* Add subtle pattern overlay for depth (only for gradients, not images) */
  .hero-gradient:not(.has-image)::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-image:
      radial-gradient(circle at 20% 50%, rgba(255, 255, 255, 0.1) 0%, transparent 50%),
      radial-gradient(circle at 80% 80%, rgba(255, 255, 255, 0.1) 0%, transparent 50%);
    pointer-events: none;
  }

  /* Dark mode overlay - dims the gradient (not needed for images as they have built-in overlay) */
  .hero-gradient.dark-mode:not(.has-image)::after {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-color: rgba(0, 0, 0, 0.4);
    pointer-events: none;
    z-index: 0;
  }

  .hero-content {
    position: relative;
    z-index: 1;
  }
</style>
