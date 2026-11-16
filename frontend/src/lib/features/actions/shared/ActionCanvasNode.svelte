<script>
  import { Handle, useUpdateNodeInternals } from '@xyflow/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import Tooltip from '../../../components/Tooltip.svelte';
  import { getHandlePositions, getConditionOutputPositions } from '../nodes/flowDirection.js';
  import { nodePresentation } from './actionNodeDefinitions.js';
  import { buildNodePreview, PREVIEW_CARD_WIDTH, PREVIEW_VALUE_LINES } from './actionNodePreview.js';
  import { useActionEditorContext } from './actionEditorContext.js';

  let { id, type, data = {}, selected = false } = $props();
  const editor = useActionEditorContext();
  const updateNodeInternals = useUpdateNodeInternals();
  let editorData = $derived(editor.getEditorData());
  let direction = $derived(editor.flowStore.direction);
  let presentation = $derived(nodePresentation(type, t, editorData.nodeLabels?.[type] || type));
  let Icon = $derived(presentation.icon);
  let preview = $derived(buildNodePreview({ type, config: data.config, triggerType: data.triggerType }, editorData, t));
  let positions = $derived(getHandlePositions(direction));
  let conditionPositions = $derived(getConditionOutputPositions(direction));

  $effect(() => {
    direction;
    preview.rows;
    updateNodeInternals(id);
  });
</script>

<div
  class="action-canvas-node action-flow-node"
  class:selected
  data-testid={`action-node-${type}`}
  style={`--_accent: var(--ds-accent-${presentation.accent}); --_accent-subtle: var(--ds-accent-${presentation.accent}-subtle); --_accent-subtler: var(--ds-accent-${presentation.accent}-subtler, var(--ds-border)); --_card-width: ${PREVIEW_CARD_WIDTH}px; --_value-lines: ${PREVIEW_VALUE_LINES};`}
>
  {#if presentation.layout === 'step' || presentation.layout === 'condition'}
    <Handle type="target" position={positions.input} id="input" data-testid="action-node-handle-input" />
  {/if}
  <div class="node-header" data-testid="action-node-header">
    <Icon size={16} />
    <span>{presentation.label}</span>
  </div>
  <dl class="node-body">
    {#each preview.rows as row (row.key)}
      <div class="preview-row" data-testid={`action-node-row-${row.key}`}>
        <dt>{row.label}:</dt>
        <dd>
          <Tooltip content={row.detail ?? row.value} contentClass="px-3 py-2 text-xs max-w-sm whitespace-pre-wrap break-words">
            <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard focus exposes full preview text without adding an editing control.) -->
            <span class="preview-value nodrag nopan" tabindex="0" data-testid={`action-node-value-${row.key}`}>{row.value}</span>
          </Tooltip>
        </dd>
      </div>
    {/each}
  </dl>
  {#if presentation.layout === 'condition'}
    <div class="branch-labels" class:vertical={direction === 'vertical'}>
      <span class="branch-true" style={conditionPositions.trueStyle} data-testid="action-node-branch-label-true">{t('actions.condition.true')}</span>
      <span class="branch-false" style={conditionPositions.falseStyle} data-testid="action-node-branch-label-false">{t('actions.condition.false')}</span>
    </div>
    <Handle type="source" position={conditionPositions.trueOutput} id="true" style={conditionPositions.trueStyle} data-testid="action-node-handle-true" />
    <Handle type="source" position={conditionPositions.falseOutput} id="false" style={conditionPositions.falseStyle} data-testid="action-node-handle-false" />
  {:else if presentation.layout === 'step' || presentation.layout === 'trigger'}
    <Handle type="source" position={positions.output} id="output" data-testid="action-node-handle-output" />
  {/if}
</div>

<style>
  .action-canvas-node {
    position: relative;
    width: var(--_card-width);
    background: var(--ds-surface-raised);
    border: 2px solid var(--_accent);
    border-radius: 8px;
    box-shadow: var(--shadow-md);
  }
  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background: var(--_accent-subtle);
    border-bottom: 1px solid var(--_accent-subtler);
    border-radius: 6px 6px 0 0;
    color: var(--_accent);
    font-size: 12px;
    font-weight: 600;
  }
  .node-body { margin: 0; padding: 10px 12px; display: grid; gap: 6px; }
  .preview-row { font-size: 12px; line-height: 1.4; }
  dt { color: var(--ds-text-subtle); }
  dd { margin: 0; min-width: 0; color: var(--ds-text); }
  .preview-value {
    display: -webkit-box;
    -webkit-line-clamp: var(--_value-lines);
    line-clamp: var(--_value-lines);
    -webkit-box-orient: vertical;
    overflow: hidden;
    overflow-wrap: anywhere;
    border-radius: 2px;
  }
  .preview-value:focus-visible { outline: 2px solid var(--ds-border-focused); outline-offset: 2px; }
  .branch-labels { position: absolute; inset: 0; pointer-events: none; font-size: 10px; font-weight: 500; }
  .branch-labels span { position: absolute; left: calc(100% + 12px); transform: translateY(-50%); white-space: nowrap; }
  .branch-labels.vertical span { top: calc(100% + 12px); transform: translateX(-50%); }
  .branch-true { color: var(--ds-success); }
  .branch-false { color: var(--ds-danger); }
  :global(.action-canvas-node .svelte-flow__handle) {
    width: 10px; height: 10px; background: var(--_accent); border: 2px solid var(--ds-surface-raised);
  }
  :global(.action-canvas-node .svelte-flow__handle[data-handleid="true"]) { background: var(--ds-success); }
  :global(.action-canvas-node .svelte-flow__handle[data-handleid="false"]) { background: var(--ds-danger); }
</style>
