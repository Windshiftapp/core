<script>
  import FileText from '@lucide/svelte/icons/file-text';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={FileText} title={t('actions.nodes.insertCannedResponse')} accentColor="orange">
  {#snippet body()}
    {#if data.config?.canned_response_id}
      <div class="cr-info">
        <span class="cr-label">{t('actions.config.cannedResponse')}:</span>
        <span class="cr-value">#{data.config.canned_response_id}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.selectCannedResponse')}</div>
    {/if}
  {/snippet}
</BaseActionNode>

<style>
  .cr-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }

  .cr-label {
    color: var(--ds-text-subtlest);
  }

  .cr-value {
    color: var(--ds-text);
    font-family: monospace;
    font-size: 11px;
  }
</style>
