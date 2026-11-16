<script>
  import { useActionEditorContext } from './shared/actionEditorContext.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../stores/actionFlowStore.svelte.js';
  import Select from '../../components/Select.svelte';
  import FieldMappingsEditor from './shared/FieldMappingsEditor.svelte';
  import {
    applyAssetTypeChange,
    applyMappingsChange,
  } from './shared/assetConfigHelpers.svelte.js';

  let {
    selectedNode,
    flowStore = actionFlowStore,
    showPlaceholderModal = $bindable(false),
  } = $props();

  const editor = useActionEditorContext();
  let editorData = $derived(editor.getEditorData());
  let assetFields = $derived((editorData.assetReferenceFields?.items || []).filter((field) => field.field_type === 'asset'));
  let setId = $derived(editorData.sourceAssetSet(selectedNode.data?.config));
  let assetTypes = $derived(editorData.assetDestinations?.[String(setId)]?.types?.items || []);
  let targetFields = $derived(editorData.assetFieldsByType?.[`${setId}:${selectedNode.data?.config?.asset_type_id}`]?.items || []);
  let loading = $derived(editorData.assetReferenceFields?.state === 'loading');

  function handleSourceFieldChange(e) {
    const value = e.target.value;
    flowStore.updateNodeConfig(selectedNode.id, {
      source_field_id: value,
      asset_type_id: 0,
      asset_set_id: assetFields.find((field) => field.field_name === value)?.field_config?.asset_set_id || 0,
      field_mappings: []
    });
  }

  function handleAssetTypeChange(e) {
    applyAssetTypeChange(selectedNode.id, e.target.value, {}, flowStore);
  }

  function handleMappingsChange(mappings) {
    applyMappingsChange(selectedNode.id, mappings, flowStore);
  }
</script>

<div class="space-y-4">
  <!-- Step 1: Select source asset field -->
  <div>
    <label for="source-asset-field" class="block text-xs font-medium mb-1">{t('actions.config.sourceAssetField')}</label>
    <Select
      id="source-asset-field"
      options={[{ value: '', label: t('actions.config.selectAssetField') }, ...assetFields.map(f => ({ value: f.field_name, label: f.name ?? f.display_name ?? f.label ?? f.field_name }))]}
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
      {targetFields}
      sourceFields={editorData.customFields?.items || []}
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
