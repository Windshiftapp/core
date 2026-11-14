<script>
  import { onDestroy } from 'svelte';
  import Globe from '@lucide/svelte/icons/globe';
  import { updateUserRegionalSettings, userPreferences } from '../api/users.js';
  import { collectionPreferences } from '../stores/collectionPreferences.svelte.js';
  import AlertBox from '../components/AlertBox.svelte';
  import Button from '../components/Button.svelte';
  import FormField from '../components/FormField.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import TimezonePicker from '../pickers/TimezonePicker.svelte';
  import { authStore } from '../stores/auth.svelte.js';
  import { i18n, SUPPORTED_LOCALES, t } from '../stores/i18n.svelte.js';
  import { getCalendarPreference, setCalendarPreference } from '../stores/calendar.svelte.js';
  import { getWeekStartPreference, setWeekStartPreference, WEEKDAYS } from '../stores/calendar.svelte.js';

  let { user = $bindable(null), userId = null } = $props();

  let selectedTimezone = $state('UTC');
  let selectedLanguage = $state('en');
  let selectedCalendar = $state('auto');
  let selectedWeekStart = $state('auto');
  let calendarHydrated = null;
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

  $effect(() => {
    if (user?.id && collectionPreferences.loaded && calendarHydrated !== user.id) {
      selectedCalendar = getCalendarPreference();
      selectedWeekStart = getWeekStartPreference();
      calendarHydrated = user.id;
    }
  });
  onDestroy(() => {
    if (savedTimer) clearTimeout(savedTimer);
  });

  async function save() {
    if (!userId || !user) return;

    saving = true;
    saved = false;
    error = '';
    const savingUserId = userId;
    try {
      const updatedUser = await updateUserRegionalSettings(userId, {
        timezone: selectedTimezone,
        language: selectedLanguage,
      });
      if (authStore.currentUser?.id !== savingUserId) return;
      const savedLanguage = updatedUser?.language || selectedLanguage;
      authStore.patchCurrentUser({
        language: savedLanguage,
        timezone: updatedUser?.timezone || selectedTimezone,
      });
      if (collectionPreferences.loaded && (selectedCalendar !== getCalendarPreference() || selectedWeekStart !== getWeekStartPreference())) {
        try {
          const prefs = await userPreferences.update({ calendar: selectedCalendar, week_start: selectedWeekStart });
          if (authStore.currentUser?.id !== savingUserId) return;
          setCalendarPreference(prefs.calendar);
          setWeekStartPreference(prefs.week_start);
        } catch (err) {
          if (authStore.currentUser?.id !== savingUserId) return;
          await i18n.setLocale(savedLanguage);
          if (authStore.currentUser?.id !== savingUserId) return;
          user = updatedUser || { ...user, timezone: selectedTimezone, language: savedLanguage };
          throw new Error(t('users.calendarPreferencesSaveFailed'));
        }
      }
      await i18n.setLocale(savedLanguage);
      if (authStore.currentUser?.id !== savingUserId) return;
      user = updatedUser || { ...user, timezone: selectedTimezone, language: savedLanguage };
      saved = true;
      if (savedTimer) clearTimeout(savedTimer);
      savedTimer = setTimeout(() => (saved = false), 3000);
    } catch (err) {
      if (authStore.currentUser?.id !== savingUserId) return;
      error = err.message || t('dialogs.alerts.failedToSave', { error: 'regional settings' });
    } finally {
      saving = false;
    }
  }

  const languages = SUPPORTED_LOCALES.map((locale) => ({ value: locale.code, label: locale.name }));
  const calendars = $derived([
    { value: 'auto', label: t('users.calendarAutomatic') },
    { value: 'gregory', label: t('users.gregorian') },
    { value: 'persian', label: t('users.persian') },
  ]);
  const weekStarts = $derived([
    { value: 'auto', label: t('users.calendarAutomatic') },
    ...WEEKDAYS.map((day, index) => ({
      value: day,
      label: new Intl.DateTimeFormat(selectedLanguage, { weekday: 'long', timeZone: 'UTC' }).format(new Date(Date.UTC(2025, 0, 5 + index))),
    })),
  ]);


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

{#if collectionPreferences.loadError}
  <AlertBox message={t('dialogs.alerts.failedToLoad', { error: t('users.calendar') })} />
  <Button variant="secondary" onclick={() => collectionPreferences.ensureLoaded(userId)}>
    {t('common.retry')}
  </Button>
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

  <FormField label={t('users.calendar')} id="calendar" helper={t('users.calendarHint')}>
    <BasePicker
      id="calendar"
      bind:value={selectedCalendar}
      items={calendars}
      placeholder={t('users.calendar')}
      disabled={!user || saving || !collectionPreferences.loaded}
      getValue={(item) => item.value}
      getLabel={(item) => item.label}
    />
  </FormField>
  <FormField label={t('users.weekStart')} id="week-start" helper={t('users.weekStartHint')}>
    <BasePicker
      id="week-start"
      inputTestid="regional-week-start"
      optionTestid={(item) => `regional-week-start-${item.value}`}
      menuTestid="regional-week-start-menu"
      bind:value={selectedWeekStart}
      items={weekStarts}
      disabled={!user || saving || !collectionPreferences.loaded}
      getValue={(item) => item.value}
      getLabel={(item) => item.label}
    />
  </FormField>
</div>

<div class="mt-6 flex items-center gap-4">
  <Button variant="primary" onclick={save} disabled={!user || saving} size="medium" dataTestid="regional-settings-save">
    {saving ? t('common.saving') : t('users.saveSettings')}
  </Button>
  {#if saved}
    <AlertBox variant="success" message={t('users.settingsSaved')} />
  {/if}
</div>
