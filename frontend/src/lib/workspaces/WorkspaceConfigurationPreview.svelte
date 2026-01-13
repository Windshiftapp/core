<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Settings, Workflow, Monitor, Bell } from 'lucide-svelte';
  import Lozenge from '../components/Lozenge.svelte';

  export let workspaceId;

  let configurationSet = null;
  let isUsingDefault = false;
  let loading = true;

  onMount(async () => {
    await loadConfigurationSet();
  });

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
  $: if (workspaceId) {
    loadConfigurationSet();
  }
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
      <div class="font-medium" style="color: var(--ds-text);">No Configuration Set Available</div>
      <div class="text-sm">Create a default configuration set to configure this workspace</div>
    </div>
  </div>
{:else}
  <div class="space-y-4">
    <!-- Configuration Set Info -->
    <div class="flex items-center gap-3">
      <Settings class="w-5 h-5" style="color: var(--ds-icon-accent);" />
      <div>
        <div class="font-medium flex items-center gap-2" style="color: var(--ds-text);">
          {configurationSet.name}
          {#if isUsingDefault}
            <Lozenge color="blue" text="Default" />
          {/if}
        </div>
        {#if configurationSet.description}
          <div class="text-sm" style="color: var(--ds-text-subtle);">{configurationSet.description}</div>
        {/if}
      </div>
    </div>

    <!-- Configuration Details -->
    <div class="space-y-4 pt-3 border-t" style="border-color: var(--ds-border);">
      <!-- Workflow -->
      <div class="flex items-center gap-2">
        <Workflow class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
        <div class="text-sm">
          <div style="color: var(--ds-text-subtle);">Workflow</div>
          <div class="font-medium" style="color: var(--ds-text);">
            {configurationSet.workflow_name || 'Not assigned'}
          </div>
        </div>
      </div>

      <!-- Screens -->
      <div class="flex items-start gap-2">
        <Monitor class="w-4 h-4 mt-0.5" style="color: var(--ds-icon-subtle);" />
        <div class="text-sm">
          <div class="mb-1" style="color: var(--ds-text-subtle);">Screens</div>
          <div class="space-y-1">
            <div class="flex justify-between">
              <span style="color: var(--ds-text-subtle);">Create:</span>
              <span class="font-medium" style="color: var(--ds-text);">{configurationSet.create_screen_name || 'Not assigned'}</span>
            </div>
            <div class="flex justify-between">
              <span style="color: var(--ds-text-subtle);">Edit:</span>
              <span class="font-medium" style="color: var(--ds-text);">{configurationSet.edit_screen_name || 'Not assigned'}</span>
            </div>
            <div class="flex justify-between">
              <span style="color: var(--ds-text-subtle);">View:</span>
              <span class="font-medium" style="color: var(--ds-text);">{configurationSet.view_screen_name || 'Not assigned'}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Notifications -->
      <div class="flex items-center gap-2">
        <Bell class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
        <div class="text-sm">
          <div style="color: var(--ds-text-subtle);">Notifications</div>
          <div class="font-medium" style="color: var(--ds-text);">
            {configurationSet.notification_setting_name || 'Not assigned'}
          </div>
        </div>
      </div>
    </div>
  </div>
{/if}