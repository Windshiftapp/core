<script>
  import { Bell } from '@lucide/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();

  function getRecipientLabel(recipientType) {
    const labels = {
      'assignee': t('actions.recipients.assignee'),
      'creator': t('actions.recipients.creator'),
      'specific': t('actions.recipients.specific')
    };
    return labels[recipientType] || recipientType;
  }

  // For the "specific" type, count the explicit user-id recipients so the node
  // summary shows "Specific (2)" rather than a bare "Specific".
  let specificCount = $derived(
    (data.config?.recipients || []).filter((r) => /^\d+$/.test(String(r))).length
  );
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={Bell} title={t('actions.nodes.notifyUser')} accentColor="magenta">
  {#snippet body()}
    {#if data.config?.recipient_type}
      <div class="recipient-info">
        <span class="recipient-label">{t('actions.config.to')}:</span>
        <span class="recipient-value">{getRecipientLabel(data.config.recipient_type)}{#if data.config.recipient_type === 'specific' && specificCount > 0} ({specificCount}){/if}</span>
      </div>
      {#if data.config.message}
        <div class="message-preview">{data.config.message.substring(0, 40)}...</div>
      {/if}
    {:else}
      <div class="placeholder">{t('actions.config.selectRecipient')}</div>
    {/if}
  {/snippet}
</BaseActionNode>

<style>
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
</style>
