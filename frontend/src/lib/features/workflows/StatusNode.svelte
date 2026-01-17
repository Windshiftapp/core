<script>
  import { Handle } from '@xyflow/svelte';
  import { X } from 'lucide-svelte';
  import { createEventDispatcher } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';

  export let data;
  export let selected = false;
  export let dragging = false;
  export let xPos;
  export let yPos;

  const dispatch = createEventDispatcher();

  function handleRemove() {
    dispatch('remove', { statusId: data.statusId });
  }

  function handleSetInitial(event) {
    event.stopPropagation();
    const statusId = data.statusId;
    if (!statusId) return;
    window.dispatchEvent(new CustomEvent('workflow-set-initial', { detail: { statusId } }));
  }
</script>

<div 
  class="status-node rounded border-2 shadow-lg select-none transition-all duration-200 group relative"
  class:border-blue-500={selected}
  class:border-gray-300={!selected}
  style="width: 100px; height: 32px;"
>
  <!-- Source handles (visible) - for initiating connections -->
  <Handle type="source" position="top" id="top" class="handle-source" />
  <Handle type="source" position="right" id="right" class="handle-source" />
  <Handle type="source" position="bottom" id="bottom" class="handle-source" />
  <Handle type="source" position="left" id="left" class="handle-source" />

  <!-- Target handles (invisible) - for receiving connections -->
  <Handle type="target" position="top" id="target-top" class="handle-target" />
  <Handle type="target" position="right" id="target-right" class="handle-target" />
  <Handle type="target" position="bottom" id="target-bottom" class="handle-target" />
  <Handle type="target" position="left" id="target-left" class="handle-target" />

  <div class="p-1 h-full flex flex-col justify-center relative">
    <!-- Initial marker / button -->
    <button
      class="initial-chip"
      class:initial-active={data.initial}
      on:click|stopPropagation={handleSetInitial}
      title={data.initial ? t('workflows.initialStatus') : t('workflows.setAsInitialStatus')}
    >
      {#if data.initial}
        {t('workflows.initial')}
      {:else}
        {t('workflows.setStart')}
      {/if}
    </button>

    <!-- Remove button - positioned in top-right corner -->
    <button
      class="absolute top-1 right-1 opacity-0 group-hover:opacity-100 text-red-500 hover:text-red-700 p-1 transition-opacity duration-200 z-10"
      on:click|stopPropagation={handleRemove}
      title={t('workflows.removeFromWorkflow')}
    >
      <X class="w-3 h-3" />
    </button>
    
    <!-- Status content -->
    <div class="flex items-center justify-center gap-1">
      <div 
        class="w-2 h-2 rounded border flex-shrink-0 status-dot"
        style="background-color: {data.category_color};"
      ></div>
      <div class="font-medium text-xs truncate status-label" title={data.name} style="font-size: 8px;">
        {data.name}
      </div>
    </div>
  </div>
</div>

<style>
  .status-node {
    background-color: var(--workflow-panel);
    border-color: var(--workflow-border);
    color: var(--workflow-text);
  }

  .status-node:hover {
    box-shadow: 0 10px 25px rgba(0, 0, 0, 0.15);
  }

  .status-label {
    color: var(--workflow-text);
  }

  .status-dot {
    border-color: var(--workflow-border);
  }

  .initial-chip {
    position: absolute;
    top: -12px;
    left: -8px;
    font-size: 9px;
    line-height: 1;
    padding: 2px 6px;
    border-radius: 999px;
    border: 1px solid var(--workflow-border);
    background: var(--workflow-panel);
    color: var(--workflow-text-subtle);
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.15s ease, background-color 0.15s ease, color 0.15s ease, border-color 0.15s ease;
    pointer-events: auto;
    z-index: 12;
  }

  .status-node:hover .initial-chip {
    opacity: 1;
  }

  .initial-chip:hover {
    background: var(--workflow-panel-hover);
    color: var(--workflow-accent);
    border-color: var(--workflow-accent);
  }

  .initial-active {
    opacity: 1 !important;
    background: rgba(59, 130, 246, 0.12);
    color: var(--workflow-accent);
    border-color: var(--workflow-accent);
  }

  /* Source handles - visible, higher z-index to capture clicks first */
  :global(.handle-source) {
    width: 12px !important;
    height: 12px !important;
    background: var(--workflow-accent) !important;
    border: 2px solid var(--workflow-panel) !important;
    opacity: 0.3 !important;
    transition: opacity 0.2s ease !important;
    pointer-events: auto !important;
    z-index: 10 !important;
  }

  .status-node:hover :global(.handle-source) {
    opacity: 1 !important;
  }

  :global(.handle-source:hover) {
    background: var(--workflow-accent-strong) !important;
    transform: scale(1.2) !important;
  }

  /* Target handles - invisible but functional, lower z-index */
  :global(.handle-target) {
    width: 12px !important;
    height: 12px !important;
    background: transparent !important;
    border: none !important;
    opacity: 0 !important;
    pointer-events: auto !important;
    z-index: 5 !important;
  }
</style>
