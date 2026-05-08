<script>
  import { onMount } from 'svelte';
  import { ArrowLeft, KeyRound, Plus, Trash2, ShieldCheck, X } from 'lucide-svelte';
  import { portalStore } from '../stores/portal.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import {
    isWebAuthnSupported,
    prepareCredentialCreationOptions,
    processCredentialCreationResponse,
    getWebAuthnErrorMessage,
  } from '../utils/webauthn-utils.js';
  import Button from '../components/Button.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';

  let credentials = $state([]);
  let loadingCredentials = $state(false);
  let listError = $state('');
  let actionError = $state('');

  let showAddModal = $state(false);
  let newCredentialName = $state('');
  let registering = $state(false);

  let removingId = $state(null);

  // Browser support for passkeys (recomputed client-side).
  let passkeySupported = $state(false);
  $effect(() => {
    if (typeof window !== 'undefined') {
      passkeySupported = isWebAuthnSupported();
    }
  });

  let slug = $derived(portalStore.currentSlug);

  onMount(async () => {
    await loadCredentials();
  });

  async function loadCredentials() {
    if (!slug) return;
    loadingCredentials = true;
    listError = '';
    try {
      const result = await api.portalPasskey.list(slug);
      credentials = Array.isArray(result) ? result : [];
      portalAuthStore.setPasskeyCount(credentials.length);
    } catch (err) {
      listError = err?.message || 'Failed to load passkeys';
    } finally {
      loadingCredentials = false;
    }
  }

  function openAddModal() {
    newCredentialName = '';
    actionError = '';
    showAddModal = true;
  }

  function closeAddModal() {
    if (registering) return;
    showAddModal = false;
    newCredentialName = '';
    actionError = '';
  }

  async function handleRegister(e) {
    e?.preventDefault?.();
    const name = newCredentialName.trim();
    if (!name) {
      actionError = 'Please give this passkey a name.';
      return;
    }
    if (!passkeySupported) {
      actionError = 'Passkeys are not supported in this browser.';
      return;
    }
    registering = true;
    actionError = '';
    try {
      const startResponse = await api.portalPasskey.startRegistration(slug, name);
      const creationOptions = prepareCredentialCreationOptions(startResponse);
      const credential = await navigator.credentials.create(creationOptions);
      if (!credential) throw new Error('No credential returned from authenticator');
      const processed = processCredentialCreationResponse(/** @type {any} */ (credential));
      await api.portalPasskey.completeRegistration(slug, {
        sessionId: startResponse.sessionId,
        credentialName: name,
        response: processed,
      });
      showAddModal = false;
      newCredentialName = '';
      await loadCredentials();
    } catch (err) {
      actionError = getWebAuthnErrorMessage(err);
    } finally {
      registering = false;
    }
  }

  async function handleRemove(credential) {
    const ok = window.confirm(
      `Remove passkey "${credential.credential_name}"? You won't be able to sign in with this device anymore.`
    );
    if (!ok) return;
    removingId = credential.id;
    actionError = '';
    try {
      await api.portalPasskey.remove(slug, credential.id);
      await loadCredentials();
    } catch (err) {
      actionError = err?.message || 'Failed to remove passkey';
    } finally {
      removingId = null;
    }
  }

  function formatDate(value) {
    if (!value) return '';
    try {
      return new Date(value).toLocaleString();
    } catch (_err) {
      return value;
    }
  }

  function backToPortal() {
    if (slug) navigate(`/portal/${slug}`);
  }
</script>

