<script>
  import { Settings, ArrowLeft, Palette, Sun, Moon, User, LogOut, List, ShieldCheck, KeyRound, FileText } from '@lucide/svelte';
  import { authStore } from '../stores';
  import { portalStore } from '../stores/portal.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { navigate } from '../router.js';
  import GlassButton from '../components/GlassButton.svelte';

  function goToProfile() {
    if (portalStore.currentSlug) {
      navigate(`/portal/${portalStore.currentSlug}/profile`);
    }
    portalStore.showProfileMenu = false;
  }

  let hoveredMenuItem = $state(null);

  // Check if user is internal (either via authStore or portalAuth detecting internal session)
  let isInternalUser = $derived($authStore.isAuthenticated || $portalAuthStore.isInternal);

  // Combined check: either internal admin OR portal customer is authenticated
  let isAnyUserAuthenticated = $derived($authStore.isAuthenticated || $portalAuthStore.isAuthenticated);

  async function handleLogout() {
    if ($portalAuthStore.isAuthenticated && !$portalAuthStore.isInternal) {
      // Portal customer logout
      await portalAuthStore.logout(portalStore.currentSlug);
    } else {
      // Internal admin logout
      await authStore.logout();
      // Also reset portal auth state since internal session is gone
      portalAuthStore.reset();
    }
    portalStore.showProfileMenu = false;
  }

  function handleLoginClick() {
    portalStore.showLoginDialog = true;
    portalStore.showProfileMenu = false;
  }
</script>

