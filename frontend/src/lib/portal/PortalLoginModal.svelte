<script>
  import { X, Mail, Loader2 } from 'lucide-svelte';
  import { portalStore } from '../stores/portal.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let email = $state('');
  let authState = $state({});

  // Subscribe to auth store
  portalAuthStore.subscribe(value => {
    authState = value;
  });

  function closeModal() {
    portalStore.showLoginDialog = false;
    email = '';
    portalAuthStore.clearError();
    portalAuthStore.resetEmailSent();
  }

  async function handleSubmit(e) {
    e.preventDefault();
    if (!email.trim()) return;

    const result = await portalAuthStore.requestMagicLink(portalStore.slug, email.trim());
    if (result.success) {
      // Email sent - the store will update emailSent state
    }
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
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4"
    onclick={handleBackdropClick}
  >
    <!-- Modal Content -->
    <div
      class="relative w-full max-w-md rounded-xl shadow-2xl overflow-hidden"
      style="background-color: var(--ds-surface-card);"
    >
      <!-- Close Button -->
      <button
        onclick={closeModal}
        class="absolute top-4 right-4 p-2 rounded-full transition-colors"
        style="color: var(--ds-text-subtle);"
        aria-label="Close"
      >
        <X class="w-5 h-5" />
      </button>

      <div class="p-8">
        {#if authState.emailSent}
          <!-- Email Sent Confirmation -->
          <div class="text-center">
            <div class="w-16 h-16 mx-auto mb-6 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-success-subtle);">
              <Mail class="w-8 h-8" style="color: var(--ds-icon-success);" />
            </div>
            <h2 class="text-2xl font-bold mb-3" style="color: var(--ds-text);">
              {t('portal.checkYourEmail') || 'Check your email'}
            </h2>
            <p class="mb-6" style="color: var(--ds-text-subtle);">
              {t('portal.magicLinkSent') || "We've sent a sign-in link to your email. Click the link to access your portal account."}
            </p>
            <p class="text-sm mb-6" style="color: var(--ds-text-subtlest);">
              {t('portal.linkExpiresIn') || 'The link expires in 15 minutes.'}
            </p>
            <button
              onclick={() => { portalAuthStore.resetEmailSent(); email = ''; }}
              class="text-sm font-medium transition-colors"
              style="color: var(--ds-link);"
            >
              {t('portal.useAnotherEmail') || 'Use a different email'}
            </button>
          </div>
        {:else}
          <!-- Login Form -->
          <div class="text-center mb-8">
            <h2 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">
              {t('portal.signInTitle') || 'Sign in to your account'}
            </h2>
            <p style="color: var(--ds-text-subtle);">
              {t('portal.signInDescription') || 'Enter your email to receive a sign-in link'}
            </p>
          </div>

          <form onsubmit={handleSubmit} class="space-y-6">
            <div>
              <label for="email" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
                {t('common.email') || 'Email'}
              </label>
              <div class="relative">
                <Mail class="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style="color: var(--ds-text-subtle);" />
                <input
                  id="email"
                  type="email"
                  bind:value={email}
                  placeholder={t('portal.enterEmail') || 'Enter your email address'}
                  required
                  class="w-full pl-10 pr-4 py-3 rounded-lg border transition-colors focus:outline-none focus:ring-2"
                  style="
                    background-color: var(--ds-background-input);
                    border-color: var(--ds-border-input);
                    color: var(--ds-text);
                  "
                  disabled={authState.loading}
                />
              </div>
            </div>

            {#if authState.error}
              <div class="p-3 rounded-lg text-sm" style="background-color: var(--ds-background-danger-subtle); color: var(--ds-text-danger);">
                {authState.error}
              </div>
            {/if}

            <button
              type="submit"
              disabled={authState.loading || !email.trim()}
              class="w-full py-3 px-4 rounded-lg font-medium transition-colors flex items-center justify-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
              style="background-color: var(--ds-background-brand-bold); color: var(--ds-text-inverse);"
            >
              {#if authState.loading}
                <Loader2 class="w-5 h-5 animate-spin" />
                {t('common.sending') || 'Sending...'}
              {:else}
                <Mail class="w-5 h-5" />
                {t('portal.sendMagicLink') || 'Send sign-in link'}
              {/if}
            </button>
          </form>

          <p class="mt-6 text-center text-sm" style="color: var(--ds-text-subtlest);">
            {t('portal.noAccountNeeded') || "You don't need to create an account. Just enter your email and we'll send you a sign-in link."}
          </p>
        {/if}
      </div>
    </div>
  </div>
{/if}
