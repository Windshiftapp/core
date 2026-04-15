<script>
  import { FileText, Pencil, Bell, HelpCircle, Zap } from 'lucide-svelte';
  import Select from '../../components/Select.svelte';
  import Button from '../../components/Button.svelte';
  import TriggerNode from '../actions/nodes/TriggerNode.svelte';
  import SetFieldNode from '../actions/nodes/SetFieldNode.svelte';
  import SetStatusNode from '../actions/nodes/SetStatusNode.svelte';
  import ConditionNode from '../actions/nodes/ConditionNode.svelte';
  import NotifyUserNode from '../actions/nodes/NotifyUserNode.svelte';
  import CreateItemNode from '../logbook-actions/nodes/CreateItemNode.svelte';
  import CreateItemConfigPanel from '../logbook-actions/CreateItemConfigPanel.svelte';
  import BaseActionFlowEditor from '../actions/shared/BaseActionFlowEditor.svelte';
  import { assetActionFlowStore } from '../../stores/assetActionFlowStore.svelte.js';
  import FieldSelector from '../../pickers/FieldSelector.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { action, onSave, onCancel } = $props();

  const assetFieldGroups = [
    {
      category: t('pickers.fieldCategories.basic'),
      fields: [
        { id: 'title', name: 'Title', type: 'text' },
        { id: 'asset_tag', name: 'Asset Tag', type: 'identifier' },
        { id: 'description', name: 'Description', type: 'text' },
      ],
    },
  ];

  let assetCustomFields = $state([]);
  $effect(() => {
    api.customFields.getAll().then((result) => {
      assetCustomFields = (result?.data || []).map((field) => ({
        id: String(field.id),
        name: field.name,
        type: field.field_type,
        description: field.description || '',
        isCustom: true,
      }));
    }).catch(() => {
      assetCustomFields = [];
    });
  });

  function resolveSetField(selectedNode) {
    if (!selectedNode || selectedNode.type !== 'set_field') return null;
    const config = selectedNode.data?.config;
    if (!config?.field_name) return null;
    const builtIn = assetFieldGroups[0].fields.find((f) => f.id === config.field_name);
    if (builtIn) return builtIn;
    const custom = assetCustomFields.find((f) => f.id === config.field_name);
    if (custom) return custom;
    return { id: config.field_name, name: config.field_display_name || config.field_name, type: 'text' };
  }

  const nodeTypes = {
    trigger: TriggerNode,
    create_item: CreateItemNode,
    set_field: SetFieldNode,
    set_status: SetStatusNode,
    condition: ConditionNode,
    notify_user: NotifyUserNode,
  };

  const nodePalette = [
    { type: 'create_item', label: 'Create Work Item', icon: FileText },
    { type: 'set_field', label: 'Set Field', icon: Pencil },
    { type: 'set_status', label: 'Set Status', icon: Zap },
    { type: 'condition', label: 'Condition', icon: HelpCircle },
    { type: 'notify_user', label: 'Notify User', icon: Bell },
  ];

  const triggerTypes = [
    { value: 'asset_created', label: 'Asset Created' },
    { value: 'asset_updated', label: 'Asset Updated' },
    { value: 'asset_status_changed', label: 'Status Changed' },
    { value: 'manual', label: 'Manual' },
  ];

  const conditionFields = [
    { value: 'title', label: 'Title' },
    { value: 'asset_tag', label: 'Asset Tag' },
    { value: 'type_name', label: 'Type Name' },
    { value: 'status_name', label: 'Status Name' },
  ];

  const conditionOperators = [
    { value: 'eq', label: 'Equals' },
    { value: 'ne', label: 'Not Equals' },
    { value: 'contains', label: 'Contains' },
    { value: 'not_contains', label: 'Not Contains' },
    { value: 'starts_with', label: 'Starts With' },
    { value: 'ends_with', label: 'Ends With' },
  ];
</script>

<BaseActionFlowEditor
  {action}
  flowStore={assetActionFlowStore}
  {nodeTypes}
  {nodePalette}
  {triggerTypes}
  sidebarTitle="Asset Actions"
  {onSave}
  {onCancel}
