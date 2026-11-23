<script>
  const dialogId = $props.id();
  import X from '@lucide/svelte/icons/x';
  import Mail from '@lucide/svelte/icons/mail';
  import Lock from '@lucide/svelte/icons/lock';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import { get } from 'svelte/store';
  import { currentRoute } from '../router.js';
  import { authStore } from '../stores/auth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { isWebAuthnSupported, prepareCredentialRequestOptions, processCredentialRequestResponse } from '../utils/webauthn-utils.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import { customerAccess } from '../api/customerAccess.js';
  import CustomerGitHubButtons from '../customer/CustomerGitHubButtons.svelte';

  // One sign-in flow for portals and boards. The caller owns the open flag and
  // the post-sign-in refresh; the modal only renders the shared mechanisms.
  let { kind = 'portal', slug = null, open = false, visual = null, onclose = null, onloginsuccess = null } = $props();

  let email = $state('');
  let password = $state('');
  let loginMode = $state('magic'); // 'magic' or 'internal'
  let busy = $state(false);
  let error = $state('');
  let internalError = $state('');
  let emailSent = $state(false);
  let providers = $state([]);
  let passkeys = $state(false);
  let passkeySupported = $state(false);
  let hasVisual = $derived(!!visual?.hasVisual);
  let headerStyle = $derived(hasVisual && visual?.headerBackgroundStyle ? visual.headerBackgroundStyle : 'background-color: var(--ds-surface-card);');

  // Provider and passkey availability is read when the modal opens; the page
  // never fetches status just to decide whether it may render.
  $effect(() => {
    if (!open || !slug) return;
    let active = true;
    customerAccess.status(kind, slug)
      .then((status) => { if (active) { providers = status.providers || []; passkeys = status.passkeys === true; } })
      .catch(() => { if (active) { providers = []; passkeys = false; } });
    return () => { active = false; };
  });

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
    if (open && typeof window !== 'undefined') {
      const stashed = window.sessionStorage.getItem('portal_pending_email');
      if (stashed && !email) {
        email = stashed;
        window.sessionStorage.removeItem('portal_pending_email');
      }
    }
  });

  function reset() {
    email = '';
    password = '';
    loginMode = 'magic';
    busy = false;
    error = '';
    internalError = '';
    emailSent = false;
    authStore.clearError();
  }

  function closeModal() {
    reset();
    onclose?.();
  }

  // The portal page the customer was on when they asked for a sign-in link.
  // The server echoes it through the emailed verify URL so the link returns
  // them to the same request instead of the portal home. Recovery from an
  // expired/used link stashes its intended destination; prefer that over the
  // verify page sitting in the address bar.
  function isSamePortalPath(value, portalSlug) {
    if (!value || typeof value !== 'string' || !value.startsWith('/')) return false;
    if (value.startsWith('//')) return false;
    const base = `/portal/${portalSlug}`;
    return value === base || value.startsWith(`${base}/`) || value.startsWith(`${base}?`);
  }

  function resolveNextPath() {
    if (typeof window === 'undefined' || !slug) return null;

    const stashed = window.sessionStorage.getItem('portal_pending_next');
    if (stashed && isSamePortalPath(stashed, slug)) return stashed;

    const route = get(currentRoute);
    const path = route.path || '';
    const base = `/portal/${slug}`;
    if (path === `${base}/verify`) return null;
    if (path !== base && !path.startsWith(`${base}/`)) return null;

    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(route.query || {})) {
      // Never echo a one-time token back through the emailed link.
      if (key === 'token' || value === undefined || value === null) continue;
      params.append(key, String(value));
    }
    const query = params.toString();
    const candidate = query ? `${path}?${query}` : path;
    return isSamePortalPath(candidate, slug) ? candidate : null;
  }

  async function handleMagicLinkSubmit(e) {
    e.preventDefault();
    if (!email.trim()) return;
    busy = true;
    error = '';
    try {
      const body = { email: email.trim() };
      if (kind === 'portal') {
        const next = resolveNextPath();
        if (next) body.next = next;
      }
      await customerAccess.login(kind, slug, 'request', body);
      emailSent = true;
    } catch (err) {
      error = err.message || 'Failed to send sign-in link';
    } finally {
      busy = false;
    }
  }

  async function handlePasskeyLogin() {
    busy = true;
    error = '';
    try {
      const challenge = await customerAccess.login(kind, slug, 'passkey/start');
      const credential = await navigator.credentials.get(prepareCredentialRequestOptions(challenge.publicKey));
      if (!(credential instanceof PublicKeyCredential)) throw new Error('Sign-in was cancelled.');
      await customerAccess.login(kind, slug, 'passkey/complete', {
        sessionId: challenge.sessionId,
        response: processCredentialRequestResponse(credential),
      });
      await onloginsuccess?.();
      closeModal();
    } catch (err) {
      error = err?.name === 'NotAllowedError' ? 'Passkey sign-in was cancelled.' : err.message || 'Passkey sign-in failed';
    } finally {
      busy = false;
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
      remember_me: true,
    });

    if (result.success) {
      await onloginsuccess?.();
      closeModal();
    } else {
      internalError = authStore.error || 'Login failed';
    }
  }

  function switchToInternal() {
    loginMode = 'internal';
    password = '';
    internalError = '';
    error = '';
  }

  function switchToMagicLink() {
    loginMode = 'magic';
    password = '';
    internalError = '';
    error = '';
  }
</script>