<!-- Header - constrained to max-w-7xl like content area (always visible) -->
<div class="fixed left-0 right-0 z-40 max-w-7xl mx-auto px-6 {portalStore.isEditing ? 'top-16' : 'top-6'}">
  <div class="flex items-center justify-between">
    <!-- Left side: logo + title -->
    <div class="flex items-center gap-3">
      <!-- Portal Logo (clickable to go back to portal home) -->
      {#if portalStore.effectiveLogoUrl}
        <button
          onclick={() => { if (portalStore.showMyRequests) portalStore.toggleMyRequests(); }}
          class="flex-shrink-0 cursor-pointer hover:opacity-80 transition-opacity"
          title="Back to portal home"
        >
          <img
            src={portalStore.effectiveLogoUrl}
            alt="Portal logo"
            class="h-10 max-w-[120px] object-contain"
          />
        </button>
      {/if}

      <!-- Portal Title -->
      {#if portalStore.isEditing}
        <input
          type="text"
          value={portalStore.editableTitle}
          oninput={(e) => portalStore.editableTitle = /** @type {HTMLInputElement} */ (e.target).value}
          class="text-white font-semibold text-xl bg-transparent focus:outline-none max-w-[200px] truncate"
          placeholder="Portal Title"
        />
      {:else}
        <button
          onclick={() => { if (portalStore.showMyRequests) portalStore.toggleMyRequests(); }}
          class="text-white font-semibold text-xl truncate max-w-[200px] bg-transparent hover:opacity-80 transition-opacity cursor-pointer"
          title="Back to portal home"
        >
          {portalStore.editableTitle}
        </button>
      {/if}
    </div>

    <!-- Right side: done + my requests + profile -->
    <div class="flex items-center gap-3">
      <!-- My Approvals Button (visible when authenticated and there are pending approvals) -->
      {#if isAnyUserAuthenticated && !portalStore.showMyApprovals && portalStore.pendingApprovalCount > 0}
        <GlassButton
          icon={ShieldCheck}
          onclick={() => portalStore.toggleMyApprovals()}
          title="My Approvals"
          class="relative"
        >
          <span class="font-medium text-sm">My Approvals</span>
          <span class="inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 text-[11px] font-bold leading-none text-white bg-red-500 rounded-full">
            {portalStore.pendingApprovalCount}
          </span>
        </GlassButton>
      {/if}

      <!-- My Requests Button (visible when authenticated and not in requests view) -->
      {#if isAnyUserAuthenticated && !portalStore.showMyRequests}
        <GlassButton
          icon={List}
          onclick={() => portalStore.toggleMyRequests()}
          title={t('portal.myRequests')}
          class="relative"
        >
          <span class="font-medium text-sm">{t('portal.myRequests')}</span>
          {#if portalStore.openRequestCount > 0}
            <span class="inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 text-[11px] font-bold leading-none text-white bg-red-500 rounded-full">
              {portalStore.openRequestCount}
            </span>
          {/if}
        </GlassButton>
      {/if}

      <!-- Admin Settings Icon (internal users only) -->
      {#if isInternalUser}
        <div class="relative">
          <GlassButton
            variant="round"
            icon={Settings}
            onclick={() => portalStore.showMainMenu = !portalStore.showMainMenu}
            title="Settings"
          />

          {#if portalStore.showMainMenu}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <div
              class="fixed inset-0 z-[-1]"
              role="presentation"
              onclick={() => portalStore.showMainMenu = false}
            ></div>

            <div
              class="absolute top-14 right-0 min-w-[200px] rounded-lg shadow-2xl overflow-hidden border"
              style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
            >
              <!-- Back to App -->
              <button
                onclick={() => { window.location.href = '/'; portalStore.showMainMenu = false; }}
                class="w-full px-4 py-3 flex items-center gap-3 transition-all text-left"
                style="color: var(--ds-text);"
                onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral)'}
                onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
              >
                <ArrowLeft class="w-5 h-5" />
                <span class="font-medium">{t('portal.backToApp')}</span>
              </button>

              <!-- Customize -->
              <button
                onclick={() => { portalStore.showCustomizePanel = true; portalStore.showMainMenu = false; }}
                class="w-full px-4 py-3 flex items-center gap-3 transition-all text-left"
                style="color: var(--ds-text);"
                onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral)'}
                onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
              >
                <Palette class="w-5 h-5" />
                <span class="font-medium">{t('portal.customizeButton')}</span>
              </button>

              <!-- Theme Toggle -->
              <button
                onclick={() => { portalStore.toggleTheme(); portalStore.showMainMenu = false; }}
                class="w-full px-4 py-3 flex items-center gap-3 transition-all text-left"
                style="color: var(--ds-text);"
                onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral)'}
                onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
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
      {/if}

      <div class="relative">
        <!-- Profile Avatar Button -->
        <GlassButton
          variant="round"
          icon={User}
          id="portal-avatar-button"
          onclick={() => portalStore.showProfileMenu = !portalStore.showProfileMenu}
        />

        <!-- Profile Dropdown Menu -->
        {#if portalStore.showProfileMenu}
          <div
            class="absolute top-14 right-0 w-64 rounded shadow-2xl border overflow-hidden"
            style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
          >
            {#if $portalAuthStore.isAuthenticated && $portalAuthStore.isInternal && $portalAuthStore.user}
              <!-- Internal User Info (detected via portal auth) -->
              <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
                    <User class="w-5 h-5" style="color: var(--ds-text);" />
                  </div>
                  <div class="flex-1">
                    <div class="font-medium text-sm" style="color: var(--ds-text);">
                      {$portalAuthStore.user.name}
                    </div>
                    <div class="text-xs" style="color: var(--ds-text-subtle);">{$portalAuthStore.user.email}</div>
                  </div>
                </div>
              </div>

              <!-- Internal User Menu Items -->
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
                  data-testid="portal-drafts-link"
                  class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                  style="color: var(--ds-text); background-color: {hoveredMenuItem === 'drafts' ? 'var(--ds-background-neutral)' : 'transparent'};"
                  onmouseenter={() => hoveredMenuItem = 'drafts'}
                  onmouseleave={() => hoveredMenuItem = null}
                  onclick={() => portalStore.toggleMyDrafts()}
                >
                  <FileText class="w-4 h-4" />
                  <span class="text-sm">{portalStore.showMyDrafts ? t('portal.backToPortal') : t('portal.myDrafts')}</span>
                </button>
                <button
                  class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                  style="color: {hoveredMenuItem === 'logout' ? 'var(--ds-text-danger)' : 'var(--ds-text)'}; background-color: {hoveredMenuItem === 'logout' ? 'var(--ds-danger-subtle)' : 'transparent'};"
                  onmouseenter={() => hoveredMenuItem = 'logout'}
                  onmouseleave={() => hoveredMenuItem = null}
                  onclick={handleLogout}
                >
                  <LogOut class="w-4 h-4" />
                  <span class="text-sm">{t('portal.signOut')}</span>
                </button>
              </div>
            {:else if $portalAuthStore.isAuthenticated && $portalAuthStore.customer}
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

              <!-- Portal Customer Menu Items -->
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
                  data-testid="portal-drafts-link"
                  class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                  style="color: var(--ds-text); background-color: {hoveredMenuItem === 'drafts' ? 'var(--ds-background-neutral)' : 'transparent'};"
                  onmouseenter={() => hoveredMenuItem = 'drafts'}
                  onmouseleave={() => hoveredMenuItem = null}
                  onclick={() => portalStore.toggleMyDrafts()}
                >
                  <FileText class="w-4 h-4" />
                  <span class="text-sm">{portalStore.showMyDrafts ? t('portal.backToPortal') : t('portal.myDrafts')}</span>
                </button>
                <button
                  data-testid="portal-profile-link"
                  class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                  style="color: var(--ds-text); background-color: {hoveredMenuItem === 'profile' ? 'var(--ds-background-neutral)' : 'transparent'};"
                  onmouseenter={() => hoveredMenuItem = 'profile'}
                  onmouseleave={() => hoveredMenuItem = null}
                  onclick={goToProfile}
                >
                  <KeyRound class="w-4 h-4" />
                  <span class="text-sm">{t('portal.profileAndSecurity') || 'Profile & security'}</span>
                </button>
                <button
                  data-testid="portal-logout"
                  class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                  style="color: {hoveredMenuItem === 'logout' ? 'var(--ds-text-danger)' : 'var(--ds-text)'}; background-color: {hoveredMenuItem === 'logout' ? 'var(--ds-danger-subtle)' : 'transparent'};"
                  onmouseenter={() => hoveredMenuItem = 'logout'}
                  onmouseleave={() => hoveredMenuItem = null}
                  onclick={handleLogout}
                >
                  <LogOut class="w-4 h-4" />
                  <span class="text-sm">{t('portal.signOut')}</span>
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

              <!-- Internal Admin Menu Items -->
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
                  data-testid="portal-drafts-link"
                  class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                  style="color: var(--ds-text); background-color: {hoveredMenuItem === 'drafts' ? 'var(--ds-background-neutral)' : 'transparent'};"
                  onmouseenter={() => hoveredMenuItem = 'drafts'}
                  onmouseleave={() => hoveredMenuItem = null}
                  onclick={() => portalStore.toggleMyDrafts()}
                >
                  <FileText class="w-4 h-4" />
                  <span class="text-sm">{portalStore.showMyDrafts ? t('portal.backToPortal') : t('portal.myDrafts')}</span>
                </button>
                <button
                  class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                  style="color: {hoveredMenuItem === 'logout' ? 'var(--ds-text-danger)' : 'var(--ds-text)'}; background-color: {hoveredMenuItem === 'logout' ? 'var(--ds-danger-subtle)' : 'transparent'};"
                  onmouseenter={() => hoveredMenuItem = 'logout'}
                  onmouseleave={() => hoveredMenuItem = null}
                  onclick={handleLogout}
                >
                  <LogOut class="w-4 h-4" />
                  <span class="text-sm">{t('portal.signOut')}</span>
                </button>
              </div>
            {:else}
              <!-- Guest User Info -->
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

              <!-- Guest Menu Items -->
              <div class="py-1">
                <button
                  class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
                  style="color: var(--ds-text-link); background-color: {hoveredMenuItem === 'signin' ? 'var(--ds-background-neutral)' : 'transparent'};"
                  onmouseenter={() => hoveredMenuItem = 'signin'}
                  onmouseleave={() => hoveredMenuItem = null}
                  onclick={handleLoginClick}
                >
                  <User class="w-4 h-4" />
                  <span class="text-sm font-medium">{t('portal.signIn')}</span>
                </button>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>
