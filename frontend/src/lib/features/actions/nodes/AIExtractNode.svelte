<script>
  import { Sparkles } from '@lucide/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={Sparkles} title={t('actions.nodes.aiExtract')} accentColor="purple">
  {#snippet body()}
    {#if data.config?.input_field && data.config?.output_field}
      <div class="ai-info">
        <span class="ai-label">{data.config.input_field}</span>
        <span class="ai-arrow">&rarr;</span>
        <span class="ai-label">{data.config.output_field}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.configureExtract')}</div>
    {/if}
  {/snippet}
</GenericActionNode>

<style>
  .ai-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }
  .ai-label {
    color: var(--ds-text);
    font-family: monospace;
    font-size: 11px;
    background: var(--ds-surface-sunken);
    padding: 1px 5px;
    border-radius: 3px;
  }
  .ai-arrow { color: var(--ds-text-subtlest); }
  .placeholder { color: var(--ds-text-subtle); font-size: 12px; font-style: italic; }
</style>
