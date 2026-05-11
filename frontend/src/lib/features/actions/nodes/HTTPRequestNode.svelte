<script>
  import { Globe } from '@lucide/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={Globe} title={t('actions.nodes.httpRequest')} accentColor="cyan">
  {#snippet body()}
    {#if data.config?.url_template}
      <div class="http-info">
        <span class="method">{data.config.method || 'GET'}</span>
        <span class="url">{data.config.url_template}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.configureRequest')}</div>
    {/if}
  {/snippet}
</GenericActionNode>

<style>
  .http-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
  }
  .method {
    color: var(--ds-text);
    font-family: monospace;
    font-weight: 600;
    background: var(--ds-surface-sunken);
    padding: 1px 5px;
    border-radius: 3px;
  }
  .url {
    color: var(--ds-text-subtle);
    font-family: monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 140px;
  }
  .placeholder { color: var(--ds-text-subtle); font-size: 12px; font-style: italic; }
</style>
