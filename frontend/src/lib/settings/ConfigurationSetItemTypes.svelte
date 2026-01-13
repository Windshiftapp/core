<script>
  import { createEventDispatcher } from 'svelte';
  import { FileText } from 'lucide-svelte';
  import { itemTypeIconMap } from '../utils/icons.js';
  import ConfigurationSetEntityPicker from '../pickers/ConfigurationSetEntityPicker.svelte';
  import ScreenPicker from '../pickers/ScreenPicker.svelte';
  import WorkflowPicker from '../pickers/WorkflowPicker.svelte';

  export let itemTypes = [];
  export let workflows = [];
  export let screens = [];
  export let itemTypeConfigs = [];
  export let defaultWorkflowId = null;
  export let defaultCreateScreenId = null;
  export let defaultEditScreenId = null;
  export let defaultViewScreenId = null;
  export let showOverrides = false;

  const dispatch = createEventDispatcher();

  // Get currently selected item type IDs from configs
  $: selectedItemTypeIds = itemTypeConfigs.map(c => c.item_type_id);

  // Get assigned item types (those with configs)
  $: assignedItemTypes = itemTypes.filter(it => selectedItemTypeIds.includes(it.id));

  // Handle picker changes (add/remove item types)
  function handlePickerChange(event) {
    const newSelectedIds = event.detail;

    // Build new configs array
    const newConfigs = [];

    for (const itemTypeId of newSelectedIds) {
      // Check if there's an existing config
      const existingConfig = itemTypeConfigs.find(c => c.item_type_id === itemTypeId);
      if (existingConfig) {
        newConfigs.push(existingConfig);
      } else {
        // Create new config with defaults
        newConfigs.push({
          item_type_id: itemTypeId,
          workflow_id: null,
          create_screen_id: null,
          edit_screen_id: null,
          view_screen_id: null
        });
      }
    }

    dispatch('change', newConfigs);
  }

  // Get config for an item type
  function getConfig(itemTypeId) {
    return itemTypeConfigs.find(c => c.item_type_id === itemTypeId) || {
      item_type_id: itemTypeId,
      workflow_id: null,
      create_screen_id: null,
      edit_screen_id: null,
      view_screen_id: null
    };
  }

  function updateConfig(itemTypeId, field, value) {
    const newConfigs = [...itemTypeConfigs];
    const existingIndex = newConfigs.findIndex(c => c.item_type_id === itemTypeId);

    if (existingIndex >= 0) {
      newConfigs[existingIndex] = {
        ...newConfigs[existingIndex],
        [field]: value || null
      };
    } else {
      newConfigs.push({
        item_type_id: itemTypeId,
        workflow_id: null,
        create_screen_id: null,
        edit_screen_id: null,
        view_screen_id: null,
        [field]: value || null
      });
    }

    dispatch('change', newConfigs);
  }
</script>

<div class="space-y-6">
  <!-- Item Type Picker -->
  <div>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
      Select which item types are available in workspaces using this configuration set.
    </p>

    <ConfigurationSetEntityPicker
      entityType="item-types"
      allEntities={itemTypes}
      selectedIds={selectedItemTypeIds}
      on:change={handlePickerChange}
    />
  </div>

  <!-- Override Configuration Table (only when showOverrides is enabled and there are assigned item types) -->
  {#if showOverrides && assignedItemTypes.length > 0}
    <div>
      <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text);">
        Workflow & Screen Overrides
      </h4>
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
        Configure custom workflows and screens per item type. Use "Default" to inherit from the General tab.
      </p>

      <div class="border rounded-lg" style="border-color: var(--ds-border);">
        <table class="w-full text-sm">
          <thead>
            <tr style="background-color: var(--ds-surface);">
              <th class="text-left px-4 py-3 font-medium rounded-tl-lg w-40" style="color: var(--ds-text);">Item Type</th>
              <th class="text-left px-4 py-3 font-medium" style="color: var(--ds-text);">Workflow</th>
              <th class="text-left px-4 py-3 font-medium" style="color: var(--ds-text);">Create Screen</th>
              <th class="text-left px-4 py-3 font-medium" style="color: var(--ds-text);">Edit Screen</th>
              <th class="text-left px-4 py-3 font-medium rounded-tr-lg" style="color: var(--ds-text);">View Screen</th>
            </tr>
          </thead>
          <tbody>
            {#each assignedItemTypes as itemType}
              {@const config = getConfig(itemType.id)}
              <tr class="border-t" style="border-color: var(--ds-border);">
                <td class="px-4 py-3">
                  <div class="flex items-center gap-2">
                    <div
                      class="w-6 h-6 rounded flex items-center justify-center flex-shrink-0"
                      style="background-color: {itemType.color || '#3b82f6'};"
                    >
                      <svelte:component
                        this={itemTypeIconMap[itemType.icon] || FileText}
                        class="w-4 h-4 text-white"
                      />
                    </div>
                    <span class="font-medium" style="color: var(--ds-text);">{itemType.name}</span>
                  </div>
                </td>
                <td class="px-4 py-3">
                  <WorkflowPicker
                    value={config.workflow_id}
                    items={workflows}
                    {defaultWorkflowId}
                    placeholder="Select workflow..."
                    onSelect={(workflow) => updateConfig(itemType.id, 'workflow_id', workflow?.id || null)}
                  />
                </td>
                <td class="px-4 py-3">
                  <ScreenPicker
                    value={config.create_screen_id}
                    items={screens}
                    defaultScreenId={defaultCreateScreenId}
                    placeholder="Select screen..."
                    onSelect={(screen) => updateConfig(itemType.id, 'create_screen_id', screen?.id || null)}
                  />
                </td>
                <td class="px-4 py-3">
                  <ScreenPicker
                    value={config.edit_screen_id}
                    items={screens}
                    defaultScreenId={defaultEditScreenId}
                    placeholder="Select screen..."
                    onSelect={(screen) => updateConfig(itemType.id, 'edit_screen_id', screen?.id || null)}
                  />
                </td>
                <td class="px-4 py-3">
                  <ScreenPicker
                    value={config.view_screen_id}
                    items={screens}
                    defaultScreenId={defaultViewScreenId}
                    placeholder="Select screen..."
                    onSelect={(screen) => updateConfig(itemType.id, 'view_screen_id', screen?.id || null)}
                  />
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>
