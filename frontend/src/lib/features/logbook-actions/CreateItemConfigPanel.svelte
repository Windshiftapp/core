<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { logbookActionFlowStore } from '../../stores/logbookActionFlowStore.svelte.js';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';

  let { selectedNode } = $props();

  let itemTypes = $state([]);
  let loading = $state(true);

  onMount(async () => {
    try {
      itemTypes = await api.itemTypes.getAll() || [];
    } catch (error) {
      console.error('Failed to load item types:', error);
    } finally {
      loading = false;
    }
  });

  function handleWorkspaceSelect(workspace) {
    logbookActionFlowStore.updateNodeConfig(selectedNode.id, {
      workspace_id: workspace?.id ?? 0,
      item_type_id: 0
    });
  }

  function handleItemTypeSelect(itemType) {
    logbookActionFlowStore.updateNodeConfig(selectedNode.id, {
      item_type_id: itemType?.id ?? 0
    });
  }

  function handleTitleChange(e) {
    logbookActionFlowStore.updateNodeConfig(selectedNode.id, {
      title: e.target.value
    });
  }

  function handleDescriptionChange(e) {
    logbookActionFlowStore.updateNodeConfig(selectedNode.id, {
      description: e.target.value
    });
  }

  const itemTypePickerConfig = {
    icon: {
      type: 'color-dot',
      source: (item) => item.color || '#6b7280',
      size: 'w-2.5 h-2.5'
    },
    primary: { text: (item) => item.name || '' },
    searchFields: ['name'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name || ''
  };
</script>

<div class="space-y-4">
  <div>
    <label class="block text-xs font-medium mb-1">Workspace</label>
    <WorkspacePicker
      multiple={false}
      value={selectedNode.data?.config?.workspace_id || null}
      onSelect={handleWorkspaceSelect}
      placeholder="Select workspace"
    />
  </div>

  <div>
    <label class="block text-xs font-medium mb-1">Item Type</label>
    <ItemPicker
      items={itemTypes}
      value={selectedNode.data?.config?.item_type_id || null}
      config={itemTypePickerConfig}
      onSelect={handleItemTypeSelect}
      placeholder="Select item type"
      {loading}
    />
  </div>

  <div>
    <label class="block text-xs font-medium mb-1">Title Template</label>
    <input
      type="text"
      class="w-full px-3 py-2 border rounded-md text-sm config-input"
      placeholder="{'{{doc.title}}'}"
      value={selectedNode.data?.config?.title || ''}
      oninput={handleTitleChange}
    />
  </div>

  <div>
    <label class="block text-xs font-medium mb-1">Description Template</label>
    <textarea
      class="w-full px-3 py-2 border rounded-md text-sm config-input"
      rows="3"
      placeholder={"Document from logbook: {{doc.title}}\nLink: {{doc.link}}"}
      value={selectedNode.data?.config?.description || ''}
      oninput={handleDescriptionChange}
    ></textarea>
  </div>
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
</style>
