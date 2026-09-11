<script>
  import { Mail, RefreshCw, X } from '@lucide/svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';

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
      resendError = err.message || t('notifications.failedToSendVerification');
    } finally {
      resending = false;
    }
  }

  function handleDismiss() {
    ondismiss?.();
  }
</script>

{#if show}
  <div class="bg-ds-warning-subtle border-b border-ds-status-warning-border">
    <div class="max-w-7xl mx-auto py-3 px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between flex-wrap gap-2">
        <div class="flex items-center gap-3">
          <div class="flex-shrink-0">
            <Mail class="h-5 w-5 text-ds-icon-warning" />
          </div>
          <div class="text-sm text-ds-text-warning">
            <p class="font-medium">
              {t('notifications.verifyEmail')}
            </p>
            <p class="text-ds-text-warning opacity-90">
              {t('notifications.verifyEmailDescription')}
            </p>
          </div>
        </div>
        <div class="flex items-center gap-3">
          {#if resendSuccess}
            <span class="text-sm" style="color: var(--ds-text-success);">{t('notifications.verificationEmailSent')}</span>
          {:else if resendError}
            <span class="text-sm" style="color: var(--ds-text-danger);">{resendError}</span>
          {/if}
          <button
            onclick={handleResend}
            disabled={resending}
            class="inline-flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-ds-text-warning bg-ds-accent-yellow-subtle rounded-md hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-ds-warning disabled:opacity-50"
          >
            {#if resending}
              <RefreshCw class="h-4 w-4 animate-spin" />
              {t('notifications.sending')}
            {:else}
              <RefreshCw class="h-4 w-4" />
              {t('notifications.resendEmail')}
            {/if}
          </button>
          <button
            onclick={handleDismiss}
            class="p-1.5 text-ds-icon-warning hover:opacity-80 hover:bg-ds-accent-yellow-subtle rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-ds-warning"
            title={t('notifications.dismiss')}
          >
            <X class="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
