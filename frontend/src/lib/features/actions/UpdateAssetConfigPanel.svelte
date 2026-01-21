<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { Plus, Trash2, HelpCircle } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../stores/actionFlowStore.svelte.js';

  let { selectedNode, showPlaceholderModal = $bindable(false) } = $props();

  // Data state
  let customFields = $state([]);
  let assetFields = $state([]);
  let assetTypes = $state([]);
  let assetTypeFields = $state([]);
  let loading = $state(true);

  // Load custom fields on mount
  onMount(async () => {
    try {
      customFields = await api.customFields.getAll() || [];
      // Filter to only asset type fields
      assetFields = customFields.filter(f => f.field_type === 'asset');
    } catch (error) {
      console.error('Failed to load custom fields:', error);
    } finally {
      loading = false;
    }
  });

  // When source field changes, load the asset types for that field's asset set
  $effect(() => {
    const sourceFieldId = selectedNode?.data?.config?.source_field_id;
    if (sourceFieldId) {
      loadAssetTypes(sourceFieldId);
    } else {
      assetTypes = [];
      assetTypeFields = [];
    }
  });

  // When asset type changes, load the fields for that type
  $effect(() => {
    const assetTypeId = selectedNode?.data?.config?.asset_type_id;
    if (assetTypeId) {
      loadAssetTypeFields(assetTypeId);
    } else {
      assetTypeFields = [];
    }
  });

  async function loadAssetTypes(sourceFieldId) {
    const field = assetFields.find(f => f.id === sourceFieldId || f.field_name === sourceFieldId);
    if (!field?.field_config?.asset_set_id) {
      assetTypes = [];
      return;
    }

    try {
      assetTypes = await api.assetTypes.getAll(field.field_config.asset_set_id) || [];
      // Update the asset_set_id in config
      actionFlowStore.updateNodeConfig(selectedNode.id, {
        asset_set_id: field.field_config.asset_set_id
      });
    } catch (error) {
      console.error('Failed to load asset types:', error);
      assetTypes = [];
    }
  }

  async function loadAssetTypeFields(assetTypeId) {
    try {
      assetTypeFields = await api.assetTypes.getFields(assetTypeId) || [];
    } catch (error) {
      console.error('Failed to load asset type fields:', error);
      assetTypeFields = [];
    }
  }

  function handleSourceFieldChange(e) {
    const value = e.target.value;
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      source_field_id: value,
      asset_type_id: 0,
      asset_set_id: 0,
      field_mappings: []
    });
  }

  function handleAssetTypeChange(e) {
    const value = parseInt(e.target.value) || 0;
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      asset_type_id: value,
      field_mappings: []
    });
  }

  function handleMappingChange(index, field, value) {
    const mappings = [...(selectedNode.data?.config?.field_mappings || [])];
    mappings[index] = { ...mappings[index], [field]: value };
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      field_mappings: mappings
    });
  }

  function addMapping() {
    const mappings = [...(selectedNode.data?.config?.field_mappings || [])];
    mappings.push({
      source_type: 'variable',
      source_value: '',
      target_field_id: ''
    });
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      field_mappings: mappings
    });
  }

  function removeMapping(index) {
    const mappings = [...(selectedNode.data?.config?.field_mappings || [])];
    mappings.splice(index, 1);
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      field_mappings: mappings
    });
  }

  const sourceTypes = [
    { value: 'variable', label: t('actions.config.sourceTypeVariable') },
    { value: 'item_field', label: t('actions.config.sourceTypeItemField') },
    { value: 'literal', label: t('actions.config.sourceTypeLiteral') }
  ];
</script>

