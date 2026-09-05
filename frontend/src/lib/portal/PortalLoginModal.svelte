<script>
  import { X, Mail, Lock, KeyRound } from '@lucide/svelte';
  import { portalCustomizationStore as portalStore } from '../stores/portal.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { authStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { isWebAuthnSupported } from '../utils/webauthn-utils.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';

  let { onloginsuccess } = $props();

  let email = $state('');
  let password = $state('');
  let loginMode = $state('magic'); // 'magic' or 'internal'
  let internalError = $state('');
  let passkeySupported = $state(false);
  let hasPortalVisual = $derived(portalStore.hasBackgroundImage || portalStore.hasGradient);

  // Browser support is decided client-side only; default to false during SSR.
  $effect(() => {
    if (typeof window !== 'undefined') {
      passkeySupported = isWebAuthnSupported();
    }
  });

  // Pre-fill the email when the modal is opened as part of the expired-link
  // recovery flow (see Portal.svelte:handleVerifyError). The hint is written
  // by the verify failure path; reading + clearing it here means it only
  // applies to the next time the modal opens.
  $effect(() => {
    if (portalStore.showLoginDialog && typeof window !== 'undefined') {
      const stashed = window.sessionStorage.getItem('portal_pending_email');
      if (stashed && !email) {
        email = stashed;
        window.sessionStorage.removeItem('portal_pending_email');
      }
    }
  });

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

  async function handlePasskeyLogin() {
    const result = await portalAuthStore.loginWithPasskey(portalStore.currentSlug);
    if (result.success) {
      portalStore.hydrateUserBootstrap(result.userBootstrap);
      onloginsuccess?.();
      closeModal();
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
      const userBootstrap = await portalAuthStore.checkAuth(portalStore.currentSlug);
      portalStore.hydrateUserBootstrap(userBootstrap);
      onloginsuccess?.();
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

</script>

{#if portalStore.showLoginDialog}
  <ModalBackdrop
    show={portalStore.showLoginDialog}
    opacity={0.38}
    blur={2}
    ariaLabelledBy="portal-login-title"
    onclose={closeModal}
  >
    <div
      class="w-full max-w-md rounded-lg border shadow-xl overflow-hidden"
      style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
    >
      {#if $portalAuthStore.emailSent}
        <div class="p-6 sm:p-7">
          <div
            class="-mx-6 sm:-mx-7 -mt-6 sm:-mt-7 px-6 sm:px-7 py-5 flex items-start justify-between gap-4 mb-6 border-b"
            style="{hasPortalVisual ? portalStore.headerBackgroundStyle : 'background-color: var(--ds-surface-card);'} border-color: {hasPortalVisual ? 'rgba(255,255,255,0.18)' : 'var(--ds-border)'};"
          >
            <div class="flex items-start gap-3">
              <div class="w-9 h-9 rounded-md flex items-center justify-center" style="background-color: {hasPortalVisual ? 'rgba(255,255,255,0.14)' : 'var(--ds-background-neutral)'}; color: {hasPortalVisual ? '#ffffff' : 'var(--ds-text)'};">
                <Mail class="w-4 h-4" />
              </div>
              <div>
                <h2 id="portal-login-title" class="text-xl font-semibold" style="color: {hasPortalVisual ? '#ffffff' : 'var(--ds-text)'};">{t('portal.checkYourEmail')}</h2>
                <p class="text-sm mt-1" style="color: {hasPortalVisual ? 'rgba(255,255,255,0.82)' : 'var(--ds-text-subtle)'};">{t('portal.magicLinkSent')}</p>
              </div>
            </div>
            <button type="button" onclick={closeModal} class="modal-close" style="color: {hasPortalVisual ? 'rgba(255,255,255,0.88)' : 'var(--ds-text-subtle)'};" aria-label="Close">
              <X class="w-5 h-5" />
            </button>
          </div>
          <p class="text-sm mb-5" style="color: var(--ds-text-subtle);">
            {t('portal.linkExpiresIn')}
          </p>
          <button
            onclick={() => { portalAuthStore.resetEmailSent(); email = ''; }}
            class="text-sm font-medium hover:underline"
            style="color: var(--ds-text-link);"
          >
            {t('portal.useAnotherEmail')}
          </button>
        </div>
      {:else if loginMode === 'magic'}
        <div class="p-6 sm:p-7">
          <div
            class="-mx-6 sm:-mx-7 -mt-6 sm:-mt-7 px-6 sm:px-7 py-5 flex items-start justify-between gap-4 mb-6 border-b"
            style="{hasPortalVisual ? portalStore.headerBackgroundStyle : 'background-color: var(--ds-surface-card);'} border-color: {hasPortalVisual ? 'rgba(255,255,255,0.18)' : 'var(--ds-border)'};"
          >
            <div>
              <h2 id="portal-login-title" class="text-xl font-semibold" style="color: {hasPortalVisual ? '#ffffff' : 'var(--ds-text)'};">{t('portal.signInTitle')}</h2>
              <p class="text-sm mt-1 leading-relaxed" style="color: {hasPortalVisual ? 'rgba(255,255,255,0.82)' : 'var(--ds-text-subtle)'};">{t('portal.signInDescription')}</p>
            </div>
            <button type="button" onclick={closeModal} class="modal-close" style="color: {hasPortalVisual ? 'rgba(255,255,255,0.88)' : 'var(--ds-text-subtle)'};" aria-label="Close">
              <X class="w-5 h-5" />
            </button>
          </div>

          {#if $portalAuthStore.error}
            <AlertBox variant="error" message={$portalAuthStore.error} class="mb-4" />
          {/if}

          <form onsubmit={handleMagicLinkSubmit} class="space-y-4">
            <div>
              <label for="email" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                {t('common.email')}
              </label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <Mail class="h-4 w-4" style="color: var(--ds-text-subtle);" />
                </div>
                <Input
                  id="email"
                  dataAutofocus
                  type="email"
                  bind:value={email}
                  placeholder={t('portal.enterEmail')}
                  required
                  class="block w-full pl-10 pr-3 py-2.5 rounded-md border leading-5 focus:outline-none focus:ring-2"
                  style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
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
              dataTestid="portal-login-request-magic-link"
            >
              {#if !$portalAuthStore.loading}
                <Mail class="w-4 h-4 mr-2" />
              {/if}
              {$portalAuthStore.loading ? t('portal.sending') : t('portal.sendMagicLink')}
            </Button>
          </form>

          <p class="mt-4 text-xs" style="color: var(--ds-text-subtle);">
            {t('portal.noAccountNeeded')}
          </p>

          {#if passkeySupported}
            <div class="my-5 flex items-center gap-3 text-xs" style="color: var(--ds-text-subtle);">
              <span class="flex-1 h-px" style="background-color: var(--ds-border);"></span>
              <span>or</span>
              <span class="flex-1 h-px" style="background-color: var(--ds-border);"></span>
            </div>
            <Button
              variant="secondary"
              type="button"
              fullWidth={true}
              loading={$portalAuthStore.loading}
              disabled={$portalAuthStore.loading}
              onclick={handlePasskeyLogin}
              dataTestid="portal-passkey-login"
            >
              <KeyRound class="w-4 h-4 mr-2" />
              {t('portal.signInWithPasskey') || 'Sign in with passkey'}
            </Button>
          {/if}

          <div class="mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
            <button
              onclick={switchToInternal}
              class="text-sm hover:underline"
              style="color: var(--ds-text-subtle);"
            >
              {t('portal.internalSignIn')}
            </button>
          </div>
        </div>
      {:else}
        <div class="p-6 sm:p-7">
          <div
            class="-mx-6 sm:-mx-7 -mt-6 sm:-mt-7 px-6 sm:px-7 py-5 flex items-start justify-between gap-4 mb-6 border-b"
            style="{hasPortalVisual ? portalStore.headerBackgroundStyle : 'background-color: var(--ds-surface-card);'} border-color: {hasPortalVisual ? 'rgba(255,255,255,0.18)' : 'var(--ds-border)'};"
          >
            <div>
              <div class="text-xs font-semibold uppercase tracking-[0.14em] mb-2" style="color: {hasPortalVisual ? 'rgba(255,255,255,0.72)' : 'var(--ds-text-subtle)'};">Windshift staff</div>
              <h2 id="portal-login-title" class="text-xl font-semibold" style="color: {hasPortalVisual ? '#ffffff' : 'var(--ds-text)'};">{t('portal.internalSignIn')}</h2>
            </div>
            <button type="button" onclick={closeModal} class="modal-close" style="color: {hasPortalVisual ? 'rgba(255,255,255,0.88)' : 'var(--ds-text-subtle)'};" aria-label="Close">
              <X class="w-5 h-5" />
            </button>
          </div>

          {#if internalError}
            <AlertBox variant="error" message={internalError} class="mb-4" />
          {/if}

          <form onsubmit={handleInternalSubmit} class="space-y-4">
            <div>
              <label for="internal-email" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                {t('common.email')}
              </label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <Mail class="h-4 w-4" style="color: var(--ds-text-subtle);" />
                </div>
                <Input
                  id="internal-email"
                  dataAutofocus
                  type="text"
                  bind:value={email}
                  placeholder={t('portal.enterEmail')}
                  required
                  class="block w-full pl-10 pr-3 py-2.5 rounded-md border leading-5 focus:outline-none focus:ring-2"
                  style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
                  disabled={authStore.loading}
                />
              </div>
            </div>

            <div>
              <label for="password" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                {t('portal.password')}
              </label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <Lock class="h-4 w-4" style="color: var(--ds-text-subtle);" />
                </div>
                <Input
                  id="password"
                  type="password"
                  bind:value={password}
                  placeholder={t('portal.enterPassword')}
                  required
                  class="block w-full pl-10 pr-3 py-2.5 rounded-md border leading-5 focus:outline-none focus:ring-2"
                  style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
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

          <div class="mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
            <button
              onclick={switchToMagicLink}
              class="flex items-center gap-2 text-sm hover:underline"
              style="color: var(--ds-text-subtle);"
            >
              <Mail class="w-4 h-4" />
              {t('portal.backToMagicLink')}
            </button>
          </div>
        </div>
      {/if}
    </div>
  </ModalBackdrop>
{/if}

<style>
  .modal-close {
    flex: none;
    padding: 0.375rem;
    border-radius: 0.375rem;
    color: var(--ds-text-subtle);
    transition: background-color 120ms ease;
  }

  .modal-close:hover {
    background-color: var(--ds-background-neutral);
  }
</style>
