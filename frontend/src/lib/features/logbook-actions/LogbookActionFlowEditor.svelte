<script>
  import { FileText, PlusSquare, Users, HelpCircle } from '@lucide/svelte';
  import Select from '../../components/Select.svelte';
  import Button from '../../components/Button.svelte';
  import LogbookTriggerNode from './nodes/LogbookTriggerNode.svelte';
  import CreateItemNode from './nodes/CreateItemNode.svelte';
  import AssociateCustomerNode from './nodes/AssociateCustomerNode.svelte';
  import LogbookConditionNode from './nodes/LogbookConditionNode.svelte';
  import CreateAssetNode from '../actions/nodes/CreateAssetNode.svelte';
  import CreateAssetConfigPanel from '../actions/CreateAssetConfigPanel.svelte';
  import CreateItemConfigPanel from './CreateItemConfigPanel.svelte';
  import BaseActionFlowEditor from '../actions/shared/BaseActionFlowEditor.svelte';
  import { logbookActionFlowStore } from '../../stores/logbookActionFlowStore.svelte.js';

  let { action, onSave, onCancel } = $props();

  const nodeTypes = {
    trigger: LogbookTriggerNode,
    create_item: CreateItemNode,
    create_asset: CreateAssetNode,
    associate_customer: AssociateCustomerNode,
    condition: LogbookConditionNode
  };

  const nodePalette = [
    { type: 'create_item', label: 'Create Item', icon: FileText },
    { type: 'create_asset', label: 'Create Asset', icon: PlusSquare },
    { type: 'associate_customer', label: 'Associate Customer', icon: Users },
    { type: 'condition', label: 'Condition', icon: HelpCircle }
  ];

  const triggerTypes = [
    { value: 'document_classified', label: 'Document Classified' },
    { value: 'content_keyword', label: 'Content Keyword' },
    { value: 'mime_type', label: 'MIME Type' },
    { value: 'manual', label: 'Manual' }
  ];

  const conditionFields = [
    { value: 'content_type', label: 'Content Type' },
    { value: 'mime_type', label: 'MIME Type' },
    { value: 'title', label: 'Title' },
    { value: 'source_type', label: 'Source Type' },
    { value: 'author', label: 'Author' }
  ];

  const conditionOperators = [
    { value: 'eq', label: 'Equals' },
    { value: 'ne', label: 'Not Equals' },
    { value: 'contains', label: 'Contains' },
    { value: 'not_contains', label: 'Not Contains' },
    { value: 'starts_with', label: 'Starts With' },
    { value: 'ends_with', label: 'Ends With' },
    { value: 'matches', label: 'Matches (Regex)' }
  ];
</script>

<BaseActionFlowEditor
  {action}
  flowStore={logbookActionFlowStore}
  {nodeTypes}
  {nodePalette}
  {triggerTypes}
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

    {#if store.triggerType === 'document_classified'}
      <div>
        <label class="block text-xs font-medium mb-1">Content Types</label>
        <div class="flex flex-col gap-1.5">
          {#each ['knowledge', 'record', 'correspondence'] as ct}
            <label class="checkbox-label">
              <input
                type="checkbox"
                checked={selectedNode.data?.config?.content_types?.includes(ct) || false}
                onchange={(e) => {
                  const current = selectedNode.data?.config?.content_types || [];
                  const updated = e.currentTarget.checked
                    ? [...current, ct]
                    : current.filter(c => c !== ct);
                  store.updateNodeConfig(selectedNode.id, { content_types: updated });
                }}
              />
              {ct}
            </label>
          {/each}
        </div>
      </div>
    {/if}

    {#if store.triggerType === 'content_keyword'}
      <div>
        <label class="block text-xs font-medium mb-1">Keywords (one per line)</label>
        <textarea
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          rows="4"
          value={selectedNode.data?.config?.keywords?.join('\n') || ''}
          oninput={(e) => {
            const keywords = e.currentTarget.value.split('\n').filter(k => k.trim());
            store.updateNodeConfig(selectedNode.id, { keywords });
          }}
        ></textarea>
      </div>
      <div>
        <label for="keyword-mode" class="block text-xs font-medium mb-1">Match Mode</label>
        <Select
          id="keyword-mode"
          options={[{ value: 'any', label: 'Match Any' }, { value: 'all', label: 'Match All' }]}
          value={selectedNode.data?.config?.keyword_mode || 'any'}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { keyword_mode: v })}
          size="small"
        />
      </div>
    {/if}

    {#if store.triggerType === 'mime_type'}
      <div>
        <label class="block text-xs font-medium mb-1">MIME Types (one per line)</label>
        <textarea
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          rows="3"
          placeholder="e.g. application/pdf&#10;image/*"
          value={selectedNode.data?.config?.mime_types?.join('\n') || ''}
          oninput={(e) => {
            const mime_types = e.currentTarget.value.split('\n').filter(m => m.trim());
            store.updateNodeConfig(selectedNode.id, { mime_types });
          }}
        ></textarea>
      </div>
    {/if}
  {/snippet}

  {#snippet nodeConfig(selectedNode, store, handleDeleteNode)}
    {#if selectedNode.type === 'create_item'}
      <CreateItemConfigPanel {selectedNode} />
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>

    {:else if selectedNode.type === 'create_asset'}
      <CreateAssetConfigPanel {selectedNode} />
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>

    {:else if selectedNode.type === 'associate_customer'}
      <div>
        <label class="block text-xs font-medium mb-1">Customer Organisation ID</label>
        <input
          type="number"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.customer_organisation_id || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { customer_organisation_id: parseInt(e.currentTarget.value) || null })}
        />
      </div>
      <div>
        <label class="block text-xs font-medium mb-1">Portal Customer ID</label>
        <input
          type="number"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.portal_customer_id || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { portal_customer_id: parseInt(e.currentTarget.value) || null })}
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
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { field_name: v })}
          size="small"
        />
      </div>
      <div>
        <label for="condition-operator" class="block text-xs font-medium mb-1">Operator</label>
        <Select
          id="condition-operator"
          options={conditionOperators}
          value={selectedNode.data?.config?.operator || 'eq'}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { operator: v })}
          size="small"
        />
      </div>
      <div>
        <label class="block text-xs font-medium mb-1">Value</label>
        <input
          type="text"
          class="w-full px-3 py-2 border rounded-md text-sm config-input"
          value={selectedNode.data?.config?.value || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { value: e.currentTarget.value })}
        />
      </div>
      <Button variant="ghost" size="small" onclick={handleDeleteNode}>Delete Node</Button>
    {/if}
  {/snippet}

  {#snippet sidebarExtra()}
    <h4 class="text-xs font-medium sidebar-subtitle mb-2 mt-4">Variables</h4>
    <ul class="text-xs space-y-1 sidebar-hints">
      <li><code>{'{{doc.title}}'}</code>, <code>{'{{doc.content_type}}'}</code></li>
      <li><code>{'{{doc.mime_type}}'}</code>, <code>{'{{doc.source_type}}'}</code></li>
      <li><code>{'{{doc.author}}'}</code>, <code>{'{{doc.id}}'}</code></li>
    </ul>
  {/snippet}
</BaseActionFlowEditor>