<div class="space-y-4">
  <!-- Step 1: Select source asset field -->
  <div>
    <label class="block text-xs font-medium mb-1">{t('actions.config.sourceAssetField')}</label>
    <select
      class="w-full px-3 py-2 border rounded-md text-sm config-input"
      value={selectedNode.data?.config?.source_field_id || ''}
      onchange={handleSourceFieldChange}
      disabled={loading}
    >
      <option value="">{t('actions.config.selectAssetField')}</option>
      {#each assetFields as field}
        <option value={field.field_name}>{field.field_name}</option>
      {/each}
    </select>
    <p class="text-xs mt-1 hint-text">{t('actions.config.sourceAssetFieldHint')}</p>
  </div>

  <!-- Step 2: Select target asset type -->
  {#if selectedNode.data?.config?.source_field_id}
    <div>
      <label class="block text-xs font-medium mb-1">{t('actions.config.targetAssetType')}</label>
      <select
        class="w-full px-3 py-2 border rounded-md text-sm config-input"
        value={selectedNode.data?.config?.asset_type_id || ''}
        onchange={handleAssetTypeChange}
      >
        <option value="">{t('actions.config.selectAssetType')}</option>
        {#each assetTypes as assetType}
          <option value={assetType.id}>{assetType.name}</option>
        {/each}
      </select>
    </div>
  {/if}

  <!-- Step 3: Configure field mappings -->
  {#if selectedNode.data?.config?.asset_type_id}
    <div class="pt-2 border-t" style="border-color: var(--ds-border);">
      <div class="flex items-center justify-between mb-2">
        <label class="block text-xs font-medium">{t('actions.config.fieldMappingsLabel')}</label>
        <button
          onclick={() => showPlaceholderModal = true}
          class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
          title={t('actions.placeholders.showReference')}
        >
          <HelpCircle class="w-3.5 h-3.5" />
        </button>
      </div>

      <div class="space-y-3">
        {#each selectedNode.data?.config?.field_mappings || [] as mapping, index}
          <div class="mapping-row p-2 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-sunken);">
            <div class="flex items-start gap-2">
              <div class="flex-1 space-y-2">
                <!-- Source type -->
                <select
                  class="w-full px-2 py-1.5 border rounded text-xs config-input"
                  value={mapping.source_type}
                  onchange={(e) => handleMappingChange(index, 'source_type', e.target.value)}
                >
                  {#each sourceTypes as type}
                    <option value={type.value}>{type.label}</option>
                  {/each}
                </select>

                <!-- Source value -->
                <input
                  type="text"
                  class="w-full px-2 py-1.5 border rounded text-xs config-input"
                  value={mapping.source_value}
                  oninput={(e) => handleMappingChange(index, 'source_value', e.target.value)}
                  placeholder={mapping.source_type === 'variable' ? '{{item.assignee_id}}' : t('actions.config.fromField')}
                />

                <!-- Target field -->
                <select
                  class="w-full px-2 py-1.5 border rounded text-xs config-input"
                  value={mapping.target_field_id}
                  onchange={(e) => handleMappingChange(index, 'target_field_id', e.target.value)}
                >
                  <option value="">{t('actions.config.selectTargetField')}</option>
                  {#each assetTypeFields as field}
                    <option value={field.field_name}>{field.field_name}</option>
                  {/each}
                </select>
              </div>

              <button
                onclick={() => removeMapping(index)}
                class="p-1 text-red-500 hover:bg-red-50 rounded transition-colors flex-shrink-0"
                title="Remove mapping"
              >
                <Trash2 size={14} />
              </button>
            </div>
          </div>
        {/each}

        <button
          onclick={addMapping}
          class="w-full px-3 py-2 text-sm border border-dashed rounded-md flex items-center justify-center gap-2 add-mapping-btn"
        >
          <Plus size={14} />
          {t('actions.config.addMapping')}
        </button>
      </div>
    </div>
  {/if}
</div>

<style>
  .config-input {
    background-color: var(--ds-surface);
    border-color: var(--ds-border);
    color: var(--ds-text);
  }

  .config-input:focus {
    border-color: var(--ds-interactive);
    outline: none;
  }

  .hint-text {
    color: var(--ds-text-subtlest);
  }

  .add-mapping-btn {
    color: var(--ds-text-subtle);
    border-color: var(--ds-border);
    background-color: transparent;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .add-mapping-btn:hover {
    background-color: var(--ds-surface-hovered);
    border-color: var(--ds-interactive);
    color: var(--ds-interactive);
  }
</style>
