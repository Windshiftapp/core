<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../stores/actionFlowStore.svelte.js';
  import Select from '../../components/Select.svelte';
  import FieldMappingsEditor from './shared/FieldMappingsEditor.svelte';

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
      customFields = (await api.customFields.getAll())?.data || [];
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

  function handleMappingsChange(mappings) {
    actionFlowStore.updateNodeConfig(selectedNode.id, { field_mappings: mappings });
  }
</script>

<div class="space-y-4">
  <!-- Step 1: Select source asset field -->
  <div>
    <label for="source-asset-field" class="block text-xs font-medium mb-1">{t('actions.config.sourceAssetField')}</label>
    <Select
      id="source-asset-field"
      options={[{ value: '', label: t('actions.config.selectAssetField') }, ...assetFields.map(f => ({ value: f.field_name, label: f.field_name }))]}
      value={selectedNode.data?.config?.source_field_id || ''}
      onchange={(v) => handleSourceFieldChange({ target: { value: v } })}
      disabled={loading}
    />
    <p class="text-xs mt-1 hint-text">{t('actions.config.sourceAssetFieldHint')}</p>
  </div>

  <!-- Step 2: Select target asset type -->
  {#if selectedNode.data?.config?.source_field_id}
    <div>
      <label for="target-asset-type" class="block text-xs font-medium mb-1">{t('actions.config.targetAssetType')}</label>
      <Select
        id="target-asset-type"
        options={[{ value: '', label: t('actions.config.selectAssetType') }, ...assetTypes.map(a => ({ value: a.id, label: a.name }))]}
        value={selectedNode.data?.config?.asset_type_id || ''}
        onchange={(v) => handleAssetTypeChange({ target: { value: v } })}
      />
    </div>
  {/if}

  <!-- Step 3: Configure field mappings -->
  {#if selectedNode.data?.config?.asset_type_id}
    <FieldMappingsEditor
      mappings={selectedNode.data?.config?.field_mappings || []}
      targetFields={assetTypeFields}
      bind:showPlaceholderModal
      onchange={handleMappingsChange}
    />
  {/if}
</div>

<style>
  .hint-text {
    color: var(--ds-text-subtlest);
  }
</style>
