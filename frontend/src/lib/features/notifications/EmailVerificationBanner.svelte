<script>
  import { Mail, RefreshCw, X } from 'lucide-svelte';
  import { api } from '../../api.js';

  let { show = false, ondismiss } = $props();

  let resending = $state(false);
  let resendSuccess = $state(false);
  let resendError = $state(null);

  async function handleResend() {
    try {
      resending = true;
      resendError = null;
      resendSuccess = false;
      await api.auth.resendVerification();
      resendSuccess = true;
      // Clear success message after 5 seconds
      setTimeout(() => {
        resendSuccess = false;
      }, 5000);
    } catch (err) {
      resendError = err.message || 'Failed to send verification email';
    } finally {
      resending = false;
    }
  }

  function handleDismiss() {
    ondismiss?.();
  }
</script>

{#if show}
  <div class="bg-amber-50 border-b border-amber-200">
    <div class="max-w-7xl mx-auto py-3 px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between flex-wrap gap-2">
        <div class="flex items-center gap-3">
          <div class="flex-shrink-0">
            <Mail class="h-5 w-5 text-amber-600" />
          </div>
          <div class="text-sm text-amber-800">
            <p class="font-medium">
              Please verify your email address
            </p>
            <p class="text-amber-700">
              We've sent a verification link to your email. Click the link to complete your account setup.
            </p>
          </div>
        </div>
        <div class="flex items-center gap-3">
          {#if resendSuccess}
            <span class="text-sm" style="color: var(--ds-text-success);">Verification email sent!</span>
          {:else if resendError}
            <span class="text-sm" style="color: var(--ds-text-danger);">{resendError}</span>
          {/if}
          <button
            onclick={handleResend}
            disabled={resending}
            class="inline-flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-amber-800 bg-amber-100 rounded-md hover:bg-amber-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-500 disabled:opacity-50"
          >
            {#if resending}
              <RefreshCw class="h-4 w-4 animate-spin" />
              Sending...
            {:else}
              <RefreshCw class="h-4 w-4" />
              Resend email
            {/if}
          </button>
          <button
            onclick={handleDismiss}
            class="p-1.5 text-amber-600 hover:text-amber-800 hover:bg-amber-100 rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-500"
            title="Dismiss"
          >
            <X class="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
