<script>
  import { onMount } from 'svelte';
  import { Shield, Calendar, Loader2, Terminal } from 'lucide-svelte';
  import { getSecuritySettings, updateSecuritySettings } from '../api.js';
  import Toggle from '../components/Toggle.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');

  let calendarFeedEnabled = $state(true);
  let pluginCliExecEnabled = $state(false);

  onMount(async () => {
    await loadSettings();
  });

  async function loadSettings() {
    loading = true;
    error = '';
    try {
      const settings = await getSecuritySettings();
      calendarFeedEnabled = settings.calendar_feed_enabled ?? true;
      pluginCliExecEnabled = settings.plugin_cli_exec_enabled ?? false;
    } catch (err) {
      error = t('settings.security.failedToLoad');
      console.error('Failed to load security settings:', err);
    } finally {
      loading = false;
    }
  }

  async function saveSettings() {
    saving = true;
    error = '';
    try {
      await updateSecuritySettings({
        calendar_feed_enabled: calendarFeedEnabled,
        plugin_cli_exec_enabled: pluginCliExecEnabled
      });
    } catch (err) {
      error = t('settings.security.failedToSave');
      console.error('Failed to save settings:', err);
    } finally {
      saving = false;
    }
  }

  async function handleCalendarToggle(newValue) {
    calendarFeedEnabled = newValue;
    await saveSettings();
  }

  async function handleCliExecToggle(newValue) {
    pluginCliExecEnabled = newValue;
    await saveSettings();
  }
</script>

<div>
  <div class="mb-6">
    <div class="flex items-center gap-3 mb-2">
      <Shield class="w-6 h-6" style="color: var(--ds-icon);" />
      <h2 class="text-2xl font-semibold" style="color: var(--ds-text);">{t('settings.security.title')}</h2>
    </div>
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      {t('settings.security.subtitle')}
    </p>
  </div>

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-icon-subtle);" />
    </div>
  {:else}
    {#if error}
      <div class="mb-4 p-3 rounded-md" style="background-color: var(--ds-background-danger-bold); color: white;">
        {error}
      </div>
    {/if}

    <!-- Calendar Feed Settings -->
    <div class="border rounded-lg p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <div class="flex items-start gap-4">
        <div class="p-2 rounded-lg" style="background-color: var(--ds-background-neutral);">
          <Calendar class="w-5 h-5" style="color: var(--ds-icon);" />
        </div>
        <div class="flex-1">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-base font-medium" style="color: var(--ds-text);">{t('settings.security.calendarFeeds')}</h3>
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                {t('settings.security.calendarFeedsDesc')}
              </p>
            </div>
            <Toggle
              bind:checked={calendarFeedEnabled}
              disabled={saving}
              onchange={handleCalendarToggle}
            />
          </div>

          {#if !calendarFeedEnabled}
            <div class="mt-3 p-3 rounded-md text-sm" style="background-color: var(--ds-background-warning-bold); color: var(--ds-text-warning-inverse);">
              {t('settings.security.calendarFeedsWarning')}
            </div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Plugin CLI Execution Settings -->
    <div class="border rounded-lg p-6 mt-4" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <div class="flex items-start gap-4">
        <div class="p-2 rounded-lg" style="background-color: var(--ds-background-neutral);">
          <Terminal class="w-5 h-5" style="color: var(--ds-icon);" />
        </div>
        <div class="flex-1">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-base font-medium" style="color: var(--ds-text);">{t('settings.security.pluginExecution')}</h3>
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                {t('settings.security.pluginExecutionDesc')}
              </p>
            </div>
            <Toggle
              bind:checked={pluginCliExecEnabled}
              disabled={saving}
              onchange={handleCliExecToggle}
            />
          </div>

          {#if pluginCliExecEnabled}
            <div class="mt-3 p-3 rounded-md text-sm border" style="background-color: var(--ds-status-danger-bg); color: var(--ds-status-danger-text); border-color: var(--ds-status-danger-border);">
              {t('settings.security.pluginExecutionWarning')}
            </div>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>
