<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api.js';
  import ConfigurationSetEntityPicker from '../pickers/ConfigurationSetEntityPicker.svelte';

  let {
    allWorkspaces = [],
    selectedWorkspaceIds = [],
    configurationSetId = null
  } = $props();

  const dispatch = createEventDispatcher();

  let workspaceAssignments = $state({}); // Maps workspace_id to config_set info for conflict detection

  // Load which workspaces are assigned to which config sets (for conflict warnings)
  onMount(loadWorkspaceAssignments);

  async function loadWorkspaceAssignments() {
    try {
      const response = await api.configurationSets.getAll({ limit: 1000 });
      const configSets = response.configuration_sets || [];

      workspaceAssignments = {};
      for (const cs of configSets) {
        if (cs.workspace_ids) {
          for (const wsId of cs.workspace_ids) {
            // Only track workspaces assigned to OTHER config sets
            if (cs.id !== configurationSetId) {
              workspaceAssignments[wsId] = {
                configSetId: cs.id,
                configSetName: cs.name
              };
            }
          }
        }
      }
    } catch (error) {
      console.error('Failed to load workspace assignments:', error);
    }
  }

  function handleChange(event) {
    dispatch('change', event.detail);
  }
</script>

<div>
  <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
    {t('settings.configSets.selectWorkspaces')}
  </p>

  <ConfigurationSetEntityPicker
    entityType="workspaces"
    allEntities={allWorkspaces}
    selectedIds={selectedWorkspaceIds}
    {configurationSetId}
    entityAssignments={workspaceAssignments}
    on:change={handleChange}
  />
</div>
