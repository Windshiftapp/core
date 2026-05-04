<script>
  import { t } from '../stores/i18n.svelte.js';
  import { FileText, ChevronRight, ChevronDown } from 'lucide-svelte';
  import { itemTypeIconMap } from '../utils/icons.js';
  import ConfigurationSetEntityPicker from '../pickers/ConfigurationSetEntityPicker.svelte';
  import ScreenPicker from '../pickers/ScreenPicker.svelte';
  import WorkflowPicker from '../pickers/WorkflowPicker.svelte';
  import ConditionSetPicker from '../pickers/ConditionSetPicker.svelte';
  import ApprovalSetPicker from '../pickers/ApprovalSetPicker.svelte';

  let {
    itemTypes = [],
    workflows = [],
    screens = [],
    conditionSets = [],
    approvalSets = [],
    itemTypeConfigs = [],
    defaultWorkflowId = null,
    defaultConditionSetId = null,
    defaultApprovalSetId = null,
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
          condition_set_id: null,
          approval_set_id: null,
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
      condition_set_id: null,
      approval_set_id: null,
      create_screen_id: null,
      edit_screen_id: null,
      view_screen_id: null
    };
  }

  // Get the effective workflow ID for an item type (override or default)
  function getEffectiveWorkflowId(itemTypeId) {
    const config = getConfig(itemTypeId);
    return config.workflow_id || defaultWorkflowId;
  }

  function updateConfig(itemTypeId, field, value) {
    const newConfigs = [...itemTypeConfigs];
    const existingIndex = newConfigs.findIndex(c => c.item_type_id === itemTypeId);

    if (existingIndex >= 0) {
      const updated = {
        ...newConfigs[existingIndex],
        [field]: value || null
      };
      // Clear condition_set_id / approval_set_id if workflow changes and the
      // selected set no longer matches the effective workflow.
      if (field === 'workflow_id') {
        const effectiveWf = value || defaultWorkflowId;
        if (updated.condition_set_id) {
          const cs = conditionSets.find(c => c.id === updated.condition_set_id);
          if (!cs || cs.workflow_id !== effectiveWf) {
            updated.condition_set_id = null;
          }
        }
        if (updated.approval_set_id) {
          const ap = approvalSets.find(a => a.id === updated.approval_set_id);
          if (!ap || ap.workflow_id !== effectiveWf) {
            updated.approval_set_id = null;
          }
        }
      }
      newConfigs[existingIndex] = updated;
    } else {
      newConfigs.push({
        item_type_id: itemTypeId,
        workflow_id: null,
        condition_set_id: null,
        approval_set_id: null,
        create_screen_id: null,
        edit_screen_id: null,
        view_screen_id: null,
        [field]: value || null
      });
    }

    onchange?.(newConfigs);
  }

  // Per-row expansion state for the collapsible Screens cell. Auto-expands
  // for any row that already carries a screen override so the user lands on
  // the configured value without an extra click.
  let expandedScreenRows = $state(new Set());
  $effect(() => {
    const next = new Set(expandedScreenRows);
    for (const c of itemTypeConfigs) {
      if (c.create_screen_id || c.edit_screen_id || c.view_screen_id) {
        next.add(c.item_type_id);
      }
    }
    if (next.size !== expandedScreenRows.size) expandedScreenRows = next;
  });

  function toggleScreens(itemTypeId) {
    const next = new Set(expandedScreenRows);
    if (next.has(itemTypeId)) next.delete(itemTypeId);
    else next.add(itemTypeId);
    expandedScreenRows = next;
  }

  function screenSummary(config) {
    const n = (config.create_screen_id ? 1 : 0)
            + (config.edit_screen_id ? 1 : 0)
            + (config.view_screen_id ? 1 : 0);
    if (n === 0) return 'default';
    return `${n} custom`;
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
              <th class="text-left px-4 py-3 font-medium" style="color: var(--ds-text);">{t('conditionSets.title')}</th>
              <th class="text-left px-4 py-3 font-medium" style="color: var(--ds-text);">{t('approvalSets.title')}</th>
              <th class="text-left px-4 py-3 font-medium rounded-tr-lg" style="color: var(--ds-text);">Screens</th>
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
                  <ConditionSetPicker
                    value={config.condition_set_id}
                    items={conditionSets}
                    workflowId={getEffectiveWorkflowId(itemType.id)}
                    disabled={!getEffectiveWorkflowId(itemType.id)}
                    onSelect={(cs) => updateConfig(itemType.id, 'condition_set_id', cs?.id || null)}
                  />
                </td>
                <td class="px-4 py-3">
                  <ApprovalSetPicker
                    value={config.approval_set_id}
                    items={approvalSets}
                    workflowId={getEffectiveWorkflowId(itemType.id)}
                    disabled={!getEffectiveWorkflowId(itemType.id)}
                    onSelect={(ap) => updateConfig(itemType.id, 'approval_set_id', ap?.id || null)}
                  />
                </td>
                <td class="px-4 py-3">
                  <button
                    type="button"
                    class="inline-flex items-center gap-1.5 px-2 py-1 rounded text-sm hover:opacity-80"
                    style="color: var(--ds-text-subtle);"
                    onclick={() => toggleScreens(itemType.id)}
                  >
                    {#if expandedScreenRows.has(itemType.id)}
                      <ChevronDown class="w-4 h-4" />
                    {:else}
                      <ChevronRight class="w-4 h-4" />
                    {/if}
                    <span>{screenSummary(config)}</span>
                  </button>
                </td>
              </tr>
              {#if expandedScreenRows.has(itemType.id)}
                <tr>
                  <td colspan="5" class="px-4 pb-4 pt-1">
                    <div
                      class="rounded-md border p-3"
                      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
                    >
                      <div class="text-xs font-semibold uppercase tracking-wider mb-2" style="color: var(--ds-text-subtle);">
                        Screens
                      </div>
                      <div class="grid grid-cols-3 gap-3">
                        <div>
                          <label class="text-xs block mb-1" style="color: var(--ds-text-subtle);">{t('settings.configSets.createScreen')}</label>
                          <ScreenPicker
                            value={config.create_screen_id}
                            items={screens}
                            defaultScreenId={defaultCreateScreenId}
                            placeholder="Select screen..."
                            onSelect={(screen) => updateConfig(itemType.id, 'create_screen_id', screen?.id || null)}
                          />
                        </div>
                        <div>
                          <label class="text-xs block mb-1" style="color: var(--ds-text-subtle);">{t('settings.configSets.editScreen')}</label>
                          <ScreenPicker
                            value={config.edit_screen_id}
                            items={screens}
                            defaultScreenId={defaultEditScreenId}
                            placeholder="Select screen..."
                            onSelect={(screen) => updateConfig(itemType.id, 'edit_screen_id', screen?.id || null)}
                          />
                        </div>
                        <div>
                          <label class="text-xs block mb-1" style="color: var(--ds-text-subtle);">{t('settings.configSets.viewScreen')}</label>
                          <ScreenPicker
                            value={config.view_screen_id}
                            items={screens}
                            defaultScreenId={defaultViewScreenId}
                            placeholder="Select screen..."
                            onSelect={(screen) => updateConfig(itemType.id, 'view_screen_id', screen?.id || null)}
                          />
                        </div>
                      </div>
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>