{#if open}
  <ModalBackdrop show={open} opacity={0.38} blur={2} ariaLabelledBy="{dialogId}-title" onclose={closeModal}>
    <div
      class="w-full max-w-md rounded-lg border shadow-xl overflow-hidden"
      style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
    >
      {#if emailSent}
        <div class="p-6 sm:p-7" data-testid="portal-login-email-sent">
          <div
            class="-mx-6 sm:-mx-7 -mt-6 sm:-mt-7 px-6 sm:px-7 py-5 flex items-start justify-between gap-4 mb-6 border-b"
            style="{headerStyle} border-color: {hasVisual ? 'rgba(255,255,255,0.18)' : 'var(--ds-border)'};"
          >
            <div class="flex items-start gap-3">
              <div class="w-9 h-9 rounded-md flex items-center justify-center" style="background-color: {hasVisual ? 'rgba(255,255,255,0.14)' : 'var(--ds-background-neutral)'}; color: {hasVisual ? '#ffffff' : 'var(--ds-text)'};">
                <Mail class="w-4 h-4" />
              </div>
              <div>
                <h2 id="{dialogId}-title" class="text-xl font-semibold" style="color: {hasVisual ? '#ffffff' : 'var(--ds-text)'};">{t('portal.checkYourEmail')}</h2>
                <p class="text-sm mt-1" style="color: {hasVisual ? 'rgba(255,255,255,0.82)' : 'var(--ds-text-subtle)'};">{t('portal.magicLinkSent')}</p>
              </div>
            </div>
            <button type="button" onclick={closeModal} class="modal-close" style="color: {hasVisual ? 'rgba(255,255,255,0.88)' : 'var(--ds-text-subtle)'};" aria-label="Close">
              <X class="w-5 h-5" />
            </button>
          </div>
          <p class="text-sm mb-5" style="color: var(--ds-text-subtle);">
            {t('portal.linkExpiresIn')}
          </p>
          <button
            onclick={() => { emailSent = false; email = ''; }}
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
            style="{headerStyle} border-color: {hasVisual ? 'rgba(255,255,255,0.18)' : 'var(--ds-border)'};"
          >
            <div>
              <h2 id="{dialogId}-title" class="text-xl font-semibold" style="color: {hasVisual ? '#ffffff' : 'var(--ds-text)'};">{t('portal.signInTitle')}</h2>
              <p class="text-sm mt-1 leading-relaxed" style="color: {hasVisual ? 'rgba(255,255,255,0.82)' : 'var(--ds-text-subtle)'};">{t('portal.signInDescription')}</p>
            </div>
            <button type="button" onclick={closeModal} class="modal-close" style="color: {hasVisual ? 'rgba(255,255,255,0.88)' : 'var(--ds-text-subtle)'};" aria-label="Close">
              <X class="w-5 h-5" />
            </button>
          </div>

          {#if error}
            <AlertBox variant="error" message={error} class="mb-4" />
          {/if}

          <div class="space-y-3 mb-4"><CustomerGitHubButtons {kind} {slug} {providers} /></div>
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
                  disabled={busy}
                />
              </div>
            </div>

            <Button
              variant="primary"
              type="submit"
              fullWidth={true}
              loading={busy}
              disabled={busy || !email.trim()}
              dataTestid="portal-login-request-magic-link"
            >
              {#if !busy}
                <Mail class="w-4 h-4 mr-2" />
              {/if}
              {busy ? t('portal.sending') : t('portal.sendMagicLink')}
            </Button>
          </form>

          <p class="mt-4 text-xs" style="color: var(--ds-text-subtle);">
            {t('portal.noAccountNeeded')}
          </p>

          {#if passkeySupported && passkeys}
            <div class="my-5 flex items-center gap-3 text-xs" style="color: var(--ds-text-subtle);">
              <span class="flex-1 h-px" style="background-color: var(--ds-border);"></span>
              <span>or</span>
              <span class="flex-1 h-px" style="background-color: var(--ds-border);"></span>
            </div>
            <Button
              variant="secondary"
              type="button"
              fullWidth={true}
              loading={busy}
              disabled={busy}
              onclick={handlePasskeyLogin}
              dataTestid="portal-passkey-login"
            >
              <KeyRound class="w-4 h-4 mr-2" />
              {t('portal.signInWithPasskey') || 'Sign in with passkey'}
            </Button>
          {/if}

          {#if kind === 'portal'}
            <div class="mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
              <button
                onclick={switchToInternal}
                class="text-sm hover:underline"
                style="color: var(--ds-text-subtle);"
              >
                {t('portal.internalSignIn')}
              </button>
            </div>
          {/if}
        </div>
      {:else}
        <div class="p-6 sm:p-7">
          <div
            class="-mx-6 sm:-mx-7 -mt-6 sm:-mt-7 px-6 sm:px-7 py-5 flex items-start justify-between gap-4 mb-6 border-b"
            style="{headerStyle} border-color: {hasVisual ? 'rgba(255,255,255,0.18)' : 'var(--ds-border)'};"
          >
            <div>
              <div class="text-xs font-semibold uppercase tracking-[0.14em] mb-2" style="color: {hasVisual ? 'rgba(255,255,255,0.72)' : 'var(--ds-text-subtle)'};">Windshift staff</div>
              <h2 id="{dialogId}-title" class="text-xl font-semibold" style="color: {hasVisual ? '#ffffff' : 'var(--ds-text)'};">{t('portal.internalSignIn')}</h2>
            </div>
            <button type="button" onclick={closeModal} class="modal-close" style="color: {hasVisual ? 'rgba(255,255,255,0.88)' : 'var(--ds-text-subtle)'};" aria-label="Close">
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
