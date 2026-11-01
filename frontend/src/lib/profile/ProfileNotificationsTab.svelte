<script>
  import Bell from '@lucide/svelte/icons/bell';
  import AlertBox from '../components/AlertBox.svelte';
  import Button from '../components/Button.svelte';
  import FormField from '../components/FormField.svelte';
  import Input from '../components/Input.svelte';
  import Toggle from '../components/Toggle.svelte';
  import { userPreferences } from '../api/users.js';
  import { t } from '../stores/i18n.svelte.js';

  let loading = $state(true);
  let saving = $state(false);
  let saved = $state(false);
  let error = $state('');
  let enabled = $state(false);
  let days = $state(3);
  let savedTimer = null;

  $effect(() => {
    void load();
    return () => {
      if (savedTimer) clearTimeout(savedTimer);
    };
  });

  async function load() {
    try {
      const prefs = await userPreferences.get();
      enabled = prefs?.due_date_reminders_enabled === true;
      const configured = Number(prefs?.due_date_reminder_days);
      days = Number.isFinite(configured) && configured >= 1 && configured <= 30 ? configured : 3;
    } catch (err) {
      error = err?.message || String(err);
    } finally {
      loading = false;
    }
  }

  async function save() {
    saving = true;
    saved = false;
    error = '';
    try {
      const clamped = Math.min(30, Math.max(1, Number(days) || 3));
      days = clamped;
      const prefs = await userPreferences.update({
        due_date_reminders_enabled: enabled,
        due_date_reminder_days: clamped
      });
      enabled = prefs?.due_date_reminders_enabled === true;
      saved = true;
      if (savedTimer) clearTimeout(savedTimer);
      savedTimer = setTimeout(() => (saved = false), 3000);
    } catch (err) {
      error = err?.message || String(err);
    } finally {
      saving = false;
    }
  }
</script>

<div class="mb-6">
  <h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
    <Bell class="h-5 w-5" style="color: var(--ds-text-subtle);" />
    {t('users.dueDateReminders')}
  </h2>
  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.dueDateRemindersDesc')}</p>
</div>

{#if error}
  <AlertBox message={error} />
{/if}

<div class="space-y-4" data-testid="profile-due-date-reminders">
  <div class="flex items-center justify-between">
    <div>
      <div class="text-sm font-medium mb-1" style="color: var(--ds-text);">
        {t('users.dueDateRemindersEnable')}
      </div>
      <p class="text-xs" style="color: var(--ds-text-subtle);">{t('users.dueDateRemindersEnableHint')}</p>
    </div>
    <Toggle bind:checked={enabled} disabled={loading} dataTestid="due-date-reminders-toggle" />
  </div>

  {#if enabled}
    <div class="max-w-xs">
      <FormField label={t('users.dueDateReminderDays')} id="due-date-reminder-days" helper={t('users.dueDateReminderDaysHint')}>
        <Input
          id="due-date-reminder-days"
          type="number"
          min="1"
          max="30"
          bind:value={days}
          disabled={loading || saving}
          dataTestid="due-date-reminder-days"
        />
      </FormField>
    </div>
  {/if}
</div>

<div class="mt-6 flex items-center gap-4">
  <Button variant="primary" onclick={save} disabled={loading || saving} size="medium" dataTestid="due-date-reminders-save">
    {saving ? t('common.saving') : t('users.saveSettings')}
  </Button>
  {#if saved}
    <AlertBox variant="success" message={t('users.settingsSaved')} />
  {/if}
</div>
