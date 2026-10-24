<script>
  import Send from '@lucide/svelte/icons/send';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={Send} title={t('actions.nodes.notifyCustomer')} accentColor="magenta">
  {#snippet body()}
    {#if data.config?.message}
      <div class="message-preview">{data.config.message.substring(0, 40)}...</div>
    {:else}
      <div class="placeholder">{t('actions.config.enterMessage')}</div>
    {/if}
  {/snippet}
</BaseActionNode>

<style>
  .message-preview {
    font-size: 11px;
    color: var(--ds-text-subtle);
    font-style: italic;
  }
</style>
