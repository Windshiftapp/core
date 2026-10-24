<script>
  import { onDestroy } from 'svelte';
  import Globe from '@lucide/svelte/icons/globe';
  import { updateUserRegionalSettings } from '../api/users.js';
  import AlertBox from '../components/AlertBox.svelte';
  import Button from '../components/Button.svelte';
  import FormField from '../components/FormField.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import TimezonePicker from '../pickers/TimezonePicker.svelte';
  import { authStore } from '../stores/auth.svelte.js';
  import { i18n, SUPPORTED_LOCALES, t } from '../stores/i18n.svelte.js';

  let { user = $bindable(null), userId = null } = $props();

  let selectedTimezone = $state('UTC');
  let selectedLanguage = $state('en');
  let saving = $state(false);
  let saved = $state(false);
  let error = $state('');
  let hydratedSettings = '';
  let savedTimer = null;

  $effect(() => {
    if (!user) return;
    const settings = `${user.id}:${user.timezone || 'UTC'}:${user.language || 'en'}`;
    if (settings === hydratedSettings) return;
    hydratedSettings = settings;
    selectedTimezone = user.timezone || 'UTC';
    selectedLanguage = user.language || 'en';
  });

  onDestroy(() => {
    if (savedTimer) clearTimeout(savedTimer);
  });

  async function save() {
    if (!userId || !user) return;

    saving = true;
    saved = false;
    error = '';
    try {
      const updatedUser = await updateUserRegionalSettings(userId, {
        timezone: selectedTimezone,
        language: selectedLanguage,
      });
      const savedLanguage = updatedUser?.language || selectedLanguage;
      authStore.patchCurrentUser({
        language: savedLanguage,
        timezone: updatedUser?.timezone || selectedTimezone,
      });
      await i18n.setLocale(savedLanguage);
      user = updatedUser || { ...user, timezone: selectedTimezone, language: savedLanguage };
      saved = true;
      if (savedTimer) clearTimeout(savedTimer);
      savedTimer = setTimeout(() => (saved = false), 3000);
    } catch (err) {
      error = err.message || t('dialogs.alerts.failedToSave', { error: 'regional settings' });
    } finally {
      saving = false;
    }
  }

  const languages = SUPPORTED_LOCALES.map((locale) => ({ value: locale.code, label: locale.name }));
</script>

<div class="mb-6">
  <h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
    <Globe class="h-5 w-5" style="color: var(--ds-text-subtle);" />
    {t('users.regionalSettings')}
  </h2>
  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.regionalSettingsDesc')}</p>
</div>

{#if error}
  <AlertBox message={error} />
{/if}

<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
  <FormField label={t('users.timezone')} id="timezone" helper={t('users.timezoneHint')}>
    <TimezonePicker
      id="timezone"
      bind:value={selectedTimezone}
      placeholder={t('users.timezone')}
      disabled={!user || saving}
    />
  </FormField>

  <FormField label={t('users.language')} id="language" helper={t('users.languageHint')}>
    <BasePicker
      id="language"
      bind:value={selectedLanguage}
      items={languages}
      placeholder={t('users.language')}
      disabled={!user || saving}
      getValue={(item) => item.value}
      getLabel={(item) => item.label}
    />
  </FormField>
</div>

<div class="mt-6 flex items-center gap-4">
  <Button variant="primary" onclick={save} disabled={!user || saving} size="medium">
    {saving ? t('common.saving') : t('users.saveSettings')}
  </Button>
  {#if saved}
    <AlertBox variant="success" message={t('users.settingsSaved')} />
  {/if}
</div>
