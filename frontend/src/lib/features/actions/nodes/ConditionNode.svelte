<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { HelpCircle } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import { getConditionOutputPositions } from './flowDirection.js';

  let { data = {}, selected = false } = $props();

  let condPositions = $derived(getConditionOutputPositions(actionFlowStore.direction));
  let isVertical = $derived(actionFlowStore.direction === 'vertical');

  function getOperatorSymbol(operator) {
    const symbols = {
      'equals': '=',
      'not_equals': '≠',
      'contains': '∋',
      'not_contains': '∌',
      'greater_than': '>',
      'less_than': '<',
      'is_empty': '∅',
      'is_not_empty': '≠∅'
    };
    return symbols[operator] || operator;
  }
</script>

<div class="condition-node action-flow-node" class:selected>
  <Handle type="target" position={condPositions.input} id="input" />

  <div class="node-header">
    <HelpCircle size={16} class="node-icon" />
    <span class="node-title">{t('actions.nodes.condition')}</span>
  </div>
  <div class="node-body">
    {#if data.config?.field_name}
      <div class="condition-expr">
        <span class="field">{data.config.field_name}</span>
        <span class="operator">{getOperatorSymbol(data.config.operator)}</span>
        {#if data.config.value !== undefined && data.config.value !== ''}
          <span class="value">{data.config.value}</span>
        {/if}
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.setCondition')}</div>
    {/if}
  </div>

  <div class="branch-labels" class:branch-labels-vertical={isVertical}>
    <span class="branch-true">{t('actions.condition.true')}</span>
    <span class="branch-false">{t('actions.condition.false')}</span>
  </div>

  <Handle type="source" position={condPositions.trueOutput} id="true" style={condPositions.trueStyle} />
  <Handle type="source" position={condPositions.falseOutput} id="false" style={condPositions.falseStyle} />
</div>

<style>
  .condition-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--ds-accent-yellow);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--ds-accent-yellow-subtle);
    border-bottom: 1px solid var(--ds-accent-yellow-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-icon {
    flex-shrink: 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-accent-yellow);
  }

  .node-body {
    padding: 10px 12px;
  }

  .condition-expr {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    flex-wrap: wrap;
  }

  .field {
    color: var(--ds-text);
    font-weight: 500;
  }

  .operator {
    color: var(--ds-accent-yellow);
    font-weight: 600;
    font-size: 14px;
  }

  .value {
    color: var(--ds-text-subtle);
    font-family: monospace;
    font-size: 11px;
    background-color: var(--ds-surface-sunken);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .branch-labels {
    display: flex;
    flex-direction: column;
    position: absolute;
    right: 16px;
    top: 50%;
    transform: translateY(-50%);
    gap: 16px;
    font-size: 10px;
    font-weight: 500;
  }

  .branch-labels-vertical {
    flex-direction: row;
    right: auto;
    top: auto;
    bottom: 8px;
    left: 50%;
    transform: translateX(-50%);
    gap: 32px;
  }

  .branch-true {
    color: var(--ds-success);
  }

  .branch-false {
    color: var(--ds-danger);
  }

  .placeholder {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }

  :global(.condition-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-yellow);
    border: 2px solid var(--ds-surface-raised);
  }

  :global(.condition-node .svelte-flow__handle[data-handleid="true"]) {
    background-color: var(--ds-success);
  }

  :global(.condition-node .svelte-flow__handle[data-handleid="false"]) {
    background-color: var(--ds-danger);
  }
</style>
