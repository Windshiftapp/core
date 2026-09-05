<script>
  import {
    ArrowLeft,
    CheckSquare,
    FileText,
    Home,
    KeyRound,
    List,
    LogOut,
    Moon,
    Palette,
    Settings,
    ShieldCheck,
    Sun,
    User,
  } from '@lucide/svelte';
  import { authStore } from '../stores';
  import { portalCustomizationStore as portalStore } from '../stores/portal.svelte.js';
  import {
    portalApprovalsStore,
    portalDraftsStore,
    portalRequestsStore,
  } from '../stores/portalActivity.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { navigate } from '../router.js';
  import Input from '../components/Input.svelte';

  let isInternalUser = $derived($authStore.isAuthenticated || $portalAuthStore.isInternal);
  let isAnyUserAuthenticated = $derived(
    $authStore.isAuthenticated || $portalAuthStore.isAuthenticated
  );
  let hasVisualBackground = $derived(portalStore.hasBackgroundImage || portalStore.hasGradient);
  let shellText = $derived(hasVisualBackground ? '#ffffff' : 'var(--ds-text)');
  let shellTextSubtle = $derived(
    hasVisualBackground ? 'rgba(255,255,255,0.82)' : 'var(--ds-text-subtle)'
  );
  let shellControl = $derived(
    hasVisualBackground ? 'rgba(255,255,255,0.12)' : 'var(--ds-background-neutral)'
  );

  let account = $derived.by(() => {
    const internalAvatar =
      $authStore.currentUser?.avatar_url || $portalAuthStore.user?.avatar_url || null;

    if ($portalAuthStore.isAuthenticated && $portalAuthStore.customer) {
      return {
        name:
          $portalAuthStore.customer.name ||
          t('portal.portalCustomer') ||
          'Portal customer',
        email: $portalAuthStore.customer.email,
        avatar: null,
        canManageProfile: true,
      };
    }
    if ($portalAuthStore.isAuthenticated && $portalAuthStore.user) {
      return {
        name: $portalAuthStore.user.name || 'Windshift user',
        email: $portalAuthStore.user.email,
        avatar: internalAvatar,
        canManageProfile: false,
      };
    }
    if ($authStore.isAuthenticated && $authStore.currentUser) {
      return {
        name:
          `${$authStore.currentUser.first_name || ''} ${$authStore.currentUser.last_name || ''}`.trim() ||
          $authStore.currentUser.username,
        email: $authStore.currentUser.email,
        avatar: $authStore.currentUser.avatar_url,
        canManageProfile: false,
      };
    }
    return null;
  });

  let accountInitials = $derived(account ? getInitials(account.name) : '');

  function closeMenus() {
    portalStore.showProfileMenu = false;
    portalStore.showMainMenu = false;
  }

  function goHome() {
    portalRequestsStore.setVisible(false);
    portalApprovalsStore.setVisible(false);
    portalDraftsStore.setVisible(false);
    closeMenus();
    if (portalStore.currentSlug) navigate(`/portal/${portalStore.currentSlug}`);
  }

  function goToProfile() {
    if (portalStore.currentSlug) navigate(`/portal/${portalStore.currentSlug}/profile`);
    closeMenus();
  }

  async function handleLogout() {
    if ($portalAuthStore.isAuthenticated && !$portalAuthStore.isInternal) {
      await portalAuthStore.logout(portalStore.currentSlug);
    } else {
      await authStore.logout();
      portalAuthStore.reset();
    }
    closeMenus();
  }

  function handleLoginClick() {
    portalStore.showLoginDialog = true;
    closeMenus();
  }

  /** @param {string} name */
  function getInitials(name) {
    if (!name) return '';
    const parts = name.trim().split(/\s+/).filter(Boolean);
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
    }
    return parts[0].substring(0, 2).toUpperCase();
  }

  function removeBrokenAvatar(event) {
    if (event.currentTarget instanceof HTMLImageElement) {
      event.currentTarget.remove();
    }
  }
</script>

