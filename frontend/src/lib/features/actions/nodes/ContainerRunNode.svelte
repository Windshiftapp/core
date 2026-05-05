<script>
  import { Box } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={Box} title={t('actions.nodes.containerRun')} accentColor="blue">
  {#snippet body()}
    {#if data.config?.capability_id}
      <div class="cap-info">
        <span class="cap-label">{t('actions.config.capability')}:</span>
        <span class="cap-value">#{data.config.capability_id}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.selectCapability')}</div>
    {/if}
  {/snippet}
</GenericActionNode>

<style>
  .cap-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }
  .cap-label { color: var(--ds-text-subtlest); }
  .cap-value { color: var(--ds-text); font-family: monospace; font-size: 11px; }
  .placeholder { color: var(--ds-text-subtle); font-size: 12px; font-style: italic; }
</style>