>
  {#snippet triggerConfig(selectedNode, store)}
    <div>
      <label for="trigger-type" class="block text-xs font-medium mb-1">Trigger Type</label>
      <Select
        id="trigger-type"
        options={triggerTypes}
        value={store.triggerType}
        onchange={(v) => {
          store.updateNodeData(selectedNode.id, { triggerType: v });
          store.updateTriggerType(v);
        }}
        size="small"
      />
    </div>

    {#if store.triggerType === 'asset_status_changed'}
      <div>
        <label class="block text-xs font-medium mb-1">From Status ID (optional)</label>
        <input
          type="number"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.from_status_id || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, {
              from_status_id: parseInt(e.target.value) || null,
            })}
        />
      </div>
      <div>
        <label class="block text-xs font-medium mb-1">To Status ID (optional)</label>
        <input
          type="number"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.to_status_id || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, {
              to_status_id: parseInt(e.target.value) || null,
            })}
        />
      </div>
    {/if}

    {#if store.triggerType !== 'manual'}
      <div>
        <label class="block text-xs font-medium mb-1">Asset Type ID (optional filter)</label>
        <input
          type="number"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.asset_type_id || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, {
              asset_type_id: parseInt(e.target.value) || null,
            })}
        />
      </div>
    {/if}
  {/snippet}

  {#snippet nodeConfig(selectedNode, store, handleDeleteNode)}
    {#if selectedNode.type === 'create_item'}
      <CreateItemConfigPanel {selectedNode} />
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
    {:else if selectedNode.type === 'set_field'}
      <div>
        <label class="block text-xs font-medium mb-1">Field</label>
        <FieldSelector
          fieldGroups={assetFieldGroups}
          customFieldItems={assetCustomFields}
          selectedField={resolveSetField(selectedNode)}
          onSelect={(field) => {
            store.updateNodeConfig(selectedNode.id, {
              field_name: field.id,
              field_display_name: field.name,
            });
          }}
          onClear={() => {
            store.updateNodeConfig(selectedNode.id, {
              field_name: '',
              field_display_name: '',
            });
          }}
        />
      </div>
      <div>
        <label class="block text-xs font-medium mb-1">Value</label>
        <input
          type="text"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.value || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, { value: e.target.value })}
        />
      </div>
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
    {:else if selectedNode.type === 'set_status'}
      <div>
        <label class="block text-xs font-medium mb-1">Status ID</label>
        <input
          type="number"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.status_id || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, {
              status_id: parseInt(e.target.value) || 0,
            })}
        />
      </div>
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
    {:else if selectedNode.type === 'condition'}
      <div>
        <label for="condition-field" class="block text-xs font-medium mb-1">Field</label>
        <Select
          id="condition-field"
          options={conditionFields}
          value={selectedNode.data?.config?.field_name || ''}
          onchange={(v) =>
            store.updateNodeConfig(selectedNode.id, { field_name: v })}
          size="small"
        />
      </div>
      <div>
        <label for="condition-operator" class="block text-xs font-medium mb-1">Operator</label>
        <Select
          id="condition-operator"
          options={conditionOperators}
          value={selectedNode.data?.config?.operator || 'eq'}
          onchange={(v) =>
            store.updateNodeConfig(selectedNode.id, { operator: v })}
          size="small"
        />
      </div>
      <div>
        <label class="block text-xs font-medium mb-1">Value</label>
        <input
          type="text"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.value || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, { value: e.target.value })}
        />
      </div>
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
    {:else if selectedNode.type === 'notify_user'}
      <div>
        <label class="block text-xs font-medium mb-1">User ID</label>
        <input
          type="number"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.user_id || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, {
              user_id: parseInt(e.target.value) || 0,
            })}
        />
      </div>
      <div>
        <label class="block text-xs font-medium mb-1">Message</label>
        <textarea
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          rows="3"
          value={selectedNode.data?.config?.message || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, { message: e.target.value })}
        ></textarea>
      </div>
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
    {/if}
  {/snippet}

  {#snippet sidebarExtra()}
    <h4 class="text-xs font-medium sidebar-subtitle mb-2 mt-4">Variables</h4>
    <ul class="text-xs space-y-1 sidebar-hints">
      <li><code>{'{{asset.title}}'}</code>, <code>{'{{asset.tag}}'}</code></li>
      <li><code>{'{{asset.type_name}}'}</code>, <code>{'{{asset.status_name}}'}</code></li>
      <li><code>{'{{asset.id}}'}</code>, <code>{'{{actor.id}}'}</code></li>
    </ul>
  {/snippet}
</BaseActionFlowEditor>
