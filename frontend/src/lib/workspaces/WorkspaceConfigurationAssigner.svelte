<script>
  import { onMount } from 'svelte';
  import { configurationSets as configurationSetsApi } from '../api/configuration.js';
  import { t } from '../stores/i18n.svelte.js';
  import { isSystemAdmin } from '../stores/permissions.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Label from '../components/Label.svelte';
  import Card from '../components/Card.svelte';
  import ConfigurationSetPicker from '../pickers/ConfigurationSetPicker.svelte';
  import MigrationAssistant from '../pages/MigrationAssistant.svelte';

  let { workspaceId, onconfigurationChanged = null } = $props();
  
  let configurationSets = $state([]);
  let selectedConfigurationSetId = $state(null);
  let loading = $state(true);
  let saving = $state(false);

  // Migration assistant state
  let showMigrationAssistant = $state(false);
  let migrationConfigSet = $state(null);
  let pendingConfigurationChange = $state(null);

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      loading = true;

      // Load all configuration sets
      const response = await configurationSetsApi.getAll();
      // Filter out Personal Tasks Configuration (system-managed) and the default config set
      // (since selecting "Default Configuration" option uses it automatically)
      configurationSets = (response?.configuration_sets || []).filter(cs =>
        cs.name !== 'Personal Tasks Configuration' && !cs.is_default
      );

      // Find the currently assigned configuration set for this workspace
      const assignedConfigSet = configurationSets.find(cs =>
        cs.workspace_ids && cs.workspace_ids.includes(parseInt(workspaceId))
      );
      selectedConfigurationSetId = assignedConfigSet ? assignedConfigSet.id : null;

    } catch (error) {
      console.error('Failed to load configuration sets:', error);
      configurationSets = [];
      selectedConfigurationSetId = null;
    } finally {
      loading = false;
    }
  }

  async function updateConfigurationSet(newConfigSetId) {
    try {
      saving = true;
      
      // Get the currently assigned configuration set to check for workflow changes
      const currentConfigSet = configurationSets.find(cs => 
        cs.workspace_ids && cs.workspace_ids.includes(parseInt(workspaceId))
      );
      const newConfigSet = configurationSets.find(cs => cs.id === newConfigSetId);
      
      // Check if we're assigning a different config set
      const configSetChanging = newConfigSet &&
        (!currentConfigSet || currentConfigSet.id !== newConfigSet.id);

      // If config set is changing, check if comprehensive migration is required
      if (configSetChanging) {

        try {
          // Analyze comprehensive migration requirements (item types, statuses, custom fields, priorities)
          const migrationAnalysis = await configurationSetsApi.analyzeComprehensiveMigration(newConfigSet.id, parseInt(workspaceId));
          
          if (migrationAnalysis.requires_migration) {
            // Migrations mutate shared configuration state and are executed
            // through system-admin-only endpoints (WI-1359). Non-admins get a
            // clear refusal instead of a wizard that cannot succeed.
            if (!$isSystemAdmin) {
              errorToast(t('settings.configSets.migrationRequiresSystemAdmin'));
              return;
            }
            // Store the pending configuration change
            pendingConfigurationChange = {
              currentConfigSet,
              newConfigSet,
              newConfigSetId
            };
            migrationConfigSet = {
              ...newConfigSet,
              workspace_ids: [parseInt(workspaceId)] // Only this workspace needs migration
            };
            showMigrationAssistant = true;
            return; // Don't apply configuration change yet - wait for migration completion
          } else {
            // No migration needed, apply configuration change immediately
            await applyConfigurationChange(newConfigSetId, currentConfigSet, newConfigSet);
            return;
          }
        } catch (error) {
          console.error('Failed to analyze migration requirements:', error);
          // If analysis fails, show migration assistant as a fallback
          pendingConfigurationChange = {
            currentConfigSet,
            newConfigSet,
            newConfigSetId
          };
          migrationConfigSet = {
            ...newConfigSet,
            workspace_ids: [parseInt(workspaceId)]
          };
          showMigrationAssistant = true;
          return;
        }
      }
      
      // No migration needed - apply configuration change immediately
      await applyConfigurationChange(newConfigSetId, currentConfigSet, newConfigSet);
      
    } catch (error) {
      console.error('Failed to update configuration set:', error);
      errorToast(t('dialogs.alerts.failedToUpdate', { error: error.message || error }));
    } finally {
      saving = false;
    }
  }

  // Separate function to apply the actual configuration change. The server
  // endpoint swaps the workspace-assignment join rows atomically; a required
  // data migration surfaces as a 409 before anything is written.
  async function applyConfigurationChange(newConfigSetId, currentConfigSet, newConfigSet) {
    await configurationSetsApi.assignToWorkspace(parseInt(workspaceId), newConfigSetId);

    await loadData(); // Reload to refresh the data
    
    // Notify parent component about the change
    onconfigurationChanged?.({
      oldConfigSet: currentConfigSet,
      newConfigSet: newConfigSet
    });
  }

  function getScreenName(screenId, context) {
    if (!screenId) return t('settings.configSets.none');
    // The screen names are already loaded in the configuration set data
    return t('settings.configSets.configured'); // We could enhance this to show actual screen names
  }

  async function handleMigrationAssistantClose(data) {
    const { success, cancelled } = data || {};
    
    try {
      if (success && pendingConfigurationChange) {
        // Migration was successful - apply the configuration change
        saving = true;
        await applyConfigurationChange(
          pendingConfigurationChange.newConfigSetId,
          pendingConfigurationChange.currentConfigSet,
          pendingConfigurationChange.newConfigSet
        );
      } else if (cancelled || !success) {
        // Migration was cancelled or failed - revert the UI selection
        const currentConfigSet = configurationSets.find(cs => 
          cs.workspace_ids && cs.workspace_ids.includes(parseInt(workspaceId))
        );
        selectedConfigurationSetId = currentConfigSet ? currentConfigSet.id : null;
      }
    } catch (error) {
      console.error('Failed to apply configuration change after migration:', error);
      errorToast(t('dialogs.alerts.failedToApplyConfig', { error: error.message || error }));
      
      // Revert the UI selection on error
      const currentConfigSet = configurationSets.find(cs => 
        cs.workspace_ids && cs.workspace_ids.includes(parseInt(workspaceId))
      );
      selectedConfigurationSetId = currentConfigSet ? currentConfigSet.id : null;
    } finally {
      // Clean up migration assistant state
      showMigrationAssistant = false;
      migrationConfigSet = null;
      pendingConfigurationChange = null;
      saving = false;
    }
  }
