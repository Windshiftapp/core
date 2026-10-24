<script>
  import FileText from '@lucide/svelte/icons/file-text';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Bell from '@lucide/svelte/icons/bell';
  import HelpCircle from '@lucide/svelte/icons/help-circle';
  import Zap from '@lucide/svelte/icons/zap';
  import X from '@lucide/svelte/icons/x';
  import Select from '../../components/Select.svelte';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import TriggerNode from '../actions/nodes/TriggerNode.svelte';
  import SetFieldNode from '../actions/nodes/SetFieldNode.svelte';
  import SetStatusNode from '../actions/nodes/SetStatusNode.svelte';
  import ConditionNode from '../actions/nodes/ConditionNode.svelte';
  import NotifyUserNode from '../actions/nodes/NotifyUserNode.svelte';
  import CreateItemNode from '../logbook-actions/nodes/CreateItemNode.svelte';
  import CreateItemConfigPanel from '../logbook-actions/CreateItemConfigPanel.svelte';
  import BaseActionFlowEditor from '../actions/shared/BaseActionFlowEditor.svelte';
  import ConditionConfigPanel from '../actions/shared/ConditionConfigPanel.svelte';
  import { assetActionFlowStore } from '../../stores/assetActionFlowStore.svelte.js';
  import FieldSelector from '../../pickers/FieldSelector.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import { assetStatuses as assetStatusesApi, assetTypes as assetTypesApi } from '../../api/assets.js';
  import { getUsers } from '../../api/users.js';
  import { t } from '../../stores/i18n.svelte.js';
  import {
    assetActionConditionFields,
    loadAssetActionCustomFields,
  } from './assetActionVariables.js';

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
  let assetTypes = $state([]);
  let assetStatuses = $state([]);
  let users = $state([]);
  let taxonomyLoadToken = 0;
  let fieldLoadToken = 0;

  let scopedAssetTypeId = $derived(
    assetActionFlowStore.nodes.find((node) => node.type === 'trigger')?.data?.config?.asset_type_id,
  );

  $effect(() => {
    const setId = action?.set_id;
    if (!setId) {
      taxonomyLoadToken += 1;
      assetTypes = [];
      assetStatuses = [];
      return;
    }
    const token = ++taxonomyLoadToken;
    Promise.all([
      assetTypesApi.getAll(setId),
      assetStatusesApi.getAll(setId),
    ]).then(([types, statuses]) => {
      if (token !== taxonomyLoadToken) return;
      assetTypes = types || [];
      assetStatuses = statuses || [];
    }).catch(() => {
      if (token !== taxonomyLoadToken) return;
      assetTypes = [];
      assetStatuses = [];
    });
  });

  $effect(() => {
    const typeId = scopedAssetTypeId;
    const token = ++fieldLoadToken;
    if (!typeId) {
      assetCustomFields = [];
      return;
    }
    loadAssetActionCustomFields(typeId).then((result) => {
      if (token !== fieldLoadToken) return;
      assetCustomFields = result;
    }).catch(() => {
      if (token !== fieldLoadToken) return;
      assetCustomFields = [];
    });
  });

  $effect(() => {
    getUsers().then((result) => {
      users = result || [];
    }).catch(() => {
      users = [];
    });
  });

  function getUserDisplayName(user) {
    if (!user) return '';
    const fullName = `${user.first_name || ''} ${user.last_name || ''}`.trim();
    return fullName || user.username || user.email || `User #${user.id}`;
  }

  function specificRecipientIds(config) {
    return (config?.recipients || []).filter((recipient) => /^\d+$/.test(String(recipient)));
  }

  function recipientDisplayName(id) {
    const user = users.find((candidate) => String(candidate.id) === String(id));
    return user ? getUserDisplayName(user) : `#${id}`;
  }

  function addRecipient(store, nodeId, config, user) {
    if (!user?.id) return;
    const recipients = specificRecipientIds(config);
    const id = String(user.id);
    if (recipients.includes(id)) return;
    store.updateNodeConfig(nodeId, {
      recipient_type: 'specific',
      recipients: [...recipients, id],
    });
  }

  function removeRecipient(store, nodeId, config, id) {
    store.updateNodeConfig(nodeId, {
      recipient_type: 'specific',
      recipients: specificRecipientIds(config).filter(
        (recipient) => String(recipient) !== String(id),
      ),
    });
  }

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

  const conditionFields = assetActionConditionFields;

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
        <label for="asset-trigger-from-status" class="block text-xs font-medium mb-1">From Status (optional)</label>
        <Select
          id="asset-trigger-from-status"
          options={[{ value: '', label: 'Any status' }, ...assetStatuses.map(status => ({ value: status.id, label: status.name }))]}
          value={selectedNode.data?.config?.from_status_id || ''}
          onchange={(value) =>
            store.updateNodeConfig(selectedNode.id, {
              from_status_id: parseInt(value) || null,
            })}
        />
      </div>
      <div>
        <label for="asset-trigger-to-status" class="block text-xs font-medium mb-1">To Status (optional)</label>
        <Select
          id="asset-trigger-to-status"
          options={[{ value: '', label: 'Any status' }, ...assetStatuses.map(status => ({ value: status.id, label: status.name }))]}
          value={selectedNode.data?.config?.to_status_id || ''}
          onchange={(value) =>
            store.updateNodeConfig(selectedNode.id, {
              to_status_id: parseInt(value) || null,
            })}
        />
      </div>
    {/if}

    <div>
      <label for="asset-trigger-type-filter" class="block text-xs font-medium mb-1">
        Asset Type {store.triggerType === 'manual' ? '(field scope)' : '(optional filter)'}
      </label>
      <Select
        id="asset-trigger-type-filter"
        options={[{ value: '', label: 'Any type' }, ...assetTypes.map(type => ({ value: type.id, label: type.name }))]}
        value={selectedNode.data?.config?.asset_type_id || ''}
        onchange={(value) =>
          store.updateNodeConfig(selectedNode.id, {
            asset_type_id: parseInt(value) || null,
          })}
      />
    </div>
  {/snippet}

  {#snippet nodeConfig(selectedNode, store, handleDeleteNode)}
    {#if selectedNode.type === 'create_item'}
      <CreateItemConfigPanel {selectedNode} flowStore={store} />
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
    {:else if selectedNode.type === 'set_field'}
      <div>
        <div class="block text-xs font-medium mb-1">Field</div>
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
        <div class="block text-xs font-medium mb-1">Value</div>
        <Input
          type="text"
          value={selectedNode.data?.config?.value || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, { value: e.currentTarget.value })}
          size="small"
        />
      </div>
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
    {:else if selectedNode.type === 'set_status'}
      <div>
        <label for="asset-action-set-status" class="block text-xs font-medium mb-1">Status</label>
        <Select
          id="asset-action-set-status"
          options={[{ value: '', label: 'Select status' }, ...assetStatuses.map(status => ({ value: status.id, label: status.name }))]}
          value={selectedNode.data?.config?.status_id || ''}
          onchange={(value) =>
            store.updateNodeConfig(selectedNode.id, {
              status_id: parseInt(value) || 0,
            })}
        />
      </div>
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
    {:else if selectedNode.type === 'condition'}
      <ConditionConfigPanel
        {selectedNode}
        {store}
        fields={conditionFields}
        operators={conditionOperators}
        onDelete={handleDeleteNode}
      />
    {:else if selectedNode.type === 'notify_user'}
      {@const recipientIds = specificRecipientIds(selectedNode.data?.config)}
      <div>
        <span class="block text-xs font-medium mb-1">Recipients</span>
        {#if recipientIds.length > 0}
          <div class="flex flex-wrap gap-1.5 mb-2">
            {#each recipientIds as id (id)}
              <span class="chip" data-testid={`asset-notify-recipient-chip-${id}`}>
                {recipientDisplayName(id)}
                <button
                  type="button"
                  class="chip-remove"
                  onclick={() => removeRecipient(store, selectedNode.id, selectedNode.data?.config, id)}
                  aria-label="Remove recipient"
                >
                  <X class="w-3 h-3" />
                </button>
              </span>
            {/each}
          </div>
        {/if}
        <div data-testid="asset-notify-recipient-add">
          <UserPicker
            value={null}
            {users}
            placeholder="Add recipient"
            showSelectedInTrigger={false}
            allowClear={false}
            onSelect={(user) =>
              addRecipient(store, selectedNode.id, selectedNode.data?.config, user)}
          />
        </div>
        <p class="text-xs mt-1 sidebar-hints">
          Recipients without access to this asset set are skipped.
        </p>
      </div>
      <div>
        <div class="block text-xs font-medium mb-1">Message</div>
        <Textarea
          rows={3}
          value={selectedNode.data?.config?.message || ''}
          oninput={(e) =>
            store.updateNodeConfig(selectedNode.id, { message: e.currentTarget.value })}
          size="small"
        />
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
