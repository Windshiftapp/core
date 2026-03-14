<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { Bell } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import { getHandlePositions } from './flowDirection.js';

  let { data = {}, selected = false } = $props();

  let positions = $derived(getHandlePositions(actionFlowStore.direction));

  function getRecipientLabel(recipientType) {
    const labels = {
      'assignee': t('actions.recipients.assignee'),
      'creator': t('actions.recipients.creator'),
      'specific': t('actions.recipients.specific')
    };
    return labels[recipientType] || recipientType;
  }
</script>

<div class="notify-user-node action-flow-node" class:selected>
  <Handle type="target" position={positions.input} id="input" />

  <div class="node-header">
    <Bell size={16} class="node-icon" />
    <span class="node-title">{t('actions.nodes.notifyUser')}</span>
  </div>
  <div class="node-body">
    {#if data.config?.recipient_type}
      <div class="recipient-info">
        <span class="recipient-label">{t('actions.config.to')}:</span>
        <span class="recipient-value">{getRecipientLabel(data.config.recipient_type)}</span>
      </div>
      {#if data.config.message}
        <div class="message-preview">{data.config.message.substring(0, 40)}...</div>
      {/if}
    {:else}
      <div class="placeholder">{t('actions.config.selectRecipient')}</div>
    {/if}
  </div>

  <Handle type="source" position={positions.output} id="output" />
</div>

<style>
  .notify-user-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--ds-accent-magenta);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--ds-accent-magenta-subtle);
    border-bottom: 1px solid var(--ds-accent-magenta-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-icon {
    flex-shrink: 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-accent-magenta);
  }

  .node-body {
    padding: 10px 12px;
  }

  .recipient-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }

  .recipient-label {
    color: var(--ds-text-subtlest);
  }

  .recipient-value {
    color: var(--ds-text);
    font-weight: 500;
  }

  .message-preview {
    margin-top: 6px;
    font-size: 11px;
    color: var(--ds-text-subtle);
    font-style: italic;
  }

  .placeholder {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }

  :global(.notify-user-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-magenta);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
