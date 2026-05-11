<script>
  import { MessageSquare } from '@lucide/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import Badge from '../../../components/Badge.svelte';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();

  function truncateContent(content, maxLength = 50) {
    if (!content) return '';
    return content.length > maxLength ? content.substring(0, maxLength) + '...' : content;
  }
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={MessageSquare} title={t('actions.nodes.addComment')} accentColor="orange">
  {#snippet body()}
    {#if data.config?.content}
      <div class="comment-preview">
        {#if data.config.is_private}
          <Badge variant="warning" size="xs">{t('actions.config.private')}</Badge>
        {/if}
        <span class="comment-text">{truncateContent(data.config.content)}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.enterComment')}</div>
    {/if}
  {/snippet}
</GenericActionNode>

<style>
  .comment-preview {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .comment-text {
    font-size: 12px;
    color: var(--ds-text-subtle);
    line-height: 1.4;
  }
</style>