<div class="max-w-3xl mx-auto px-4 py-8">
  <button
    onclick={backToPortal}
    class="flex items-center gap-2 text-sm mb-6 hover:underline"
    style="color: var(--ds-text-link);"
  >
    <ArrowLeft class="w-4 h-4" />
    {t('portal.backToPortal') || 'Back to portal'}
  </button>

  <h1 class="text-3xl font-bold mb-1" style="color: var(--ds-text);">
    {t('portal.profileTitle') || 'Profile'}
  </h1>
  <p class="text-sm mb-8" style="color: var(--ds-text-subtle);">
    {t('portal.profileSubtitle') || 'Manage how you sign in to this portal.'}
  </p>

  <!-- Profile section -->
  <section
    class="rounded-lg border p-6 mb-8"
    style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
  >
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      {t('portal.profileSection') || 'Account'}
    </h2>
    {#if $portalAuthStore.customer}
      <dl class="grid grid-cols-1 sm:grid-cols-3 gap-y-3 text-sm">
        <dt style="color: var(--ds-text-subtle);">{t('common.name') || 'Name'}</dt>
        <dd class="sm:col-span-2" style="color: var(--ds-text);">
          {$portalAuthStore.customer.name || '—'}
        </dd>
        <dt style="color: var(--ds-text-subtle);">{t('common.email') || 'Email'}</dt>
        <dd class="sm:col-span-2" style="color: var(--ds-text);">
          {$portalAuthStore.customer.email}
        </dd>
      </dl>
    {:else}
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        {t('portal.notSignedIn') || 'Not signed in.'}
      </p>
    {/if}
  </section>

  <!-- Security / passkeys section -->
  <section
    class="rounded-lg border p-6"
    style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
  >
    <div class="flex items-start justify-between gap-4 mb-1">
      <div>
        <h2 class="text-lg font-semibold flex items-center gap-2" style="color: var(--ds-text);">
          <ShieldCheck class="w-5 h-5" />
          {t('portal.passkeysTitle') || 'Passkeys'}
        </h2>
        <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          {t('portal.passkeysSubtitle') ||
            'Sign in faster and more securely without an email link.'}
        </p>
      </div>
      {#if passkeySupported}
        <Button
          variant="primary"
          icon={Plus}
          onclick={openAddModal}
          disabled={loadingCredentials}
          dataTestid="portal-add-passkey"
        >
          {t('portal.addPasskey') || 'Add a passkey'}
        </Button>
      {/if}
    </div>

    {#if !passkeySupported}
      <AlertBox
        variant="warning"
        class="mt-4"
        message={t('portal.passkeysUnsupported') ||
          'Your browser does not support passkeys. Use a recent version of Chrome, Safari, Edge, or Firefox.'}
      />
    {/if}

    {#if listError}
      <AlertBox variant="error" class="mt-4" message={listError} />
    {/if}
    {#if actionError && !showAddModal}
      <AlertBox variant="error" class="mt-4" message={actionError} />
    {/if}

    {#if loadingCredentials}
      <p class="text-sm mt-4" style="color: var(--ds-text-subtle);">
        {t('common.loading') || 'Loading…'}
      </p>
    {:else if credentials.length === 0}
      <div class="mt-6 py-8 text-center border-2 border-dashed rounded" style="border-color: var(--ds-border);">
        <KeyRound class="w-8 h-8 mx-auto mb-2" style="color: var(--ds-text-subtle);" />
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          {t('portal.noPasskeys') || 'No passkeys yet.'}
        </p>
      </div>
    {:else}
      <ul class="mt-4 divide-y" style="border-color: var(--ds-border);" data-testid="portal-passkey-list">
        {#each credentials as cred (cred.id)}
          <li class="py-3 flex items-center justify-between gap-4">
            <div class="min-w-0">
              <div class="font-medium truncate" style="color: var(--ds-text);">
                {cred.credential_name}
              </div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">
                {t('portal.passkeyAdded') || 'Added'}
                {formatDate(cred.created_at)}
                {#if cred.last_used_at}
                  · {t('portal.passkeyLastUsed') || 'last used'}
                  {formatDate(cred.last_used_at)}
                {/if}
              </div>
            </div>
            <button
              type="button"
              class="p-2 rounded hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors disabled:opacity-50"
              style="color: var(--ds-text-danger, #b91c1c);"
              disabled={removingId === cred.id}
              onclick={() => handleRemove(cred)}
              aria-label={t('common.remove') || 'Remove'}
            >
              <Trash2 class="w-4 h-4" />
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</div>

<!-- Add passkey modal -->
<ModalBackdrop
  show={showAddModal}
  closeOnClick={!registering}
  closeOnEscape={!registering}
  onclose={closeAddModal}
>
  <div
    class="relative w-full max-w-md rounded-xl shadow-2xl overflow-hidden"
    style="background-color: var(--ds-surface-card);"
  >
    <button
      type="button"
      onclick={closeAddModal}
      class="absolute top-3 right-3 p-2 rounded hover:bg-black/5 transition-colors"
      aria-label="Close"
      disabled={registering}
    >
      <X class="w-4 h-4" style="color: var(--ds-text);" />
    </button>
    <form onsubmit={handleRegister} class="p-6">
      <h3 class="text-lg font-semibold mb-1" style="color: var(--ds-text);">
        {t('portal.addPasskeyTitle') || 'Add a passkey'}
      </h3>
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
        {t('portal.addPasskeyHint') ||
          'Give this device a name so you can recognise it later.'}
      </p>

      {#if actionError}
        <AlertBox variant="error" class="mb-4" message={actionError} />
      {/if}

      <label for="passkey-name" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
        {t('portal.passkeyName') || 'Passkey name'}
      </label>
      <input
        id="passkey-name"
        type="text"
        bind:value={newCredentialName}
        placeholder="e.g. Personal MacBook"
        maxlength="100"
        autocomplete="off"
        disabled={registering}
        class="block w-full px-3 py-2 rounded border focus:outline-none focus:ring-2"
        style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
      />

      <div class="mt-6 flex items-center justify-end gap-2">
        <Button variant="secondary" type="button" onclick={closeAddModal} disabled={registering}>
          {t('common.cancel') || 'Cancel'}
        </Button>
        <Button
          variant="primary"
          type="submit"
          loading={registering}
          disabled={registering || !newCredentialName.trim()}
          dataTestid="portal-passkey-register-submit"
        >
          <KeyRound class="w-4 h-4 mr-2" />
          {t('portal.addPasskey') || 'Add a passkey'}
        </Button>
      </div>
    </form>
  </div>
</ModalBackdrop>