<header
  class="relative z-40 border-b {hasVisualBackground ? 'portal-header-branded' : ''}"
  style="{hasVisualBackground
    ? portalStore.headerBackgroundStyle
    : 'background-color: color-mix(in srgb, var(--ds-surface-card) 94%, transparent);'} border-color: {hasVisualBackground
    ? 'rgba(255,255,255,0.18)'
    : 'var(--ds-border)'};"
>
  <div class="max-w-6xl mx-auto h-16 px-4 sm:px-6 flex items-center gap-3">
    <button
      type="button"
      onclick={goHome}
      class="min-w-0 flex items-center gap-3 text-left"
      title={t('portal.backToPortal') || 'Back to portal'}
    >
      {#if portalStore.effectiveLogoUrl}
        <img
          src={portalStore.effectiveLogoUrl}
          alt=""
          class="h-8 w-8 sm:w-auto sm:max-w-28 object-contain flex-none"
        />
      {/if}
      {#if portalStore.isEditing}
        <Input
          type="text"
          value={portalStore.editableTitle}
          oninput={(event) =>
            (portalStore.editableTitle = /** @type {HTMLInputElement} */ (event.target).value)}
          onclick={(event) => event.stopPropagation()}
          class="min-w-0 w-full max-w-md bg-transparent text-base font-semibold focus:outline-none"
          style="color: {shellText};"
          placeholder="Portal title"
        />
      {:else}
        <span
          class="block min-w-0 truncate text-sm sm:text-base font-semibold"
          style="color: {shellText};"
        >
          {portalStore.editableTitle}
        </span>
      {/if}
    </button>

    <div class="ml-auto flex items-center gap-1.5 sm:gap-2 flex-none">
      {#if isAnyUserAuthenticated}
        <button
          type="button"
          onclick={() => portalRequestsStore.toggle()}
          class="hidden sm:inline-flex h-9 items-center gap-2 rounded-md px-3 text-sm font-medium transition-colors"
          style="color: {shellText}; background-color: {portalRequestsStore.visible
            ? shellControl
            : 'transparent'};"
          title={t('portal.myRequests')}
        >
          <List class="w-4 h-4" />
          <span>{t('portal.myRequests')}</span>
          {#if portalRequestsStore.openCount > 0}
            <span
              class="min-w-5 h-5 px-1.5 inline-flex items-center justify-center rounded-full text-[11px] font-semibold"
              style="background-color: var(--ds-danger-subtle); color: var(--ds-text-danger);"
            >
              {portalRequestsStore.openCount}
            </span>
          {/if}
        </button>

        {#if portalApprovalsStore.pendingCount > 0}
          <button
            type="button"
            onclick={() => portalApprovalsStore.toggle()}
            class="hidden md:inline-flex h-9 items-center gap-2 rounded-md px-3 text-sm font-medium transition-colors"
            style="color: {shellText}; background-color: {portalApprovalsStore.visible
              ? shellControl
              : 'transparent'};"
            title="My approvals"
          >
            <ShieldCheck class="w-4 h-4" />
            <span>Approvals</span>
            <span
              class="min-w-5 h-5 px-1.5 inline-flex items-center justify-center rounded-full text-[11px] font-semibold"
              style="background-color: var(--ds-warning-subtle, #fffbeb); color: var(--ds-text-warning, #92400e);"
            >
              {portalApprovalsStore.pendingCount}
            </span>
          </button>
        {/if}
      {/if}

      {#if isInternalUser}
        <div class="relative">
          <button
            type="button"
            onclick={() => {
              portalStore.showMainMenu = !portalStore.showMainMenu;
              portalStore.showProfileMenu = false;
            }}
            class="h-9 w-9 inline-flex items-center justify-center rounded-md transition-colors"
            style="color: {shellTextSubtle};"
            title="Portal settings"
            aria-label="Portal settings"
          >
            <Settings class="w-[18px] h-[18px]" />
          </button>

          {#if portalStore.showMainMenu}
            <button
              type="button"
              class="fixed inset-0 z-[-1] cursor-default"
              onclick={() => (portalStore.showMainMenu = false)}
              aria-label="Close portal settings"
            ></button>
            <div
              class="absolute top-11 right-0 w-56 rounded-lg border p-1.5 shadow-lg"
              style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
            >
              <button type="button" onclick={() => navigate('/')} class="portal-menu-item">
                <ArrowLeft class="w-4 h-4" />
                <span>{t('portal.backToApp')}</span>
              </button>
              <button
                type="button"
                onclick={() => {
                  portalStore.showCustomizePanel = true;
                  portalStore.showMainMenu = false;
                }}
                class="portal-menu-item"
              >
                <Palette class="w-4 h-4" />
                <span>{t('portal.customizeButton')}</span>
              </button>
              <button
                type="button"
                onclick={() => {
                  portalStore.toggleTheme();
                  portalStore.showMainMenu = false;
                }}
                class="portal-menu-item"
              >
                {#if portalStore.isDarkMode}
                  <Sun class="w-4 h-4" />
                  <span>{t('portal.lightMode')}</span>
                {:else}
                  <Moon class="w-4 h-4" />
                  <span>{t('portal.darkMode')}</span>
                {/if}
              </button>
            </div>
          {/if}
        </div>
      {/if}

      <div class="relative">
        {#if isAnyUserAuthenticated}
          <button
            type="button"
            id="portal-avatar-button"
            onclick={() => {
              portalStore.showProfileMenu = !portalStore.showProfileMenu;
              portalStore.showMainMenu = false;
            }}
            class="relative h-9 w-9 inline-flex items-center justify-center rounded-full border transition-colors"
            style="color: {shellText}; border-color: {hasVisualBackground ? 'rgba(255,255,255,0.28)' : 'var(--ds-border)'}; background-color: {hasVisualBackground ? 'rgba(255,255,255,0.1)' : 'var(--ds-surface-card)'};"
            aria-label="Open account menu"
          >
            <span
              data-testid="portal-user-avatar-fallback"
              class="relative z-0 flex items-center justify-center text-xs font-semibold select-none"
              aria-hidden="true"
            >
              {#if accountInitials}
                {accountInitials}
              {:else}
                <User class="w-[18px] h-[18px]" />
              {/if}
            </span>
            {#if account?.avatar}
              <img
                src={account.avatar}
                alt=""
                class="absolute inset-0 z-10 h-full w-full rounded-full object-cover"
                data-testid="portal-user-avatar"
                onerror={removeBrokenAvatar}
              />
            {/if}
            {#if portalRequestsStore.openCount + portalApprovalsStore.pendingCount > 0}
              <span
                class="sm:hidden absolute -top-1 -right-1 min-w-4 h-4 px-1 inline-flex items-center justify-center rounded-full text-[10px] font-semibold text-white bg-red-500"
              >
                {portalRequestsStore.openCount + portalApprovalsStore.pendingCount}
              </span>
            {/if}
          </button>
        {:else}
          <button
            type="button"
            id="portal-avatar-button"
            onclick={handleLoginClick}
            class="h-9 inline-flex items-center gap-2 rounded-md px-3 text-sm font-medium"
            style="color: {shellText}; background-color: {shellControl};"
          >
            <User class="w-4 h-4" />
            <span class="hidden sm:inline">{t('portal.signIn')}</span>
          </button>
        {/if}

        {#if portalStore.showProfileMenu && isAnyUserAuthenticated}
          <button
            type="button"
            class="fixed inset-0 z-[-1] cursor-default"
            onclick={() => (portalStore.showProfileMenu = false)}
            aria-label="Close account menu"
          ></button>
          <div
            class="portal-account-menu fixed left-4 right-4 top-[4.5rem] sm:absolute sm:left-auto sm:right-0 sm:top-11 sm:w-72 rounded-lg border p-1.5 shadow-lg"
            style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
          >
            {#if account}
              <div class="px-3 py-3 mb-1 border-b flex items-center gap-3" style="border-color: var(--ds-border);">
                <div class="relative w-9 h-9 flex-none">
                  <div
                    data-testid="portal-account-avatar-fallback"
                    class="w-9 h-9 rounded-full flex items-center justify-center text-xs font-semibold select-none"
                    style="background-color: var(--ds-background-neutral); color: var(--ds-text);"
                    aria-hidden="true"
                  >
                    {#if accountInitials}
                      {accountInitials}
                    {:else}
                      <User class="w-4 h-4" />
                    {/if}
                  </div>
                  {#if account.avatar}
                    <img
                      src={account.avatar}
                      alt=""
                      class="absolute inset-0 z-10 w-9 h-9 rounded-full object-cover"
                      onerror={removeBrokenAvatar}
                    />
                  {/if}
                </div>
                <div class="min-w-0">
                  <div class="font-medium text-sm truncate" style="color: var(--ds-text);">
                    {account.name}
                  </div>
                  <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
                    {account.email}
                  </div>
                </div>
              </div>
            {/if}

            <button type="button" onclick={goHome} class="portal-menu-item sm:hidden">
              <Home class="w-4 h-4" />
              <span>Portal home</span>
            </button>
            <button
              type="button"
              onclick={() => portalRequestsStore.toggle()}
              class="portal-menu-item"
              class:portal-menu-active={portalRequestsStore.visible}
            >
              <List class="w-4 h-4" />
              <span class="flex-1">{t('portal.myRequests')}</span>
              {#if portalRequestsStore.openCount > 0}
                <span class="portal-menu-count">{portalRequestsStore.openCount}</span>
              {/if}
            </button>
            <button
              type="button"
              data-testid="portal-drafts-link"
              onclick={() => portalDraftsStore.toggle()}
              class="portal-menu-item"
              class:portal-menu-active={portalDraftsStore.visible}
            >
              <FileText class="w-4 h-4" />
              <span>{t('portal.myDrafts')}</span>
            </button>
            <button
              type="button"
              onclick={() => portalApprovalsStore.toggle()}
              class="portal-menu-item"
              class:portal-menu-active={portalApprovalsStore.visible}
            >
              <CheckSquare class="w-4 h-4" />
              <span class="flex-1">My approvals</span>
              {#if portalApprovalsStore.pendingCount > 0}
                <span class="portal-menu-count">{portalApprovalsStore.pendingCount}</span>
              {/if}
            </button>
            {#if account?.canManageProfile}
              <button
                type="button"
                data-testid="portal-profile-link"
                onclick={goToProfile}
                class="portal-menu-item"
              >
                <KeyRound class="w-4 h-4" />
                <span>{t('portal.profileAndSecurity') || 'Profile & security'}</span>
              </button>
            {/if}
            <div class="my-1 border-t" style="border-color: var(--ds-border);"></div>
            <button
              type="button"
              data-testid="portal-logout"
              onclick={handleLogout}
              class="portal-menu-item"
              style="color: var(--ds-text-danger);"
            >
              <LogOut class="w-4 h-4" />
              <span>{t('portal.signOut')}</span>
            </button>
          </div>
        {/if}
      </div>
    </div>
  </div>
</header>

<style>
  .portal-account-menu {
    max-width: calc(100vw - 2rem);
  }

  @media (max-width: 639px) {
    .portal-account-menu {
      left: max(1rem, env(safe-area-inset-left));
      right: max(1rem, env(safe-area-inset-right));
    }
  }

  .portal-menu-item {
    display: flex;
    width: 100%;
    align-items: center;
    gap: 0.75rem;
    border-radius: 0.375rem;
    padding: 0.625rem 0.75rem;
    text-align: left;
    font-size: 0.875rem;
    color: var(--ds-text);
    transition: background-color 120ms ease;
  }

  .portal-menu-item:hover,
  .portal-menu-active {
    background-color: var(--ds-background-neutral);
  }

  .portal-menu-count {
    min-width: 1.25rem;
    height: 1.25rem;
    padding-inline: 0.375rem;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 9999px;
    font-size: 0.6875rem;
    font-weight: 600;
    background-color: var(--ds-background-neutral);
    color: var(--ds-text-subtle);
  }

  .portal-header-branded :global(button:hover) {
    background-color: rgba(255, 255, 255, 0.14);
  }

  .portal-header-branded :global(.portal-menu-item:hover),
  .portal-header-branded :global(.portal-menu-active) {
    background-color: var(--ds-background-neutral);
  }
</style>
