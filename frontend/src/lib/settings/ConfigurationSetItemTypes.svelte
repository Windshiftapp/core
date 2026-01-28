<script>
  import { t } from '../stores/i18n.svelte.js';
  import { FileText } from 'lucide-svelte';
  import { itemTypeIconMap } from '../utils/icons.js';
  import ConfigurationSetEntityPicker from '../pickers/ConfigurationSetEntityPicker.svelte';
  import ScreenPicker from '../pickers/ScreenPicker.svelte';
  import WorkflowPicker from '../pickers/WorkflowPicker.svelte';

  let {
    itemTypes = [],
    workflows = [],
    screens = [],
    itemTypeConfigs = [],
    defaultWorkflowId = null,
    defaultCreateScreenId = null,
    defaultEditScreenId = null,
    defaultViewScreenId = null,
    showOverrides = false,
    onchange
  } = $props();

  // Get currently selected item type IDs from configs
  const selectedItemTypeIds = $derived(itemTypeConfigs.map(c => c.item_type_id));

  // Get assigned item types (those with configs)
  const assignedItemTypes = $derived(itemTypes.filter(it => selectedItemTypeIds.includes(it.id)));

  // Handle picker changes (add/remove item types)
  function handlePickerChange(newSelectedIds) {
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

    onchange?.(newConfigs);
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

    onchange?.(newConfigs);
  }
</script>

<div class="space-y-6">
  <!-- Item Type Picker -->
  <div>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
      {t('settings.configSets.selectItemTypes')}
    </p>

    <ConfigurationSetEntityPicker
      entityType="item-types"
      allEntities={itemTypes}
      selectedIds={selectedItemTypeIds}
      onchange={handlePickerChange}
    />
  </div>

  <!-- Override Configuration Table (only when showOverrides is enabled and there are assigned item types) -->
  {#if showOverrides && assignedItemTypes.length > 0}
    <div>
      <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text);">
        {t('settings.configSets.workflowScreenOverrides')}
      </h4>
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
        {t('settings.configSets.overridesDesc')}
      </p>

      <div class="border rounded-lg" style="border-color: var(--ds-border);">
        <table class="w-full text-sm">
          <thead>
            <tr style="background-color: var(--ds-surface);">
              <th class="text-left px-4 py-3 font-medium rounded-tl-lg w-40" style="color: var(--ds-text);">{t('settings.configSets.itemType')}</th>
              <th class="text-left px-4 py-3 font-medium" style="color: var(--ds-text);">{t('settings.configSets.workflow')}</th>
              <th class="text-left px-4 py-3 font-medium" style="color: var(--ds-text);">{t('settings.configSets.createScreen')}</th>
              <th class="text-left px-4 py-3 font-medium" style="color: var(--ds-text);">{t('settings.configSets.editScreen')}</th>
              <th class="text-left px-4 py-3 font-medium rounded-tr-lg" style="color: var(--ds-text);">{t('settings.configSets.viewScreen')}</th>
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
