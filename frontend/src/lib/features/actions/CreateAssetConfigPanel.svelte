<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { Plus, Trash2, HelpCircle } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../stores/actionFlowStore.svelte.js';

  let { selectedNode, showPlaceholderModal = $bindable(false) } = $props();

  // Data state
  let assetSets = $state([]);
  let assetTypes = $state([]);
  let assetTypeFields = $state([]);
  let categories = $state([]);
  let statuses = $state([]);
  let loading = $state(true);

  // Load asset sets on mount
  onMount(async () => {
    try {
      assetSets = await api.assetSets.getAll() || [];
    } catch (error) {
      console.error('Failed to load asset sets:', error);
    } finally {
      loading = false;
    }
  });

  // When asset set changes, load types, categories, and statuses
  $effect(() => {
    const setId = selectedNode?.data?.config?.asset_set_id;
    if (setId) {
      loadAssetSetData(setId);
    } else {
      assetTypes = [];
      categories = [];
      statuses = [];
      assetTypeFields = [];
    }
  });

  // When asset type changes, load fields
  $effect(() => {
    const assetTypeId = selectedNode?.data?.config?.asset_type_id;
    if (assetTypeId) {
      loadAssetTypeFields(assetTypeId);
    } else {
      assetTypeFields = [];
    }
  });

  async function loadAssetSetData(setId) {
    try {
      const [typesResult, categoriesResult, statusesResult] = await Promise.all([
        api.assetTypes.getAll(setId),
        api.assetCategories.getAll(setId),
        api.assetStatuses.getAll(setId)
      ]);
      assetTypes = typesResult || [];
      categories = categoriesResult || [];
      statuses = statusesResult || [];
    } catch (error) {
      console.error('Failed to load asset set data:', error);
      assetTypes = [];
      categories = [];
      statuses = [];
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

  function handleAssetSetChange(e) {
    const value = parseInt(e.target.value) || 0;
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      asset_set_id: value,
      asset_type_id: 0,
      category_id: null,
      status_id: null,
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

  function handleTitleChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      title: e.target.value
    });
  }

  function handleDescriptionChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      description: e.target.value
    });
  }

  function handleAssetTagChange(e) {
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      asset_tag: e.target.value
    });
  }

  function handleCategoryChange(e) {
    const value = e.target.value ? parseInt(e.target.value) : null;
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      category_id: value
    });
  }

  function handleStatusChange(e) {
    const value = e.target.value ? parseInt(e.target.value) : null;
    actionFlowStore.updateNodeConfig(selectedNode.id, {
      status_id: value
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
  <!-- Step 1: Select asset set -->
  <div>
    <label for="asset-set" class="block text-xs font-medium mb-1">{t('actions.config.assetSet')}</label>
    <select
      id="asset-set"
      class="w-full px-3 py-2 border rounded-md text-sm config-input"
      value={selectedNode.data?.config?.asset_set_id || ''}
      onchange={handleAssetSetChange}
      disabled={loading}
    >
      <option value="">{t('actions.config.selectAssetSet')}</option>
      {#each assetSets as set}
        <option value={set.id}>{set.name}</option>
      {/each}
    </select>
  </div>

  <!-- Step 2: Select asset type -->
  {#if selectedNode.data?.config?.asset_set_id}
    <div>
      <label for="asset-type" class="block text-xs font-medium mb-1">{t('actions.config.targetAssetType')}</label>
      <select
        id="asset-type"
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

  <!-- Step 3: Asset details -->
  {#if selectedNode.data?.config?.asset_type_id}
    <div class="pt-2 border-t" style="border-color: var(--ds-border);">
      <!-- Title -->
      <div class="mb-3">
        <div class="flex items-center gap-1 mb-1">
          <label for="asset-title" class="block text-xs font-medium">{t('actions.config.assetTitle')}</label>
          <span class="text-red-500 text-xs">*</span>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        <input
          id="asset-title"
          type="text"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.title || ''}
          oninput={handleTitleChange}
          placeholder="Laptop for {'{{'}item.title{'}}'}"
        />
        <p class="text-xs mt-1 hint-text">{t('actions.config.assetTitleHint')}</p>
      </div>

      <!-- Description -->
      <div class="mb-3">
        <div class="flex items-center gap-1 mb-1">
          <label for="asset-description" class="block text-xs font-medium">{t('actions.config.assetDescription')}</label>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        <textarea
          id="asset-description"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          rows="2"
          value={selectedNode.data?.config?.description || ''}
          oninput={handleDescriptionChange}
          placeholder={t('actions.config.assetDescription')}
        ></textarea>
      </div>

      <!-- Asset Tag -->
      <div class="mb-3">
        <div class="flex items-center gap-1 mb-1">
          <label for="asset-tag" class="block text-xs font-medium">{t('actions.config.assetTagLabel')}</label>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        <input
          id="asset-tag"
          type="text"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.asset_tag || ''}
          oninput={handleAssetTagChange}
          placeholder="LAP-{'{{'}item.id{'}}'}"
        />
      </div>

      <!-- Category -->
      <div class="mb-3">
        <label for="asset-category" class="block text-xs font-medium mb-1">{t('actions.config.assetCategory')}</label>
        <select
          id="asset-category"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.category_id || ''}
          onchange={handleCategoryChange}
        >
          <option value="">{t('actions.config.selectCategory')}</option>
          {#each categories as category}
            <option value={category.id}>{category.name}</option>
          {/each}
        </select>
      </div>

      <!-- Status -->
      <div class="mb-3">
        <label for="asset-status" class="block text-xs font-medium mb-1">{t('actions.config.assetStatus')}</label>
        <select
          id="asset-status"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.status_id || ''}
          onchange={handleStatusChange}
        >
          <option value="">{t('actions.config.selectStatusOptional')}</option>
          {#each statuses as status}
            <option value={status.id}>{status.name}</option>
          {/each}
        </select>
      </div>
    </div>

    <!-- Step 4: Field mappings -->
    <div class="pt-2 border-t" style="border-color: var(--ds-border);">
      <div class="flex items-center justify-between mb-2">
        <span class="block text-xs font-medium">{t('actions.config.fieldMappingsLabel')}</span>
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
