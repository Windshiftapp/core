<script>
  import { onMount } from 'svelte';
  import { itemTypes as itemTypesApi } from '../../api/configuration.js';
  import { logbookActionFlowStore } from '../../stores/logbookActionFlowStore.svelte.js';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';

  let { selectedNode, flowStore = logbookActionFlowStore } = $props();

  let itemTypes = $state([]);
  let loading = $state(true);

  onMount(async () => {
    try {
      itemTypes = await itemTypesApi.getAll() || [];
    } catch (error) {
      console.error('Failed to load item types:', error);
    } finally {
      loading = false;
    }
  });

  function handleWorkspaceSelect(workspace) {
    flowStore.updateNodeConfig(selectedNode.id, {
      workspace_id: workspace?.id ?? 0,
      item_type_id: 0
    });
  }

  function handleItemTypeSelect(itemType) {
    flowStore.updateNodeConfig(selectedNode.id, {
      item_type_id: itemType?.id ?? 0
    });
  }

  function handleTitleChange(e) {
    flowStore.updateNodeConfig(selectedNode.id, {
      title: e.target.value
    });
  }

  function handleDescriptionChange(e) {
    flowStore.updateNodeConfig(selectedNode.id, {
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
    <div class="block text-xs font-medium mb-1">Workspace</div>
    <WorkspacePicker
      multiple={false}
      value={selectedNode.data?.config?.workspace_id || null}
      onSelect={handleWorkspaceSelect}
      placeholder="Select workspace"
    />
  </div>

  <div>
    <div class="block text-xs font-medium mb-1">Item Type</div>
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
    <label for="create-item-title" class="block text-xs font-medium mb-1">Title Template</label>
    <Input
      id="create-item-title"
      type="text"
      placeholder={'{{doc.title}}'}
      value={selectedNode.data?.config?.title || ''}
      oninput={handleTitleChange}
      size="small"
    />
  </div>

  <div>
    <label for="create-item-description" class="block text-xs font-medium mb-1">Description Template</label>
    <Textarea
      id="create-item-description"
      rows={3}
      placeholder={"Document from logbook: {{doc.title}}\nLink: {{doc.link}}"}
      value={selectedNode.data?.config?.description || ''}
      oninput={handleDescriptionChange}
      style="background-color: var(--ds-surface);"
      size="small"
    />
  </div>
</div>
