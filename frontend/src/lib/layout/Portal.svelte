<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { currentRoute } from '../router.js';
  import { authStore } from '../stores';
  import { AlertCircle, Check } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Components
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import Spinner from '../components/Spinner.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Button from '../components/Button.svelte';
  import PortalHeader from '../portal/PortalHeader.svelte';
  import PortalHero from '../portal/PortalHero.svelte';
  import PortalFooter from '../portal/PortalFooter.svelte';
  import PortalMyRequests from '../portal/PortalMyRequests.svelte';
  import PortalMyApprovals from '../portal/PortalMyApprovals.svelte';
  import PortalMyDrafts from '../portal/PortalMyDrafts.svelte';
  import PortalSections from '../portal/PortalSections.svelte';
  import PortalCustomizePanel from '../portal/PortalCustomizePanel.svelte';

  // Modals
  import PortalLoginModal from '../portal/PortalLoginModal.svelte';
  import PortalVerifyLink from '../portal/PortalVerifyLink.svelte';
  import PortalProfile from '../portal/PortalProfile.svelte';
  import PortalPasskeyBanner from '../portal/PortalPasskeyBanner.svelte';
  import RequestTypeFieldsModal from '../dialogs/RequestTypeFieldsModal.svelte';
  import RequestFormModal from '../dialogs/RequestFormModal.svelte';
  import RequestTypeModal from '../dialogs/RequestTypeModal.svelte';
  import AssetReportModal from '../dialogs/AssetReportModal.svelte';
  import AssetReportFormModal from '../dialogs/AssetReportFormModal.svelte';

  /** @type {any} */
  const AlertCircleIcon = AlertCircle;

  // Store
  import {
    portalCatalogStore,
    portalCustomizationStore as portalStore,
  } from '../stores/portal.svelte.js';
  import {
    portalApprovalsStore,
    portalDraftsStore,
    portalRequestsStore,
  } from '../stores/portalActivity.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { api } from '../api.js';
  import { navigate } from '../router.js';

  // Modal states (kept local since they are component-specific)
  // Request-type fields editing has moved inline into PortalCustomizePanel —
  // no Portal-level state needed for that flow anymore.
  let showRequestFormModal = $state(false);
  let selectedRequestTypeForForm = $state(null);
  let showRequestTypeModal = $state(false);
  let requestTypeModalMode = $state('create');
  let selectedRequestTypeForModal = $state(null);
  let availableItemTypes = $state([]);
  let showAssetReportModal = $state(false);
  let assetReportModalMode = $state('create');
  let selectedAssetReportForModal = $state(null);
  let showAssetReportFieldsModal = $state(false);
  let selectedAssetReportForFields = $state(null);
  let showAssetReportForm = $state(false);
  let selectedAssetReportForForm = $state(null);

  // Magic link token: prefer URL fragment (#token=...), which never reaches the
  // server and isn't sent in Referer headers, falling back to the legacy
  // ?token=... query string for in-flight emails sent before the fragment
  // switch. Approval-requested emails additionally carry a `next=` segment in
  // the fragment that points at the specific approval. Capture and strip the
  // fragment on mount and on hash navigation so a fresh link opened in the
  // same tab after recovery is handled as another real login attempt.
  let hashToken = $state(/** @type {string|null} */ (null));
  let hashNext = $state(/** @type {string|null} */ (null));

  function captureMagicLinkHash() {
    if (typeof window === 'undefined') return;

    const tokenMatch = window.location.hash.match(/(?:^#|&)token=([^&]*)/);
    hashToken = tokenMatch?.[1] ? decodeURIComponent(tokenMatch[1]) : null;
    const nextMatch = window.location.hash.match(/(?:^#|&)next=([^&]*)/);
    hashNext = nextMatch?.[1] ? decodeURIComponent(nextMatch[1]) : null;
    // svelte-ignore state_referenced_locally
    if (hashToken || hashNext) {
      const cleaned = window.location.hash
        .replace(/(?:^#|&)(?:token|next)=[^&]*/g, (s) => (s.startsWith('#') ? '#' : ''))
        .replace(/^#$/, '');
      window.history.replaceState(
        null,
        '',
        window.location.pathname + window.location.search + cleaned
      );
    }
  }

  captureMagicLinkHash();
  let verifyToken = $derived(hashToken || $currentRoute.query?.token);

  // Validate a `next` URL before redirecting to it: must be a same-origin
  // path under /portal/ to prevent open-redirect via crafted email URLs.
  function safeNextPath(next) {
    if (!next || typeof next !== 'string') return null;
    if (!next.startsWith('/portal/')) return null;
    if (next.startsWith('//')) return null; // protocol-relative
    return next;
  }

  // Parse view params from URL
  let viewParam = $derived($currentRoute.query?.view);
  let requestIdParam = $derived($currentRoute.query?.id);
  let approvalIdParam = $derived($currentRoute.query?.id);
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

  // Whether the URL targets the portal customer Profile/Security page.
  // The /profile route shares the 'portal' view; we discriminate on path.
  let isProfileRoute = $derived(
    typeof $currentRoute?.path === 'string' && $currentRoute.path.endsWith('/profile')
  );

  onMount(async () => {
    window.addEventListener('hashchange', captureMagicLinkHash);

    const slug = $currentRoute.params?.slug;
    const [, userBootstrap] = await Promise.all([
      portalStore.loadPortal(slug),
      portalAuthStore.checkAuth(slug),
    ]);
    portalStore.hydrateUserBootstrap(userBootstrap);
    previousAuthState = userBootstrap?.authenticated === true;
    authCheckComplete = true;

    // Apply theme CSS variables
    applyThemeStyles();
  });

  onDestroy(() => {
    window.removeEventListener('hashchange', captureMagicLinkHash);
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
      portalRequestsStore.setVisible(true);

      // If a specific request ID is in URL, load and view it
      if (requestIdParam) {
        portalRequestsStore.loadAndView(requestIdParam);
      }
    } else if (viewParam === 'approvals') {
      portalApprovalsStore.setVisible(true);

      // Deep-link to a specific approval (magic-link `&next=` lands here).
      if (approvalIdParam) {
        portalApprovalsStore.loadAndView(approvalIdParam);
      }
    } else {
      portalRequestsStore.setVisible(false);
      portalApprovalsStore.setVisible(false);

      // Check for request-type param to auto-open form
      if (requestTypeParam) {
        const requestTypeId = parseInt(requestTypeParam, 10);
        if (!isNaN(requestTypeId)) {
          // Wait for request types to load, then open the form
          const checkAndOpenForm = () => {
            const rt = portalCatalogStore.requestTypes.find(t => t.id === requestTypeId);
            if (rt) {
              openRequestForm(rt);
              // Clear the query param from URL without reload
              const slug = $currentRoute.params?.slug;
              window.history.replaceState({}, '', `/portal/${slug}`);
            }
          };
          // If request types already loaded, open immediately; otherwise wait
          if (portalCatalogStore.requestTypes.length > 0) {
            checkAndOpenForm();
          } else {
            // Poll briefly for request types to load
            const interval = setInterval(() => {
              if (portalCatalogStore.requestTypes.length > 0) {
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

  // Replace the anonymous sign-in shell with the protected portal after login,
  // and clear protected portal state again after logout or access revocation.
  $effect(() => {
    const currentAuth = $authStore.isAuthenticated || $portalAuthStore.isAuthenticated;

    if (authCheckComplete && currentAuth !== previousAuthState && portalStore.currentSlug) {
      if (currentAuth) {
        if (portalAuthStore.userBootstrap?.authenticated) {
          portalStore.hydrateUserBootstrap(portalAuthStore.userBootstrap);
        }
      } else {
        portalStore.hydrateUserBootstrap(null);
      }
      void portalStore.loadPortal(portalStore.currentSlug);
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
    portalCatalogStore.loadRequestTypes();
  }

  async function openAssetReportModal(mode, assetReport = null) {
    assetReportModalMode = mode;
    selectedAssetReportForModal = assetReport;
    await tick();
    showAssetReportModal = true;
  }

  async function handleAssetReportSaved() {
    showAssetReportModal = false;
    selectedAssetReportForModal = null;
    await tick();
    portalCatalogStore.loadAssetReports();
  }

  function openAssetReportFieldsModal(report) {
    selectedAssetReportForFields = report;
    showAssetReportFieldsModal = true;
  }

  function handleAssetReportFieldsSaved() {
    portalCatalogStore.loadAssetReports();
  }

  function openAssetReportForm(report) {
    if (portalStore.isEditing || (portalStore.showCustomizePanel && portalStore.activeSection === 'asset-reports')) {
      return;
    }
    selectedAssetReportForForm = report;
    showAssetReportForm = true;
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

  function handleRequestSubmitted(itemId) {
    if (itemId) {
      portalRequestsStore.setVisible(true);
      portalRequestsStore.loadAndView(itemId);
      navigate(`/portal/${portalStore.currentSlug}?view=requests&id=${itemId}`);
    }
  }

  // Resume from the Drafts view: switch back to portal home, then open the
  // request form modal for the requested type. The modal's load effect picks
  // up the existing draft via api.portal.drafts.getForRequestType and jumps
  // to the saved step automatically — no extra plumbing required here.
  function handleResumeDraft({ requestType }) {
    if (!requestType) return;
    portalDraftsStore.visible = false;
    selectedRequestTypeForForm = requestType;
    showRequestFormModal = true;
    navigate(`/portal/${portalStore.currentSlug}`);
  }

  function handleLoginSuccess() {
    portalStore.showLoginDialog = false;

    // Check if there's a pending request type to open
    if (portalStore.pendingRequestType) {
      selectedRequestTypeForForm = portalStore.pendingRequestType;
      portalStore.pendingRequestType = null;
      showRequestFormModal = true;
    } else if (portalRequestsStore.visible) {
      portalRequestsStore.load();
    }
  }

  async function handleVerifySuccess(customer) {
    // Clear token from URL using replaceState so the prior URL containing the
    // token cannot be restored via the Back button or browser history sync.
    // hashToken is the captured-and-stripped fragment value; null it so the
    // verify modal (show={!!verifyToken}) closes.
    hashToken = null;
    const slug = $currentRoute.params?.slug;

    // Resolve the post-sign-in destination, in priority order:
    //   1. `&next=` from this verify URL's fragment (approval-link deep-link)
    //   2. sessionStorage `portal_pending_next` (recovery from a prior expired
    //      link — set by handleVerifyError when the customer re-auths)
    //   3. portal home
    let next = safeNextPath(hashNext);
    if (!next && typeof window !== 'undefined') {
      const stashed = window.sessionStorage.getItem('portal_pending_next');
      if (stashed) {
        next = safeNextPath(stashed);
        window.sessionStorage.removeItem('portal_pending_next');
      }
    }
    hashNext = null;
    if (typeof window !== 'undefined') {
      window.sessionStorage.removeItem('portal_pending_email');
    }

    navigate(next || `/portal/${slug}`, { replace: true });

    // Re-check auth. The auth-state effect reuses its badge snapshot and
    // replaces the anonymous shell with the protected portal bootstrap.
    const userBootstrap = await portalAuthStore.checkAuth(slug);
    portalStore.hydrateUserBootstrap(userBootstrap);
  }

  function handleVerifyError(message, code, hintEmail) {
    // For expired or already-used tokens, recover smoothly: stash the intended
    // post-sign-in destination + the customer's email, dismiss the verify
    // modal, and open the sign-in modal so they can request a fresh link
    // that — once consumed — lands them on the original target.
    if (code === 'expired' || code === 'used') {
      const next = safeNextPath(hashNext);
      if (typeof window !== 'undefined') {
        if (next) window.sessionStorage.setItem('portal_pending_next', next);
        if (hintEmail) window.sessionStorage.setItem('portal_pending_email', hintEmail);
      }
      hashToken = null;
      hashNext = null;
      portalStore.showLoginDialog = true;
      return;
    }
    // Invalid / unknown errors: leave the user on the verify modal so they
    // can read the error from PortalVerifyLink and use its "Back to Portal"
    // link to navigate away.
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
<div
  class="min-h-screen flex flex-col"
  style="background-color: var(--ds-surface, #ffffff);"
  data-testid="portal-page"
  data-ready={!!portalStore.portalData && authCheckComplete && !portalStore.loading}
>
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
        icon={AlertCircleIcon}
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
        <!-- Edit Mode Top Bar -->
        {#if portalStore.isEditing}
          <div class="relative z-[60] h-10" style="background-color: #fefce8; border-bottom: 1px solid #fde68a;">
            <div class="max-w-6xl mx-auto px-4 sm:px-6 flex items-center justify-between h-full">
              <span class="text-sm font-semibold" style="color: #92400e;">Editing portal</span>
              <Button variant="primary" size="small" icon={Check} onclick={() => portalStore.toggleEditing()}>
                Done
              </Button>
            </div>
          </div>
        {/if}

        <!-- Stable portal navigation -->
        <PortalHeader />

        <!-- Branded search hero on the portal home. -->
        {#if !portalRequestsStore.visible && !portalApprovalsStore.visible && !portalDraftsStore.visible && !isProfileRoute}
          <PortalHero />
        {/if}

        <!-- Portal work area -->
        <main class="flex-1" style="background-color: var(--ds-surface);">
          <div class="max-w-6xl mx-auto px-4 sm:px-6 py-8 sm:py-12">
            {#if isProfileRoute}
              <PortalProfile />
            {:else if portalApprovalsStore.visible}
              <PortalMyApprovals />
            {:else if portalDraftsStore.visible}
              <PortalMyDrafts onresume={handleResumeDraft} />
            {:else if portalRequestsStore.visible}
              <PortalMyRequests />
            {:else}
              <PortalPasskeyBanner />
              <PortalSections
                onOpenRequestForm={openRequestForm}
                onOpenAssetReportForm={openAssetReportForm}
              />
            {/if}
          </div>
        </main>

        <!-- Footer -->
        <PortalFooter />
      </div>

      <!-- Customization Panel (only for authenticated internal users).
           Request-type fields editing is now inline inside the panel itself
           (RequestTypeFieldsBuilder); only asset reports still use the modal. -->
      <PortalCustomizePanel
        onOpenRequestTypeModal={openRequestTypeModal}
        onOpenAssetReportModal={openAssetReportModal}
        onOpenAssetReportFieldsModal={openAssetReportFieldsModal}
      />

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
          channelWorkspaceIds={portalStore.portalData.workspace_ids || []}
          {availableItemTypes}
          isDarkMode={portalStore.isDarkMode}
          onsaved={handleRequestTypeSaved}
          onclose={() => showRequestTypeModal = false}
        />
      {/if}

      <!-- Asset Report Modal (Create/Edit) -->
      {#if showAssetReportModal && portalStore.portalData}
        <AssetReportModal
          isOpen={showAssetReportModal}
          mode={assetReportModalMode}
          assetReport={selectedAssetReportForModal}
          channelId={portalStore.portalData.channel_id}
          channelWorkspaceIds={portalStore.portalData.workspace_ids || []}
          isDarkMode={portalStore.isDarkMode}
          onsaved={handleAssetReportSaved}
          onclose={() => showAssetReportModal = false}
        />
      {/if}

      <!-- Asset Report Form Modal (portal launch) -->
      {#if showAssetReportForm && selectedAssetReportForForm && portalStore.portalData}
        <AssetReportFormModal
          bind:isOpen={showAssetReportForm}
          report={selectedAssetReportForForm}
          portalSlug={portalStore.portalData.slug}
          isDarkMode={portalStore.isDarkMode}
          onclose={() => showAssetReportForm = false}
        />
      {/if}

      <!-- Asset Report Fields Modal (form-mode only) -->
      {#if showAssetReportFieldsModal && selectedAssetReportForFields}
        <RequestTypeFieldsModal
          bind:isOpen={showAssetReportFieldsModal}
          resourceId={selectedAssetReportForFields.id}
          resourceName={selectedAssetReportForFields.name}
          apiHandlers={{
            getFields: (id) => api.assetReports.getFields(id),
            getAvailableFields: (id) => api.assetReports.getAvailableFields(id),
            updateFields: (id, fields) => api.assetReports.updateFields(portalStore.portalData?.channel_id, id, fields)
          }}
          isDarkMode={portalStore.isDarkMode}
          onsaved={handleAssetReportFieldsSaved}
          onclose={() => showAssetReportFieldsModal = false}
        />
      {/if}
    {:else}
      <!-- NOT AUTHENTICATED: a focused portal entry, not a marketing hero. -->
      <div class="flex-1 flex flex-col">
        <PortalHeader />
        <main class="flex-1 flex items-center" style="background-color: var(--ds-surface);">
          <div class="w-full max-w-6xl mx-auto px-4 sm:px-6 py-16 sm:py-24">
            <div class="grid lg:grid-cols-[minmax(0,1fr)_24rem] gap-10 lg:gap-20 items-center">
              <div
                class="max-w-2xl {(portalStore.hasBackgroundImage || portalStore.hasGradient)
                  ? 'rounded-xl p-7 sm:p-10'
                  : ''}"
                style={(portalStore.hasBackgroundImage || portalStore.hasGradient)
                  ? portalStore.headerBackgroundStyle
                  : ''}
              >
                <div
                  class="text-xs font-semibold uppercase tracking-[0.16em] mb-4"
                  style="color: {(portalStore.hasBackgroundImage || portalStore.hasGradient)
                    ? 'rgba(255,255,255,0.78)'
                    : 'var(--ds-text-subtle)'};"
                >
                  Customer portal
                </div>
                <h1
                  class="text-4xl sm:text-5xl font-semibold tracking-tight mb-5"
                  style="color: {(portalStore.hasBackgroundImage || portalStore.hasGradient)
                    ? '#ffffff'
                    : 'var(--ds-text)'};"
                >
                  {portalStore.editableTitle}
                </h1>
                {#if portalStore.editableDescription}
                  <p
                    class="text-lg sm:text-xl leading-relaxed max-w-xl"
                    style="color: {(portalStore.hasBackgroundImage || portalStore.hasGradient)
                      ? 'rgba(255,255,255,0.88)'
                      : 'var(--ds-text-subtle)'};"
                  >
                    {portalStore.editableDescription}
                  </p>
                {/if}
              </div>

              <div
                class="border rounded-lg p-6 sm:p-8"
                style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
              >
                <h2 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">
                  Access your requests
                </h2>
                <p class="text-sm leading-relaxed mb-6" style="color: var(--ds-text-subtle);">
                  {t('portal.signInToAccess')}
                </p>
                <button
                  id="portal-sign-in"
                  type="button"
                  onclick={() => (portalStore.showLoginDialog = true)}
                  class="w-full h-10 rounded-md px-4 text-sm font-semibold text-white transition-opacity hover:opacity-90"
                  style="background-color: var(--ds-interactive, #2563eb);"
                >
                  {t('auth.signIn')}
                </button>
                <p class="text-xs mt-4" style="color: var(--ds-text-subtle);">
                  No password required for customer access.
                </p>
              </div>
            </div>
          </div>
        </main>
        <PortalFooter />
      </div>
    {/if}

    <!-- Portal Login Modal (Magic Link) - always accessible -->
    <PortalLoginModal onloginsuccess={handleLoginSuccess} />

    <!-- Wait for the initial auth check before redeeming the link so its
         response cannot overwrite the verified session. Verification
         redirects programmatically, so close without an outro. Escape still
         lets the user leave a hung request; outside clicks stay disabled. -->
    <ModalBackdrop
      show={authCheckComplete && !!verifyToken}
      blur={4}
      transition={false}
      closeOnClick={false}
      closeOnEscape={true}
      onclose={() => {
        hashToken = null;
        const slug = $currentRoute.params?.slug;
        navigate(`/portal/${slug}`, { replace: true });
      }}
    >
      <div
        class="relative w-full max-w-md rounded-lg shadow-2xl overflow-hidden"
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
