<script>
  import { createEventDispatcher } from 'svelte';
  import { X, Mail, User, Lock, Building2 } from 'lucide-svelte';
  import { portalStore } from '../stores/portal.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { authStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import Button from '../components/Button.svelte';

  const dispatch = createEventDispatcher();

  let email = $state('');
  let password = $state('');
  let loginMode = $state('magic'); // 'magic' or 'internal'
  let internalError = $state('');

  function closeModal() {
    portalStore.showLoginDialog = false;
    email = '';
    password = '';
    loginMode = 'magic';
    internalError = '';
    portalAuthStore.clearError();
    portalAuthStore.resetEmailSent();
    authStore.clearError();
  }

  async function handleMagicLinkSubmit(e) {
    e.preventDefault();
    if (!email.trim()) return;

    const result = await portalAuthStore.requestMagicLink(portalStore.currentSlug, email.trim());
    if (result.success) {
      // Email sent - the store will update emailSent state
    }
  }

  async function handleInternalSubmit(e) {
    e.preventDefault();
    if (!email.trim() || !password) return;

    internalError = '';
    authStore.clearError();

    const result = await authStore.login({
      email_or_username: email.trim(),
      password: password,
      remember_me: true
    });

    if (result.success) {
      // Refresh portal auth state to detect internal user session
      await portalAuthStore.checkAuth(portalStore.currentSlug);
      dispatch('loginsuccess');
      closeModal();
    } else {
      internalError = authStore.error || 'Login failed';
    }
  }

  function switchToInternal() {
    loginMode = 'internal';
    password = '';
    internalError = '';
    portalAuthStore.clearError();
  }

  function switchToMagicLink() {
    loginMode = 'magic';
    password = '';
    internalError = '';
    authStore.clearError();
  }

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) {
      closeModal();
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') {
      closeModal();
    }
  }

</script>

<svelte:window onkeydown={handleKeydown} />

