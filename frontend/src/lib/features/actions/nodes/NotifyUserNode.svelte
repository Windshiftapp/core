<script>
  import { Bell } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();

  function getRecipientLabel(recipientType) {
    const labels = {
      'assignee': t('actions.recipients.assignee'),
      'creator': t('actions.recipients.creator'),
      'specific': t('actions.recipients.specific')
    };
    return labels[recipientType] || recipientType;
  }
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={Bell} title={t('actions.nodes.notifyUser')} accentColor="magenta">
  {#snippet body()}
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
  {/snippet}
</GenericActionNode>

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
