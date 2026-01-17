<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { currentRoute } from '../router.js';
  import { authStore } from '../stores';
  import { AlertCircle, Menu, ArrowLeft, Palette, Edit3, Sun, Moon, User, LogOut, List } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Components
  import Spinner from '../components/Spinner.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import PortalHeader from '../portal/PortalHeader.svelte';
  import PortalHero from '../portal/PortalHero.svelte';
  import PortalFooter from '../portal/PortalFooter.svelte';
  import PortalMyRequests from '../portal/PortalMyRequests.svelte';
  import PortalSections from '../portal/PortalSections.svelte';
  import PortalCustomizePanel from '../portal/PortalCustomizePanel.svelte';

  // Modals
  import PortalLoginDialog from '../dialogs/PortalLoginDialog.svelte';
  import RequestTypeFieldsModal from '../dialogs/RequestTypeFieldsModal.svelte';
  import RequestFormModal from '../dialogs/RequestFormModal.svelte';
  import RequestTypeModal from '../dialogs/RequestTypeModal.svelte';

  // Store
  import { portalStore, gradients } from '../stores/portal.svelte.js';
  import { api } from '../api.js';

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

  onMount(async () => {
    // Initialize auth (non-blocking for portal)
    authStore.init().catch(err => {
      console.log('Auth initialization failed (expected for public portal):', err);
    });

    // Load portal data
    const slug = $currentRoute.params?.slug;
    await portalStore.loadPortal(slug);

    // Apply theme CSS variables
    applyThemeStyles();
  });

  onDestroy(() => {
    portalStore.reset();
  });

  // Apply theme CSS variables when isDarkMode changes
  $effect(() => {
    applyThemeStyles();
  });

  function applyThemeStyles() {
    if (typeof document === 'undefined') return;

    const root = document.documentElement;
    if (portalStore.isDarkMode) {
      root.style.setProperty('--ds-surface', '#0f172a');
      root.style.setProperty('--ds-surface-raised', '#1e293b');
      root.style.setProperty('--ds-text', '#f1f5f9');
      root.style.setProperty('--ds-text-subtle', '#94a3b8');
      root.style.setProperty('--ds-border', '#475569');
      root.style.setProperty('--ds-surface-card', '#475569');
      root.style.setProperty('--ds-background-neutral', '#64748b');
      root.style.setProperty('--ds-icon-inner', '#94a3b8');
    } else {
      root.style.setProperty('--ds-surface', '#ffffff');
      root.style.setProperty('--ds-surface-raised', '#f9fafb');
      root.style.setProperty('--ds-text', '#111827');
      root.style.setProperty('--ds-text-subtle', '#6b7280');
      root.style.setProperty('--ds-border', '#e5e7eb');
      root.style.setProperty('--ds-surface-card', '#ffffff');
      root.style.setProperty('--ds-background-neutral', '#f3f4f6');
      root.style.setProperty('--ds-icon-inner', '#d1d5db');
    }
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
    console.log('Fields saved successfully');
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
    if (!portalStore.isEditing && !(portalStore.showCustomizePanel && portalStore.activeSection === 'request-types')) {
      selectedRequestTypeForForm = requestType;
      showRequestFormModal = true;
    }
  }

  function handleRequestSubmitted() {
    console.log('Request submitted successfully');
  }

  function handleLoginSuccess() {
    portalStore.showLoginDialog = false;
    if (portalStore.showMyRequests) {
      portalStore.loadMyRequests();
    }
  }

  async function handleLogout() {
    await authStore.logout();
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
    <!-- Main Content Wrapper -->
    <div class="flex-1 flex flex-col">
      <!-- Header (hamburger menu + profile) -->
      <PortalHeader />

      <!-- Hero Section (shown in normal portal view) -->
      {#if !portalStore.showMyRequests}
        <PortalHero />
      {:else}
        <!-- Compact Header for Request Views -->
        <div class="hero-gradient {portalStore.isDarkMode ? 'dark-mode' : ''} border-b border-white/20" style="background: {gradients[portalStore.selectedGradient].value};">
          <div class="hero-content max-w-7xl mx-auto px-6 py-4">
            <div class="flex items-center justify-between">
              <!-- Left: Portal Name -->
              <div class="flex items-center gap-3">
                <h1 class="text-xl font-semibold text-white">
                  {portalStore.editableTitle || portalStore.portalData?.name || 'Portal'}
                </h1>
              </div>

              <!-- Right: Navigation Buttons -->
              <div class="flex items-center gap-2">
                <!-- Hamburger Menu Button -->
                <div class="relative">
                  <button
                    onclick={() => portalStore.showMainMenu = !portalStore.showMainMenu}
                    class="w-10 h-10 rounded flex items-center justify-center text-white bg-white/10 backdrop-blur-sm hover:bg-white/20 transition-all shadow-lg border border-white/20"
                    title={t('portal.menu')}
                  >
                    <Menu class="w-5 h-5" />
                  </button>

                  <!-- Dropdown Menu -->
                  {#if portalStore.showMainMenu}
                    <div
                      class="fixed inset-0 z-[-1]"
                      onclick={() => portalStore.showMainMenu = false}
                    ></div>

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

                      {#if authStore.isAuthenticated}
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
                    class="w-10 h-10 rounded flex items-center justify-center text-white bg-white/10 backdrop-blur-sm hover:bg-white/20 transition-all shadow-lg border border-white/20"
                    title={t('common.profile')}
                  >
                    <User class="w-5 h-5" />
                  </button>

                  {#if portalStore.showProfileMenu}
                    <div
                      class="absolute top-14 right-0 w-64 rounded shadow-2xl border overflow-hidden"
                      style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
                    >
                      {#if authStore.isAuthenticated && authStore.currentUser}
                        <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
                          <div class="flex items-center gap-3">
                            {#if authStore.currentUser.avatar_url}
                              <img src={authStore.currentUser.avatar_url} alt={authStore.currentUser.username} class="w-10 h-10 rounded-full" />
                            {:else}
                              <div class="w-10 h-10 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
                                <User class="w-5 h-5" style="color: var(--ds-text);" />
                              </div>
                            {/if}
                            <div class="flex-1">
                              <div class="font-medium text-sm" style="color: var(--ds-text);">
                                {authStore.currentUser.first_name} {authStore.currentUser.last_name}
                              </div>
                              <div class="text-xs" style="color: var(--ds-text-subtle);">{authStore.currentUser.email}</div>
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
                  class="flex items-center gap-2 px-4 py-2 rounded text-white bg-white/10 backdrop-blur-sm hover:bg-white/20 transition-all shadow-lg border border-white/20"
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

    <!-- Customization Panel -->
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
        on:saved={handleFieldsSaved}
        on:close={() => showFieldsModal = false}
      />
    {/if}

    <!-- Portal Login Dialog -->
    <PortalLoginDialog
      bind:isOpen={portalStore.showLoginDialog}
      gradientValue={gradients[portalStore.selectedGradient].value}
      isDarkMode={portalStore.isDarkMode}
      on:success={handleLoginSuccess}
    />

    <!-- Request Form Modal -->
    {#if showRequestFormModal && selectedRequestTypeForForm && portalStore.portalData}
      <RequestFormModal
        bind:isOpen={showRequestFormModal}
        requestType={selectedRequestTypeForForm}
        portalSlug={portalStore.portalData.slug}
        isDarkMode={portalStore.isDarkMode}
        on:submitted={handleRequestSubmitted}
        on:close={() => showRequestFormModal = false}
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
        on:saved={handleRequestTypeSaved}
        on:close={() => showRequestTypeModal = false}
      />
    {/if}

  {/if}
</div>

<style>
  /* Hero gradient background */
  .hero-gradient {
    width: 100%;
    position: relative;
  }

  .hero-gradient::before {
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

  .hero-gradient.dark-mode::after {
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