{#if portalStore.showLoginDialog}
  <!-- Modal Backdrop -->
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4"
    style="background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(8px);"
    onclick={handleBackdropClick}
  >
    <!-- Modal Content -->
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div
      class="w-full max-w-md rounded-2xl shadow-2xl overflow-hidden"
      style="background-color: {portalStore.isDarkMode ? '#1e293b' : '#ffffff'};"
      onclick={(e) => e.stopPropagation()}
    >
      {#if $portalAuthStore.emailSent}
        <!-- Email Sent Confirmation - Gradient Header -->
        <div
          class="px-8 py-12 text-white text-center relative"
          style="{portalStore.headerBackgroundStyle}"
        >
          <div class="absolute top-4 right-4">
            <button
              type="button"
              onclick={closeModal}
              class="p-2 rounded hover:bg-white/10 transition-all"
            >
              <X class="w-5 h-5" />
            </button>
          </div>

          <div class="flex justify-center mb-4">
            <div class="w-16 h-16 rounded-full bg-white/20 backdrop-blur-sm flex items-center justify-center">
              <Mail class="w-8 h-8 text-white" />
            </div>
          </div>
          <h2 class="text-2xl font-bold">{t('portal.checkYourEmail')}</h2>
          <p class="text-white/80 mt-2">{t('portal.magicLinkSent')}</p>
        </div>

        <!-- Confirmation Content -->
        <div class="px-8 py-6 text-center">
          <p class="text-sm mb-6" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
            {t('portal.linkExpiresIn')}
          </p>
          <button
            onclick={() => { portalAuthStore.resetEmailSent(); email = ''; }}
            class="text-sm font-medium transition-colors hover:underline"
            style="color: {portalStore.isDarkMode ? '#60a5fa' : '#2563eb'};"
          >
            {t('portal.useAnotherEmail')}
          </button>
        </div>

      {:else if loginMode === 'magic'}
        <!-- Magic Link Login Form - Gradient Header -->
        <div
          class="px-8 py-12 text-white text-center relative"
          style="{portalStore.headerBackgroundStyle}"
        >
          <div class="absolute top-4 right-4">
            <button
              type="button"
              onclick={closeModal}
              class="p-2 rounded hover:bg-white/10 transition-all"
            >
              <X class="w-5 h-5" />
            </button>
          </div>

          <div class="flex justify-center mb-4">
            <div class="w-16 h-16 rounded-full bg-white/20 backdrop-blur-sm flex items-center justify-center">
              <User class="w-8 h-8 text-white" />
            </div>
          </div>
          <h2 class="text-2xl font-bold">{t('portal.signInTitle')}</h2>
          <p class="text-white/80 mt-2">{t('portal.signInDescription')}</p>
        </div>

        <!-- Form Content -->
        <div class="px-8 py-6">
          {#if $portalAuthStore.error}
            <div class="mb-4 p-3 rounded" style="background-color: {portalStore.isDarkMode ? 'rgba(239, 68, 68, 0.1)' : '#fef2f2'}; border: 1px solid {portalStore.isDarkMode ? 'rgba(239, 68, 68, 0.3)' : '#fecaca'};">
              <p class="text-sm" style="color: {portalStore.isDarkMode ? '#fca5a5' : '#dc2626'};">{$portalAuthStore.error}</p>
            </div>
          {/if}

          <form onsubmit={handleMagicLinkSubmit} class="space-y-4">
            <div>
              <label for="email" class="block text-sm font-medium mb-2" style="color: {portalStore.isDarkMode ? '#e2e8f0' : '#374151'};">
                {t('common.email')}
              </label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <Mail class="h-4 w-4" style="color: {portalStore.isDarkMode ? '#64748b' : '#9ca3af'};" />
                </div>
                <input
                  id="email"
                  type="email"
                  bind:value={email}
                  placeholder={t('portal.enterEmail')}
                  required
                  class="block w-full pl-10 pr-3 py-2.5 rounded leading-5 focus:outline-none focus:ring-2 focus:ring-offset-0 transition-all"
                  style="background-color: {portalStore.isDarkMode ? '#334155' : '#f9fafb'}; color: {portalStore.isDarkMode ? '#e2e8f0' : '#111827'}; border: 1px solid {portalStore.isDarkMode ? '#475569' : '#e5e7eb'};"
                  disabled={$portalAuthStore.loading}
                />
              </div>
            </div>

            <Button
              variant="primary"
              type="submit"
              fullWidth={true}
              loading={$portalAuthStore.loading}
              disabled={$portalAuthStore.loading || !email.trim()}
            >
              {#if !$portalAuthStore.loading}
                <Mail class="w-4 h-4 mr-2" />
              {/if}
              {$portalAuthStore.loading ? t('portal.sending') : t('portal.sendMagicLink')}
            </Button>
          </form>

          <p class="mt-6 text-center text-sm" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
            {t('portal.noAccountNeeded')}
          </p>

          <!-- Internal login option -->
          <div class="mt-6 pt-4 border-t" style="border-color: {portalStore.isDarkMode ? '#475569' : '#e5e7eb'};">
            <button
              onclick={switchToInternal}
              class="w-full flex items-center justify-center gap-2 text-sm transition-colors hover:underline"
              style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};"
            >
              <Building2 class="w-4 h-4" />
              {t('portal.internalSignIn')}
            </button>
          </div>
        </div>

      {:else}
        <!-- Internal Login Form - Gradient Header -->
        <div
          class="px-8 py-12 text-white text-center relative"
          style="{portalStore.headerBackgroundStyle}"
        >
          <div class="absolute top-4 right-4">
            <button
              type="button"
              onclick={closeModal}
              class="p-2 rounded hover:bg-white/10 transition-all"
            >
              <X class="w-5 h-5" />
            </button>
          </div>

          <div class="flex justify-center mb-4">
            <div class="w-16 h-16 rounded-full bg-white/20 backdrop-blur-sm flex items-center justify-center">
              <Building2 class="w-8 h-8 text-white" />
            </div>
          </div>
          <h2 class="text-2xl font-bold">{t('portal.internalSignIn')}</h2>
        </div>

        <!-- Form Content -->
        <div class="px-8 py-6">
          {#if internalError}
            <div class="mb-4 p-3 rounded" style="background-color: {portalStore.isDarkMode ? 'rgba(239, 68, 68, 0.1)' : '#fef2f2'}; border: 1px solid {portalStore.isDarkMode ? 'rgba(239, 68, 68, 0.3)' : '#fecaca'};">
              <p class="text-sm" style="color: {portalStore.isDarkMode ? '#fca5a5' : '#dc2626'};">{internalError}</p>
            </div>
          {/if}

          <form onsubmit={handleInternalSubmit} class="space-y-4">
            <div>
              <label for="internal-email" class="block text-sm font-medium mb-2" style="color: {portalStore.isDarkMode ? '#e2e8f0' : '#374151'};">
                {t('common.email')}
              </label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <Mail class="h-4 w-4" style="color: {portalStore.isDarkMode ? '#64748b' : '#9ca3af'};" />
                </div>
                <input
                  id="internal-email"
                  type="text"
                  bind:value={email}
                  placeholder={t('portal.enterEmail')}
                  required
                  class="block w-full pl-10 pr-3 py-2.5 rounded leading-5 focus:outline-none focus:ring-2 focus:ring-offset-0 transition-all"
                  style="background-color: {portalStore.isDarkMode ? '#334155' : '#f9fafb'}; color: {portalStore.isDarkMode ? '#e2e8f0' : '#111827'}; border: 1px solid {portalStore.isDarkMode ? '#475569' : '#e5e7eb'};"
                  disabled={authStore.loading}
                />
              </div>
            </div>

            <div>
              <label for="password" class="block text-sm font-medium mb-2" style="color: {portalStore.isDarkMode ? '#e2e8f0' : '#374151'};">
                {t('portal.password')}
              </label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <Lock class="h-4 w-4" style="color: {portalStore.isDarkMode ? '#64748b' : '#9ca3af'};" />
                </div>
                <input
                  id="password"
                  type="password"
                  bind:value={password}
                  placeholder={t('portal.enterPassword')}
                  required
                  class="block w-full pl-10 pr-3 py-2.5 rounded leading-5 focus:outline-none focus:ring-2 focus:ring-offset-0 transition-all"
                  style="background-color: {portalStore.isDarkMode ? '#334155' : '#f9fafb'}; color: {portalStore.isDarkMode ? '#e2e8f0' : '#111827'}; border: 1px solid {portalStore.isDarkMode ? '#475569' : '#e5e7eb'};"
                  disabled={authStore.loading}
                />
              </div>
            </div>

            <Button
              variant="primary"
              type="submit"
              fullWidth={true}
              loading={authStore.loading}
              disabled={authStore.loading || !email.trim() || !password}
            >
              {authStore.loading ? t('portal.signingIn') : t('portal.signIn')}
            </Button>
          </form>

          <!-- Back to magic link option -->
          <div class="mt-6 pt-4 border-t" style="border-color: {portalStore.isDarkMode ? '#475569' : '#e5e7eb'};">
            <button
              onclick={switchToMagicLink}
              class="w-full flex items-center justify-center gap-2 text-sm transition-colors hover:underline"
              style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};"
            >
              <Mail class="w-4 h-4" />
              {t('portal.backToMagicLink')}
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}
