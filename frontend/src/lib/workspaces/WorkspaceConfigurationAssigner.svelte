<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { api } from '../api.js';
  import { Settings } from 'lucide-svelte';
  import Label from '../components/Label.svelte';
  import Card from '../components/Card.svelte';
  import ConfigurationSetPicker from '../pickers/ConfigurationSetPicker.svelte';
  import MigrationAssistant from '../pages/MigrationAssistant.svelte';

  export let workspaceId;
  
  const dispatch = createEventDispatcher();
  
  let configurationSets = [];
  let selectedConfigurationSetId = null;
  let loading = true;
  let saving = false;
  
  // Migration assistant state
  let showMigrationAssistant = false;
  let migrationConfigSet = null;
  let pendingConfigurationChange = null;

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      loading = true;

      // Load all configuration sets
      const response = await api.configurationSets.getAll();
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
          const migrationAnalysis = await api.configurationSets.analyzeComprehensiveMigration(newConfigSet.id, parseInt(workspaceId));
          
          if (migrationAnalysis.requires_migration) {
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
      alert('Failed to update configuration set: ' + (error.message || error));
    } finally {
      saving = false;
    }
  }

  // Separate function to apply the actual configuration change
  async function applyConfigurationChange(newConfigSetId, currentConfigSet, newConfigSet) {
    // First, remove this workspace from all configuration sets
    const updatePromises = configurationSets
      .filter(cs => cs.workspace_ids && cs.workspace_ids.includes(parseInt(workspaceId)))
      .map(cs => {
        const updatedWorkspaceIds = cs.workspace_ids.filter(id => id !== parseInt(workspaceId));
        return api.configurationSets.update(cs.id, {
          name: cs.name,
          description: cs.description,
          workspace_ids: updatedWorkspaceIds,
          workflow_id: cs.workflow_id,
          create_screen_id: cs.create_screen_id,
          edit_screen_id: cs.edit_screen_id,
          view_screen_id: cs.view_screen_id,
          notification_setting_id: cs.notification_setting_id,
          is_default: cs.is_default,
          item_type_configs: cs.item_type_configs || [],
          priority_ids: cs.priority_ids || [],
          differentiate_by_item_type: cs.differentiate_by_item_type || false,
          default_item_type_id: cs.default_item_type_id || null
        });
      });
    
    // Wait for all removals to complete
    await Promise.all(updatePromises);
    
    // If a configuration set is selected, assign this workspace to it
    if (newConfigSetId) {
      const selectedConfigSet = configurationSets.find(cs => cs.id === newConfigSetId);
      if (selectedConfigSet) {
        const updatedWorkspaceIds = [...(selectedConfigSet.workspace_ids || []), parseInt(workspaceId)];
        await api.configurationSets.update(newConfigSetId, {
          name: selectedConfigSet.name,
          description: selectedConfigSet.description,
          workspace_ids: updatedWorkspaceIds,
          workflow_id: selectedConfigSet.workflow_id,
          create_screen_id: selectedConfigSet.create_screen_id,
          edit_screen_id: selectedConfigSet.edit_screen_id,
          view_screen_id: selectedConfigSet.view_screen_id,
          notification_setting_id: selectedConfigSet.notification_setting_id,
          is_default: selectedConfigSet.is_default,
          item_type_configs: selectedConfigSet.item_type_configs || [],
          priority_ids: selectedConfigSet.priority_ids || [],
          differentiate_by_item_type: selectedConfigSet.differentiate_by_item_type || false,
          default_item_type_id: selectedConfigSet.default_item_type_id || null
        });
      }
    }
    
    await loadData(); // Reload to refresh the data
    
    // Notify parent component about the change
    dispatch('configurationChanged', {
      oldConfigSet: currentConfigSet,
      newConfigSet: newConfigSet
    });
  }

  function getScreenName(screenId, context) {
    if (!screenId) return 'None';
    // The screen names are already loaded in the configuration set data
    return 'Configured'; // We could enhance this to show actual screen names
  }

  async function handleMigrationAssistantClose(event) {
    const { success, cancelled } = event.detail || {};
    
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
      alert('Failed to apply configuration change: ' + (error.message || error));
      
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
  <div>
    <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">Configuration Set</h3>
    <p class="text-sm mb-6" style="color: var(--ds-text-subtle);">
      Select a configuration set to define the workflow and screens used for different contexts (create, edit, view) within this workspace. Only one configuration set can be assigned per workspace.
    </p>
  </div>

  {#if loading}
    <Card rounded="xl" shadow padding="loose" class="text-center">
      <div class="animate-pulse" style="color: var(--ds-text-subtle);">Loading configuration sets...</div>
    </Card>
  {:else}
    <Card rounded="xl" shadow padding="spacious">
      <div class="space-y-6">
        <!-- Configuration Set Selection -->
        <div>
          <Label color="default" class="mb-3">Select Configuration Set</Label>
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
            <div class="text-sm" style="color: var(--ds-text-subtle);">Updating configuration...</div>
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
  on:close={handleMigrationAssistantClose}
/>