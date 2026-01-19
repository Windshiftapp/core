<script>
  import { Mail } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Props
  let { email = '', onRequestAnother } = $props();
</script>

<div class="text-center p-8">
  <div class="w-16 h-16 mx-auto mb-6 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-success-subtle);">
    <Mail class="w-8 h-8" style="color: var(--ds-icon-success);" />
  </div>

  <h2 class="text-2xl font-bold mb-3" style="color: var(--ds-text);">
    {t('portal.checkYourEmail') || 'Check your email'}
  </h2>

  <p class="mb-2" style="color: var(--ds-text-subtle);">
    {t('portal.magicLinkSentTo') || "We've sent a sign-in link to"}
  </p>

  {#if email}
    <p class="font-medium mb-4" style="color: var(--ds-text);">
      {email}
    </p>
  {/if}

  <p class="text-sm mb-6" style="color: var(--ds-text-subtlest);">
    {t('portal.clickLinkToSignIn') || 'Click the link in the email to sign in. The link expires in 15 minutes.'}
  </p>

  <div class="space-y-3">
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      {t('portal.didntReceiveEmail') || "Didn't receive the email?"}
    </p>

    <ul class="text-sm text-left max-w-xs mx-auto space-y-1" style="color: var(--ds-text-subtlest);">
      <li>{t('portal.checkSpam') || '- Check your spam or junk folder'}</li>
      <li>{t('portal.waitFewMinutes') || '- Wait a few minutes and refresh'}</li>
      <li>{t('portal.checkEmailCorrect') || '- Make sure you entered the correct email'}</li>
    </ul>

    {#if onRequestAnother}
      <button
        onclick={onRequestAnother}
        class="mt-4 text-sm font-medium transition-colors"
        style="color: var(--ds-link);"
      >
        {t('portal.tryAnotherEmail') || 'Try with a different email'}
      </button>
    {/if}
  </div>
</div>
