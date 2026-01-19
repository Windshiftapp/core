<script>
  import { onMount } from 'svelte';
  import { Loader2, CheckCircle, XCircle } from 'lucide-svelte';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  // Props
  let { slug, token, onSuccess, onError } = $props();

  let status = $state('verifying'); // verifying, success, error
  let errorMessage = $state('');

  onMount(async () => {
    if (!token) {
      status = 'error';
      errorMessage = t('portal.invalidLink') || 'Invalid or missing token';
      onError?.(errorMessage);
      return;
    }

    const result = await portalAuthStore.verifyMagicLink(slug, token);

    if (result.success) {
      status = 'success';
      onSuccess?.(result.customer);
    } else {
      status = 'error';
      errorMessage = result.message || t('portal.verificationFailed') || 'Failed to verify link';
      onError?.(errorMessage);
    }
  });
</script>

<div class="flex flex-col items-center justify-center min-h-[300px] p-8">
  {#if status === 'verifying'}
    <div class="text-center">
      <div class="w-16 h-16 mx-auto mb-6 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
        <Loader2 class="w-8 h-8 animate-spin" style="color: var(--ds-icon-brand);" />
      </div>
      <h2 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">
        {t('portal.verifying') || 'Verifying your link...'}
      </h2>
      <p style="color: var(--ds-text-subtle);">
        {t('portal.pleaseWait') || 'Please wait while we sign you in.'}
      </p>
    </div>
  {:else if status === 'success'}
    <div class="text-center">
      <div class="w-16 h-16 mx-auto mb-6 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-success-subtle);">
        <CheckCircle class="w-8 h-8" style="color: var(--ds-icon-success);" />
      </div>
      <h2 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">
        {t('portal.signInSuccess') || 'Successfully signed in!'}
      </h2>
      <p style="color: var(--ds-text-subtle);">
        {t('portal.redirecting') || 'Redirecting you to the portal...'}
      </p>
    </div>
  {:else if status === 'error'}
    <div class="text-center">
      <div class="w-16 h-16 mx-auto mb-6 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-danger-subtle);">
        <XCircle class="w-8 h-8" style="color: var(--ds-icon-danger);" />
      </div>
      <h2 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">
        {t('portal.verificationFailed') || 'Sign in failed'}
      </h2>
      <p class="mb-6" style="color: var(--ds-text-subtle);">
        {errorMessage}
      </p>
      <a
        href="/portal/{slug}"
        class="inline-flex items-center gap-2 px-4 py-2 rounded-lg font-medium transition-colors"
        style="background-color: var(--ds-background-brand-bold); color: var(--ds-text-inverse);"
      >
        {t('portal.backToPortal') || 'Back to Portal'}
      </a>
    </div>
  {/if}
</div>
