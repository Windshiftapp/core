<script>
  import { ChevronRight, ChevronDown, GripVertical, Play, CheckCircle, X } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import WorkItemRow from '../items/WorkItemRow.svelte';
  import DropIndicator from '../../layout/DropIndicator.svelte';

  let {
    iteration = null,
    items = [],
    collapsed = false,
    workspace,
    itemTypes,
    statuses,
    statusCategories,
    styles,
    dragState,
    backlogRowGap = 2,
    isGlobalAdded = false,
    sectionHighlight = false,
    onToggleCollapse,
    onOpenItem,
    onStartSprint,
    onCompleteSprint,
    onRemoveGlobal,
  } = $props();

  const statusColors = {
    planned: 'grey',
    active: 'blue',
    completed: 'green',
    cancelled: 'red',
  };

  let sectionName = $derived(iteration ? iteration.name : t('collections.backlog'));
  let lozengeColor = $derived(iteration ? (statusColors[iteration.status] || 'grey') : null);
  let dateRange = $derived.by(() => {
    if (!iteration) return null;
    const parts = [];
    if (iteration.start_date) parts.push(formatDateShort(iteration.start_date));
    if (iteration.end_date) parts.push(formatDateShort(iteration.end_date));
    return parts.length > 0 ? parts.join(' - ') : null;
  });
  let canStart = $derived(iteration && !iteration.is_global && iteration.status === 'planned');
  let canComplete = $derived(iteration && !iteration.is_global && iteration.status === 'active');
  let sectionId = $derived(iteration ? iteration.id : 'unassigned');

  let headerClass = $derived(
    `w-full flex items-center gap-2 px-3 py-2 rounded-lg transition-colors select-none sprint-header` +
    (sectionHighlight ? ' sprint-header-highlight' : '')
  );

  let dropZoneClass = $derived(
    `flex items-center justify-center py-6 px-4 rounded-lg border-2 border-dashed transition-colors sprint-drop-zone` +
    (sectionHighlight ? ' sprint-drop-zone-highlight' : '')
  );
</script>

<div
  class="mb-4"
  data-iteration-section
  data-iteration-id={sectionId}
>
  <!-- Section Header -->
  <button
    class={headerClass}
    onclick={() => onToggleCollapse?.(sectionId)}
    data-section-header
    data-iteration-id={sectionId}
  >
    <!-- Collapse chevron -->
    <span class="flex-shrink-0" style="{styles.subtleTextStyle}">
      {#if collapsed}
        <ChevronRight class="w-4 h-4" />
      {:else}
        <ChevronDown class="w-4 h-4" />
      {/if}
    </span>

    <!-- Section name -->
    <span class="font-semibold text-sm" style="color: var(--ctx-text, var(--ds-text));">
      {sectionName}
    </span>

    <!-- Status lozenge -->
    {#if iteration && lozengeColor}
      <Lozenge color={lozengeColor} text={iteration.status} onGradient={styles.hasCustomBackground} />
    {/if}

    <!-- Date range -->
    {#if dateRange}
      <span class="text-xs" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
        {dateRange}
      </span>
    {/if}

    <!-- Item count -->
    <span class="text-xs tabular-nums ml-auto" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
      {items.length} {items.length === 1 ? t('common.item') : t('common.items')}
    </span>

    <!-- Action buttons -->
    {#if canStart}
      <button
        class="ml-2 px-2 py-0.5 text-xs font-medium rounded border transition-colors sprint-action-btn sprint-action-start"
        onclick={(e) => { e.stopPropagation(); onStartSprint?.(iteration); }}
        title={t('iterations.startSprint')}
      >
        <span class="inline-flex items-center gap-1">
          <Play class="w-3 h-3" />
          {t('iterations.start')}
        </span>
      </button>
    {/if}

    {#if canComplete}
      <button
        class="ml-2 px-2 py-0.5 text-xs font-medium rounded border transition-colors sprint-action-btn sprint-action-complete"
        onclick={(e) => { e.stopPropagation(); onCompleteSprint?.(iteration); }}
        title={t('iterations.completeSprint')}
      >
        <span class="inline-flex items-center gap-1">
          <CheckCircle class="w-3 h-3" />
          {t('iterations.complete')}
        </span>
      </button>
    {/if}

    {#if isGlobalAdded}
      <button
        class="ml-2 p-0.5 rounded hover:bg-black/10 dark:hover:bg-white/10 transition-colors"
        onclick={(e) => { e.stopPropagation(); onRemoveGlobal?.(iteration); }}
        title={t('common.remove')}
      >
        <X class="w-3.5 h-3.5" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));" />
      </button>
    {/if}
  </button>

  <!-- Section Body -->
  {#if !collapsed}
    <div class="mt-1">
      {#if items.length === 0}
        <div
          class={dropZoneClass}
          style="border-color: var(--ds-border, #e5e7eb); color: var(--ds-text-subtlest, #9ca3af);"
          data-section-drop-zone
          data-iteration-id={sectionId}
        >
          <span class="text-sm">
            {t('collections.dragItemsHere')}
          </span>
        </div>
      {:else}
        <div class="flex flex-col" style={`row-gap: ${backlogRowGap}px;`}>
          {#each items as item (item.id)}
            <div
              class="relative"
              data-item-card
              data-item-id={item.id}
              data-section-id={sectionId}
            >
              {#if dragState.get(item.id)?.closestEdge}
                <DropIndicator edge={dragState.get(item.id)?.closestEdge} gap={backlogRowGap} />
              {/if}

              <WorkItemRow
                {item}
                {workspace}
                {itemTypes}
                {statuses}
                {statusCategories}
                onclick={(e) => onOpenItem?.(item.id, e)}
                showStatus={true}
              >
                {#snippet leading()}
                  <div class="cursor-grab active:cursor-grabbing" style="{styles.dragHandleStyle}">
                    <GripVertical class="w-4 h-4" />
                  </div>
                {/snippet}
              </WorkItemRow>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .sprint-header:hover {
    background-color: rgba(0, 0, 0, 0.05);
  }
  :global(.dark) .sprint-header:hover {
    background-color: rgba(255, 255, 255, 0.05);
  }

  .sprint-header-highlight {
    background-color: var(--ctx-active-bg, rgba(59, 130, 246, 0.1));
    ring: 2px;
    box-shadow: 0 0 0 2px var(--ctx-border-focused, rgb(96, 165, 250));
  }

  .sprint-drop-zone-highlight {
    border-color: var(--ctx-border-focused, rgb(96, 165, 250));
    background-color: var(--ctx-active-bg, rgba(59, 130, 246, 0.05));
  }

  .sprint-action-btn {
    border-color: var(--ctx-border-focused, currentColor);
    color: var(--ctx-text-interactive, currentColor);
    background-color: transparent;
  }
  .sprint-action-start {
    border-color: var(--ctx-border-focused, rgb(96, 165, 250));
    color: var(--ctx-text-interactive, rgb(59, 130, 246));
  }
  .sprint-action-start:hover {
    background-color: var(--ctx-active-bg, rgba(59, 130, 246, 0.05));
  }
  .sprint-action-complete {
    border-color: var(--ctx-border-focused, rgb(74, 222, 128));
    color: var(--ctx-text-interactive, rgb(34, 197, 94));
  }
  .sprint-action-complete:hover {
    background-color: var(--ctx-active-bg, rgba(34, 197, 94, 0.05));
  }
</style>