</script>

<div class="space-y-6">
  <p class="text-sm" style="color: var(--ds-text-subtle);">
    {t('settings.configSets.assignerDescription')}
  </p>

  {#if loading}
    <Card rounded="xl" shadow padding="loose" class="text-center">
      <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('settings.configSets.loading')}</div>
    </Card>
  {:else}
    <Card rounded="xl" shadow padding="spacious">
      <div class="space-y-6">
        <!-- Configuration Set Selection -->
        <div>
          <Label color="default" class="mb-3">{t('settings.configSets.select')}</Label>
          <ConfigurationSetPicker
            bind:value={selectedConfigurationSetId}
            items={configurationSets}
            disabled={saving}
            onSelect={(configSet) => updateConfigurationSet(configSet?.id ?? null)}
          />
        </div>


        <!-- Status indicator while saving -->
        {#if saving}
          <div class="text-center py-2">
            <div class="text-sm" style="color: var(--ds-text-subtle);">{t('settings.configSets.updating')}</div>
          </div>
        {/if}
      </div>
    </Card>

  {/if}
</div>

<!-- Migration Assistant -->
<MigrationAssistant
  configurationSet={migrationConfigSet}
  targetConfigurationSet={migrationConfigSet}
  isVisible={showMigrationAssistant}
  workspaceId={parseInt(workspaceId)}
  comprehensive={true}
  onclose={handleMigrationAssistantClose}
/>
