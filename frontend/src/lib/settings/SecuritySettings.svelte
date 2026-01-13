<script>
  import { onMount } from 'svelte';
  import { Shield, Calendar, Loader2, Terminal } from 'lucide-svelte';
  import { getSecuritySettings, updateSecuritySettings } from '../api.js';
  import Toggle from '../components/Toggle.svelte';

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
      error = 'Failed to load security settings';
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
      error = 'Failed to save settings';
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
      <h2 class="text-2xl font-semibold" style="color: var(--ds-text);">Security</h2>
    </div>
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      Manage security-related settings and feature controls
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
              <h3 class="text-base font-medium" style="color: var(--ds-text);">Calendar Feed Subscriptions</h3>
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                Allow users to generate ICS feed URLs for external calendar apps
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
              <strong>Warning:</strong> When disabled, existing calendar feeds will stop working immediately.
              Users will not be able to generate new feed URLs.
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
              <h3 class="text-base font-medium" style="color: var(--ds-text);">Plugin CLI Command Execution</h3>
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                Allow plugins to execute shell commands on the server
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
              <strong>Warning:</strong> Enabling this setting allows plugins to execute shell commands.
              Commands are restricted to each plugin's own directory for security.
              Only enable if you trust all installed plugins.
            </div>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>
