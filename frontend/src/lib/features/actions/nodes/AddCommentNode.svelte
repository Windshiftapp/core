<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { MessageSquare } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';

  export let data = {};
  export let selected = false;

  function truncateContent(content, maxLength = 50) {
    if (!content) return '';
    return content.length > maxLength ? content.substring(0, maxLength) + '...' : content;
  }
</script>

<div class="add-comment-node" class:selected>
  <Handle type="target" position={Position.Left} id="input" />

  <div class="node-header">
    <MessageSquare size={16} class="node-icon" />
    <span class="node-title">{t('actions.nodes.addComment')}</span>
  </div>
  <div class="node-body">
    {#if data.config?.content}
      <div class="comment-preview">
        {#if data.config.is_private}
          <span class="private-badge">{t('actions.config.private')}</span>
        {/if}
        <span class="comment-text">{truncateContent(data.config.content)}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.enterComment')}</div>
    {/if}
  </div>

  <Handle type="source" position={Position.Right} id="output" />
</div>

<style>
  .add-comment-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--ds-accent-orange);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .add-comment-node.selected {
    box-shadow: 0 0 0 2px var(--ds-interactive), 0 0 12px rgba(59, 130, 246, 0.5);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--ds-accent-orange-subtle);
    border-bottom: 1px solid var(--ds-accent-orange-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-icon {
    flex-shrink: 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-accent-orange);
  }

  .node-body {
    padding: 10px 12px;
  }

  .comment-preview {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .private-badge {
    display: inline-block;
    font-size: 10px;
    padding: 2px 6px;
    background-color: var(--ds-warning-subtle);
    color: var(--ds-warning);
    border-radius: 4px;
    width: fit-content;
  }

  .comment-text {
    font-size: 12px;
    color: var(--ds-text-subtle);
    line-height: 1.4;
  }

  .placeholder {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }

  :global(.add-comment-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-orange);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
