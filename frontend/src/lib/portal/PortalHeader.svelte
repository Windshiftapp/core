<script>
  import { Menu, ArrowLeft, Palette, Edit3, Check, Sun, Moon, User, LogOut, List } from 'lucide-svelte';
  import { authStore } from '../stores';
  import { portalStore, gradients } from '../stores/portal.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let hoveredMenuItem = $state(null);

  // Check if user is internal (either via authStore or portalAuth detecting internal session)
  let isInternalUser = $derived(authStore.isAuthenticated || portalAuthStore.isInternal);

  // Combined check: either internal admin OR portal customer is authenticated
  let isAnyUserAuthenticated = $derived(authStore.isAuthenticated || portalAuthStore.isAuthenticated);

  async function handleLogout() {
    if (portalAuthStore.isAuthenticated && !portalAuthStore.isInternal) {
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

<!-- Hamburger Menu & Done Button - Top Left (hidden in requests view) -->
{#if !portalStore.showMyRequests}
<div class="fixed top-6 left-6 z-40 flex items-center gap-3">
  <div class="relative">
    <!-- Hamburger Button -->
    <button
      onclick={() => portalStore.showMainMenu = !portalStore.showMainMenu}
      class="glass-btn w-10 h-10 rounded flex items-center justify-center text-white transition-all shadow-lg"
      title={t('common.menu')}
    >
      <Menu class="w-5 h-5" />
    </button>

    <!-- Dropdown Menu -->
    {#if portalStore.showMainMenu}
      <!-- Click-outside overlay -->
      <div
        class="fixed inset-0 z-[-1]"
        onclick={() => portalStore.showMainMenu = false}
      ></div>

      <!-- Menu Items -->
      <div
        class="absolute top-14 left-0 min-w-[200px] rounded-lg shadow-2xl overflow-hidden border"
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

        {#if isInternalUser}
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

          <!-- Edit (only show when not editing) -->
          {#if !portalStore.isEditing}
            <button
              onclick={() => { portalStore.toggleEditing(); portalStore.showMainMenu = false; }}
              class="w-full px-4 py-3 flex items-center gap-3 transition-all text-left"
              style="color: var(--ds-text);"
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
            >
              <Edit3 class="w-5 h-5" />
              <span class="font-medium">{t('common.edit')}</span>
            </button>
          {/if}
        {/if}

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
      oninput={(e) => portalStore.editableTitle = e.target.value}
      class="text-white font-semibold text-lg bg-transparent focus:outline-none max-w-[200px] truncate"
      placeholder="Portal Title"
    />
  {:else}
    <span class="text-white font-semibold text-lg truncate max-w-[200px]">
      {portalStore.editableTitle}
    </span>
  {/if}

</div>
{/if}

<!-- Profile Menu - Top Right (hidden in requests view) -->
{#if !portalStore.showMyRequests}
<div class="fixed top-6 right-6 z-40 flex items-center gap-3">
  <!-- Done Button (visible when editing) -->
  {#if portalStore.isEditing}
    <button
      onclick={() => portalStore.toggleEditing()}
      class="glass-btn flex items-center gap-2 px-4 py-2 text-white rounded transition-all shadow-lg"
    >
      <Check class="w-5 h-5" />
      <span class="font-medium">Done</span>
    </button>
  {/if}

  <div class="relative">
    <!-- Profile Avatar Button -->
    <button
      onclick={() => portalStore.showProfileMenu = !portalStore.showProfileMenu}
      class="glass-btn w-10 h-10 rounded-full flex items-center justify-center text-white transition-all shadow-lg"
    >
      <User class="w-5 h-5" />
    </button>

    <!-- Profile Dropdown Menu -->
    {#if portalStore.showProfileMenu}
      <div
        class="absolute top-14 right-0 w-64 rounded shadow-2xl border overflow-hidden"
        style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
      >
        {#if portalAuthStore.isAuthenticated && portalAuthStore.isInternal && portalAuthStore.user}
          <!-- Internal User Info (detected via portal auth) -->
          <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
                <User class="w-5 h-5" style="color: var(--ds-text);" />
              </div>
              <div class="flex-1">
                <div class="font-medium text-sm" style="color: var(--ds-text);">
                  {portalAuthStore.user.name}
                </div>
                <div class="text-xs" style="color: var(--ds-text-subtle);">{portalAuthStore.user.email}</div>
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
              class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
              style="color: {hoveredMenuItem === 'logout' ? '#dc2626' : 'var(--ds-text)'}; background-color: {hoveredMenuItem === 'logout' ? (portalStore.isDarkMode ? 'rgba(220, 38, 38, 0.1)' : '#fee2e2') : 'transparent'};"
              onmouseenter={() => hoveredMenuItem = 'logout'}
              onmouseleave={() => hoveredMenuItem = null}
              onclick={handleLogout}
            >
              <LogOut class="w-4 h-4" />
              <span class="text-sm">{t('portal.signOut')}</span>
            </button>
          </div>
        {:else if portalAuthStore.isAuthenticated && portalAuthStore.customer}
          <!-- Portal Customer Info (Magic Link Auth) -->
          <div class="px-4 py-3 border-b" style="border-color: var(--ds-border);">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
                <User class="w-5 h-5" style="color: var(--ds-text);" />
              </div>
              <div class="flex-1">
                <div class="font-medium text-sm" style="color: var(--ds-text);">
                  {portalAuthStore.customer.name || t('portal.portalCustomer') || 'Portal Customer'}
                </div>
                <div class="text-xs" style="color: var(--ds-text-subtle);">{portalAuthStore.customer.email}</div>
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
              class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
              style="color: {hoveredMenuItem === 'logout' ? '#dc2626' : 'var(--ds-text)'}; background-color: {hoveredMenuItem === 'logout' ? (portalStore.isDarkMode ? 'rgba(220, 38, 38, 0.1)' : '#fee2e2') : 'transparent'};"
              onmouseenter={() => hoveredMenuItem = 'logout'}
              onmouseleave={() => hoveredMenuItem = null}
              onclick={handleLogout}
            >
              <LogOut class="w-4 h-4" />
              <span class="text-sm">{t('portal.signOut')}</span>
            </button>
          </div>
        {:else if authStore.isAuthenticated && authStore.currentUser}
          <!-- Internal Admin User Info -->
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
              class="w-full px-4 py-2 flex items-center gap-3 transition-colors text-left"
              style="color: {hoveredMenuItem === 'logout' ? '#dc2626' : 'var(--ds-text)'}; background-color: {hoveredMenuItem === 'logout' ? (portalStore.isDarkMode ? 'rgba(220, 38, 38, 0.1)' : '#fee2e2') : 'transparent'};"
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
              style="color: {portalStore.isDarkMode ? '#60a5fa' : '#2563eb'}; background-color: {hoveredMenuItem === 'signin' ? 'var(--ds-background-neutral)' : 'transparent'};"
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
{/if}
