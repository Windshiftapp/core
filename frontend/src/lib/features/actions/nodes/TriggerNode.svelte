<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { Zap } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';

  export let data = {};
  export let selected = false;

  function getTriggerLabel(triggerType) {
    const labels = {
      'status_transition': t('actions.trigger.statusTransition'),
      'item_created': t('actions.trigger.itemCreated'),
      'item_updated': t('actions.trigger.itemUpdated'),
      'item_linked': t('actions.trigger.itemLinked')
    };
    return labels[triggerType] || triggerType;
  }

  function getStatusName(statusId) {
    if (!statusId || !data.statuses) return statusId;
    const status = data.statuses.find(s => s.id === statusId);
    return status?.name || statusId;
  }
</script>

<div class="trigger-node action-flow-node" class:selected>
  <div class="node-header">
    <Zap size={16} class="node-icon" />
    <span class="node-title">{t('actions.nodes.trigger')}</span>
  </div>
  <div class="node-body">
    <div class="trigger-type">{getTriggerLabel(data.triggerType)}</div>
    {#if data.config?.from_status_id || data.config?.to_status_id}
      <div class="trigger-config">
        {#if data.config.from_status_id}
          <span class="config-label">{t('actions.config.from')}:</span>
          <span class="config-value">{getStatusName(data.config.from_status_id)}</span>
        {/if}
        {#if data.config.to_status_id}
          <span class="config-label">{t('actions.config.to')}:</span>
          <span class="config-value">{getStatusName(data.config.to_status_id)}</span>
        {/if}
      </div>
    {/if}
  </div>

  <Handle type="source" position={Position.Right} id="output" />
</div>

<style>
  .trigger-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--ds-accent-blue);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--ds-accent-blue-subtle);
    border-bottom: 1px solid var(--ds-accent-blue-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-icon {
    flex-shrink: 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-accent-blue);
  }

  .node-body {
    padding: 10px 12px;
  }

  .trigger-type {
    font-size: 13px;
    font-weight: 500;
    color: var(--ds-text);
  }

  .trigger-config {
    margin-top: 6px;
    font-size: 11px;
    color: var(--ds-text-subtle);
  }

  .config-label {
    color: var(--ds-text-subtlest);
    margin-right: 4px;
  }

  .config-value {
    color: var(--ds-text-subtle);
  }

  :global(.trigger-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-blue);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
