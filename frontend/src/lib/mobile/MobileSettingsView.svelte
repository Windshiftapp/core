<script>
  import { ChevronRight } from '@lucide/svelte';
  import { api } from '../api.js';
  import { authStore } from '../stores';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { i18n, SUPPORTED_LOCALES, t } from '../stores/i18n.svelte.js';
  import { navigate } from '../router.js';
  import MobileHeader from './MobileHeader.svelte';
  import MobileOptionSheet from './MobileOptionSheet.svelte';

  let languageSheetOpen = $state(false);
  let saving = $state(false);

  // Native locale names on purpose: users must be able to find their language
  // even while the UI is in one they cannot read (same as the desktop picker).
  const languages = SUPPORTED_LOCALES.map((locale) => ({ value: locale.code, label: locale.name }));
  const currentLanguageName = $derived(
    SUPPORTED_LOCALES.find((locale) => locale.code === i18n.locale)?.name ?? i18n.locale,
  );

  function back() {
    if (window.history.length > 1) window.history.back();
    else navigate('/m');
  }

  // Same save path as the desktop regional settings tab: persist to the user
  // profile, mirror into authStore (App.svelte re-syncs the locale from it on
  // boot), then switch the reactive runtime. Guest sessions (no user id) still
  // switch the runtime; localStorage keeps that choice.
  async function selectLanguage(locale) {
    if (!locale || locale.value === i18n.locale || saving) return;
    const user = authStore.currentUser;
    saving = true;
    try {
      if (user?.id) {
        const updated = await api.updateUserRegionalSettings(user.id, {
          timezone: user.timezone || 'UTC',
          language: locale.value,
        });
        authStore.patchCurrentUser({
          language: updated?.language || locale.value,
          timezone: updated?.timezone || user.timezone || 'UTC',
        });
      }
      await i18n.setLocale(locale.value);
    } catch (err) {
      console.error('Failed to save language:', err);
      errorToast(t('mobile.settings.saveFailed'));
    } finally {
      saving = false;
    }
  }
</script>

<MobileHeader title={t('mobile.settings.title')} onback={back} />

<div class="content">
  <section class="group" data-testid="mobile-settings-regional">
    <h2 class="group-title">{t('users.regionalSettings')}</h2>
    <button
      class="row"
      onclick={() => (languageSheetOpen = true)}
      disabled={saving}
      data-testid="mobile-settings-language"
      type="button"
    >
      <span class="row-label">{t('users.language')}</span>
      <span class="row-value" data-testid="mobile-settings-language-current">{currentLanguageName}</span>
      <ChevronRight size={16} class="row-chev" />
    </button>
  </section>
</div>

<MobileOptionSheet
  bind:isOpen={languageSheetOpen}
  title={t('users.language')}
  options={languages}
  getValue={(l) => l.value}
  getLabel={(l) => l.label}
  selectedValue={i18n.locale}
  onSelect={selectLanguage}
  dataTestid="language-sheet"
/>

<style>
  .content {
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  .group-title {
    margin: 0 0 0.5rem;
    font-size: 0.8125rem;
    font-weight: var(--font-semibold, 600);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--ds-text-subtle);
  }

  .row {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.875rem 1rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.75rem;
    background: var(--ds-surface);
    color: var(--ds-text);
    font-size: 0.9375rem;
    text-align: left;
    cursor: pointer;
  }

  .row:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .row-label {
    flex: 1;
  }

  .row-value {
    color: var(--ds-text-subtle);
  }

  .row :global(svg) {
    color: var(--ds-text-subtle);
    flex-shrink: 0;
  }
</style>
