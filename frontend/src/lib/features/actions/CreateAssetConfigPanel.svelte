<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { HelpCircle } from '@lucide/svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../stores/actionFlowStore.svelte.js';
  import Select from '../../components/Select.svelte';
  import FieldMappingsEditor from './shared/FieldMappingsEditor.svelte';
  import {
    useAssetTypeFields,
    applyAssetTypeChange,
    applyMappingsChange,
  } from './shared/assetConfigHelpers.svelte.js';

  let { selectedNode, showPlaceholderModal = $bindable(false) } = $props();

  // Data state
  let assetSets = $state([]);
  let assetTypes = $state([]);
  let categories = $state([]);
  let statuses = $state([]);
  let loading = $state(true);

  const assetTypeFields = useAssetTypeFields(
    () => selectedNode?.data?.config?.asset_type_id
  );

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
    applyAssetTypeChange(selectedNode.id, e.target.value);
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

  function handleMappingsChange(mappings) {
    applyMappingsChange(selectedNode.id, mappings);
  }
</script>

<div class="space-y-4">
  <!-- Step 1: Select asset set -->
  <div>
    <label for="asset-set" class="block text-xs font-medium mb-1">{t('actions.config.assetSet')}</label>
    <Select
      id="asset-set"
      options={[{ value: '', label: t('actions.config.selectAssetSet') }, ...assetSets.map(set => ({ value: set.id, label: set.name }))]}
      value={selectedNode.data?.config?.asset_set_id || ''}
      onchange={(v) => handleAssetSetChange({ target: { value: v } })}
      disabled={loading}
    />
  </div>

  <!-- Step 2: Select asset type -->
  {#if selectedNode.data?.config?.asset_set_id}
    <div>
      <label for="asset-type" class="block text-xs font-medium mb-1">{t('actions.config.targetAssetType')}</label>
      <Select
        id="asset-type"
        options={[{ value: '', label: t('actions.config.selectAssetType') }, ...assetTypes.map(assetType => ({ value: assetType.id, label: assetType.name }))]}
        value={selectedNode.data?.config?.asset_type_id || ''}
        onchange={(v) => handleAssetTypeChange({ target: { value: v } })}
      />
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
        <Select
          id="asset-category"
          options={[{ value: '', label: t('actions.config.selectCategory') }, ...categories.map(category => ({ value: category.id, label: category.name }))]}
          value={selectedNode.data?.config?.category_id || ''}
          onchange={(v) => handleCategoryChange({ target: { value: v } })}
        />
      </div>

      <!-- Status -->
      <div class="mb-3">
        <label for="asset-status" class="block text-xs font-medium mb-1">{t('actions.config.assetStatus')}</label>
        <Select
          id="asset-status"
          options={[{ value: '', label: t('actions.config.selectStatusOptional') }, ...statuses.map(status => ({ value: status.id, label: status.name }))]}
          value={selectedNode.data?.config?.status_id || ''}
          onchange={(v) => handleStatusChange({ target: { value: v } })}
        />
      </div>
    </div>

    <!-- Step 4: Field mappings -->
    <FieldMappingsEditor
      mappings={selectedNode.data?.config?.field_mappings || []}
      targetFields={assetTypeFields.fields}
      bind:showPlaceholderModal
      onchange={handleMappingsChange}
    />
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
</style>
