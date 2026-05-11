<script>
  import { Bot } from '@lucide/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={Bot} title={t('actions.nodes.aiAgent')} accentColor="magenta">
  {#snippet body()}
    {#if data.config?.capability_id}
      <div class="agent-info">
        <span class="cap-label">{t('actions.config.model')}:</span>
        <span class="cap-value">#{data.config.capability_id}</span>
        <span class="tools-count">
          {(data.config.tools?.length || 0)} {t('actions.config.tools')}
        </span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.selectModelAndTools')}</div>
    {/if}
  {/snippet}
</GenericActionNode>

<style>
  .agent-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    flex-wrap: wrap;
  }
  .cap-label { color: var(--ds-text-subtlest); }
  .cap-value { color: var(--ds-text); font-family: monospace; font-size: 11px; }
  .tools-count {
    color: var(--ds-text-subtle);
    font-size: 11px;
    background: var(--ds-surface-sunken);
    padding: 1px 5px;
    border-radius: 3px;
  }
  .placeholder { color: var(--ds-text-subtle); font-size: 12px; font-style: italic; }
</style>
