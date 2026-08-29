<script>
  import { api } from '../api.js';
  import { Settings, Workflow, Monitor, Bell } from '@lucide/svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId } = $props();

  let configurationSet = $state(null);
  let isUsingDefault = $state(false);
  let loading = $state(true);

  function isSystemDefaultConfiguration(configSet) {
    return Boolean(configSet?.builtin_key);
  }

  function getConfigurationDisplayValue(field) {
    return isSystemDefaultConfiguration(configurationSet)
      ? t(`settings.configSets.defaults.configuration.${field}`)
      : (configurationSet?.[field] || '');
  }

  function getSystemReferenceName(value, builtinKey, type) {
    if (!value) return t('settings.configSets.noneAssigned');
    if (!builtinKey) return value;
    if (type === 'workflow') return t('workflows.defaults.default.name');
    if (type === 'screen') return t('screensPage.defaults.default.name');
    if (type === 'notifications') return t('settings.configSets.defaults.notifications.name');
    return value;
  }

  async function loadConfigurationSet() {
    try {
      loading = true;

      // Load all configuration sets and find the one assigned to this workspace
      const response = await api.configurationSets.getAll();
      if (response?.configuration_sets) {
        // First, try to find workspace-assigned config set
        configurationSet = response.configuration_sets.find(cs =>
          cs.workspace_ids && cs.workspace_ids.includes(parseInt(workspaceId))
        );

        // If not found, use the default configuration set
        if (!configurationSet) {
          configurationSet = response.configuration_sets.find(cs => cs.is_default);
          isUsingDefault = true;
        } else {
          isUsingDefault = false;
        }
      }
    } catch (error) {
      console.error('Failed to load configuration set:', error);
    } finally {
      loading = false;
    }
  }

  // Refresh when workspace changes
  $effect(() => {
    if (workspaceId) {
      loadConfigurationSet();
    }
  });
</script>

{#if loading}
  <div class="animate-pulse space-y-3">
    <div class="h-4 rounded w-1/4" style="background-color: var(--ds-background-neutral);"></div>
    <div class="h-3 rounded w-3/4" style="background-color: var(--ds-background-neutral);"></div>
    <div class="h-3 rounded w-1/2" style="background-color: var(--ds-background-neutral);"></div>
  </div>
{:else if !configurationSet}
  <div class="flex items-center gap-3" style="color: var(--ds-text-subtle);">
    <Settings class="w-5 h-5" />
    <div>
      <div class="font-medium" style="color: var(--ds-text);">{t('settings.configSets.previewUnavailable')}</div>
      <div class="text-sm">{t('settings.configSets.previewUnavailableDescription')}</div>
    </div>
  </div>
{:else}
  <div class="space-y-4">
    <!-- Configuration Set Info -->
    <div class="flex items-center gap-3">
      <Settings class="w-5 h-5" style="color: var(--ds-icon-accent);" />
      <div>
        <div class="font-medium flex items-center gap-2" style="color: var(--ds-text);">
          {getConfigurationDisplayValue('name')}
          {#if isUsingDefault}
            <Lozenge color="blue" text={t('common.default')} />
          {/if}
        </div>
        {#if getConfigurationDisplayValue('description')}
          <div class="text-sm" style="color: var(--ds-text-subtle);">{getConfigurationDisplayValue('description')}</div>
        {/if}
      </div>
    </div>

    <!-- Configuration Details -->
    <div class="space-y-4 pt-3 border-t" style="border-color: var(--ds-border);">
      <!-- Workflow -->
      <div class="flex items-center gap-2">
        <Workflow class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
        <div class="text-sm">
          <div style="color: var(--ds-text-subtle);">{t('settings.configSets.workflow')}</div>
          <div class="font-medium" style="color: var(--ds-text);">
            {getSystemReferenceName(configurationSet.workflow_name, configurationSet.workflow_builtin_key, 'workflow')}
          </div>
        </div>
      </div>

      <!-- Screens -->
      <div class="flex items-start gap-2">
        <Monitor class="w-4 h-4 mt-0.5" style="color: var(--ds-icon-subtle);" />
        <div class="text-sm">
          <div class="mb-1" style="color: var(--ds-text-subtle);">{t('settings.configSets.screens')}</div>
          <div class="space-y-1">
            <div class="flex justify-between">
              <span style="color: var(--ds-text-subtle);">{t('settings.configSets.createScreen')}</span>
              <span class="font-medium" style="color: var(--ds-text);">{getSystemReferenceName(configurationSet.create_screen_name, configurationSet.create_screen_builtin_key, 'screen')}</span>
            </div>
            <div class="flex justify-between">
              <span style="color: var(--ds-text-subtle);">{t('settings.configSets.editScreen')}</span>
              <span class="font-medium" style="color: var(--ds-text);">{getSystemReferenceName(configurationSet.edit_screen_name, configurationSet.edit_screen_builtin_key, 'screen')}</span>
            </div>
            <div class="flex justify-between">
              <span style="color: var(--ds-text-subtle);">{t('settings.configSets.viewScreen')}</span>
              <span class="font-medium" style="color: var(--ds-text);">{getSystemReferenceName(configurationSet.view_screen_name, configurationSet.view_screen_builtin_key, 'screen')}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Notifications -->
      <div class="flex items-center gap-2">
        <Bell class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
        <div class="text-sm">
          <div style="color: var(--ds-text-subtle);">{t('settings.configSets.notifications')}</div>
          <div class="font-medium" style="color: var(--ds-text);">
            {getSystemReferenceName(configurationSet.notification_setting_name, configurationSet.notification_setting_builtin_key, 'notifications')}
          </div>
        </div>
      </div>
    </div>
  </div>
{/if}
