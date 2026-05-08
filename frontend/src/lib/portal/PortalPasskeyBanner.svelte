<script>
  import { KeyRound, X } from 'lucide-svelte';
  import { portalStore } from '../stores/portal.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { navigate } from '../router.js';

  let slug = $derived(portalStore.currentSlug);

  function goSetup() {
    if (slug) navigate(`/portal/${slug}/profile`);
  }

  async function dismiss() {
    if (!slug) return;
    await portalAuthStore.dismissPasskeyPrompt(slug);
  }
</script>

{#if $portalAuthStore.showPasskeyBanner}
  <div
    class="max-w-2xl mx-auto rounded-lg border p-4 mb-6 flex items-start gap-3"
    style="background-color: var(--ds-info-subtle, #eff6ff); border-color: var(--ds-info, #93c5fd);"
    data-testid="portal-passkey-banner"
  >
    <div
      class="rounded-full p-2 flex-shrink-0"
      style="background-color: var(--ds-info, #3b82f6); color: white;"
    >
      <KeyRound class="w-4 h-4" />
    </div>
    <div class="flex-1 min-w-0">
      <div class="text-sm font-medium" style="color: var(--ds-text);">
        {t('portal.passkeyBannerTitle') || 'Skip the email next time'}
      </div>
      <div class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">
        {t('portal.passkeyBannerBody') ||
          'Set up a passkey for faster, phishing-resistant sign-in.'}
      </div>
      <div class="mt-3 flex items-center gap-3">
        <button
          type="button"
          class="text-sm font-medium hover:underline"
          style="color: var(--ds-text-link);"
          onclick={goSetup}
        >
          {t('portal.passkeyBannerCta') || 'Set up passkey'}
        </button>
      </div>
    </div>
    <button
      type="button"
      class="p-1 rounded hover:bg-black/5 transition-colors flex-shrink-0"
      onclick={dismiss}
      aria-label={t('common.dismiss') || 'Dismiss'}
    >
      <X class="w-4 h-4" style="color: var(--ds-text-subtle);" />
    </button>
  </div>
{/if}
