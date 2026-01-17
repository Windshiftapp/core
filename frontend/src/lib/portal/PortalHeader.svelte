<script>
  import { Menu, ArrowLeft, Palette, Edit3, Check, Sun, Moon, User, LogOut, List } from 'lucide-svelte';
  import { authStore } from '../stores';
  import { portalStore, gradients } from '../stores/portal.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let hoveredMenuItem = $state(null);

  async function handleLogout() {
    await authStore.logout();
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
      class="w-10 h-10 rounded flex items-center justify-center text-white bg-white/10 backdrop-blur-sm hover:bg-white/20 transition-all shadow-lg border border-white/20"
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
        class="absolute top-16 left-0 min-w-[200px] rounded shadow-2xl border overflow-hidden bg-white/10 backdrop-blur-sm"
        style="border-color: rgba(255, 255, 255, 0.2);"
      >
        <!-- Back to App -->
        <button
          onclick={() => { window.location.href = '/'; portalStore.showMainMenu = false; }}
          class="w-full px-4 py-3 flex items-center gap-3 text-white transition-all hover:bg-white/20 text-left"
        >
          <ArrowLeft class="w-5 h-5" />
          <span class="font-medium">{t('portal.backToApp')}</span>
        </button>

        {#if authStore.isAuthenticated}
          <!-- Customize -->
          <button
            onclick={() => { portalStore.showCustomizePanel = true; portalStore.showMainMenu = false; }}
            class="w-full px-4 py-3 flex items-center gap-3 text-white transition-all hover:bg-white/20 text-left"
          >
            <Palette class="w-5 h-5" />
            <span class="font-medium">{t('portal.customize')}</span>
          </button>

          <!-- Edit (only show when not editing) -->
          {#if !portalStore.isEditing}
            <button
              onclick={() => { portalStore.toggleEditing(); portalStore.showMainMenu = false; }}
              class="w-full px-4 py-3 flex items-center gap-3 text-white transition-all hover:bg-white/20 text-left"
            >
              <Edit3 class="w-5 h-5" />
              <span class="font-medium">{t('common.edit')}</span>
            </button>
          {/if}
        {/if}

        <!-- Theme Toggle -->
        <button
          onclick={() => { portalStore.toggleTheme(); portalStore.showMainMenu = false; }}
          class="w-full px-4 py-3 flex items-center gap-3 text-white transition-all hover:bg-white/20 text-left"
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

  <!-- Done Button (visible when editing) -->
  {#if portalStore.isEditing}
    <button
      onclick={() => portalStore.toggleEditing()}
      class="flex items-center gap-2 px-4 py-2 bg-white/30 backdrop-blur-sm text-white rounded hover:bg-white/40 transition-all shadow-lg border border-white"
    >
      <Check class="w-5 h-5" />
      <span class="font-medium">Done</span>
    </button>
  {/if}
</div>
{/if}

<!-- Profile Menu - Top Right (hidden in requests view) -->
{#if !portalStore.showMyRequests}
<div class="fixed top-6 right-6 z-40">
  <div class="relative">
    <!-- Profile Avatar Button -->
    <button
      onclick={() => portalStore.showProfileMenu = !portalStore.showProfileMenu}
      class="w-10 h-10 rounded-full flex items-center justify-center text-white bg-white/10 backdrop-blur-sm hover:bg-white/20 transition-all shadow-lg border border-white/20"
    >
      <User class="w-5 h-5" />
    </button>

    <!-- Profile Dropdown Menu -->
    {#if portalStore.showProfileMenu}
      <div
        class="absolute top-14 right-0 w-64 rounded shadow-2xl border overflow-hidden"
        style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
      >
        {#if authStore.isAuthenticated && authStore.currentUser}
          <!-- Authenticated User Info -->
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

          <!-- Authenticated Menu Items -->
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
