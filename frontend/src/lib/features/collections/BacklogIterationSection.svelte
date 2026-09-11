<script>
  import { ChevronRight, ChevronDown, GripVertical, Play, CheckCircle, X } from '@lucide/svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import WorkItemRow from '../items/WorkItemRow.svelte';
  import DropIndicator from '../../layout/DropIndicator.svelte';
  import LazyRender from '../../components/LazyRender.svelte';
  import BacklogItemActions from './BacklogItemActions.svelte';

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
    assignableIterations = [],
    pendingActionItemIds = new Set(),
    onToggleCollapse,
    onOpenItem,
    onMoveItemToBoundary = null,
    onAssignItemToIteration = null,
    onStartIteration = null,
    onCompleteIteration = null,
    onRemoveGlobal = null,
    storyPointsConfiguredForItem = null,
    storyPointsPendingItemIds = new Set(),
    onUpdateStoryPoints = null,
    itemDndAction = () => {},
    sectionDndAction = () => {},
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
    `w-full flex items-center gap-2 px-3 py-2 rounded-lg transition-colors select-none iteration-header` +
    (sectionHighlight ? ' iteration-header-highlight' : '')
  );

  let dropZoneClass = $derived(
    `flex items-center justify-center py-6 px-4 rounded-lg border-2 border-dashed transition-colors iteration-drop-zone` +
    (sectionHighlight ? ' iteration-drop-zone-highlight' : '')
  );
</script>

<div
  class="mb-4"
  data-testid={`backlog-iteration-section-${sectionId}`}
  data-iteration-section
  data-iteration-id={sectionId}
>
  <!-- Section Header -->
  <div
    use:sectionDndAction={sectionId}
    role="button"
    tabindex="0"
    class={headerClass}
    onclick={() => onToggleCollapse?.(sectionId)}
    onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onToggleCollapse?.(sectionId); } }}
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
        type="button"
        class="ml-2 px-2 py-0.5 text-xs font-medium rounded border transition-colors iteration-action-btn iteration-action-start"
        onclick={(e) => { e.stopPropagation(); onStartIteration?.(iteration); }}
        title={t('iterations.startIteration')}
      >
        <span class="inline-flex items-center gap-1">
          <Play class="w-3 h-3" />
          {t('iterations.start')}
        </span>
      </button>
    {/if}

    {#if canComplete}
      <button
        type="button"
        class="ml-2 px-2 py-0.5 text-xs font-medium rounded border transition-colors iteration-action-btn iteration-action-complete"
        onclick={(e) => { e.stopPropagation(); onCompleteIteration?.(iteration); }}
        title={t('iterations.completeIteration')}
      >
        <span class="inline-flex items-center gap-1">
          <CheckCircle class="w-3 h-3" />
          {t('iterations.complete')}
        </span>
      </button>
    {/if}

    {#if isGlobalAdded}
      <button
        type="button"
        class="ml-2 p-0.5 rounded hover:bg-black/10 dark:hover:bg-white/10 transition-colors"
        onclick={(e) => { e.stopPropagation(); onRemoveGlobal?.(iteration); }}
        title={t('common.remove')}
      >
        <X class="w-3.5 h-3.5" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));" />
      </button>
    {/if}
  </div>

  <!-- Section Body -->
  {#if !collapsed}
    <div class="mt-1">
      {#if items.length === 0}
        <div
          use:sectionDndAction={sectionId}
          class={dropZoneClass}
          style="color: var(--ctx-text-subtle, var(--ds-text-subtle));"
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
              use:itemDndAction={item.id}
              class="relative"
              data-testid="backlog-item"
              data-item-card
              data-item-id={item.id}
              data-section-id={sectionId}
            >
              <LazyRender height={36}>
                {#snippet children()}
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
                    showStoryPoints={storyPointsConfiguredForItem?.(item) ?? false}
                    storyPointsSaving={storyPointsPendingItemIds.has(item.id)}
                    onStoryPointsChange={(value) => onUpdateStoryPoints?.(item, value)}
                  >
                    {#snippet leading()}
                      <div class="cursor-grab active:cursor-grabbing" style="{styles.dragHandleStyle}">
                        <GripVertical class="w-4 h-4" />
                      </div>
                    {/snippet}
                    {#snippet trailing()}
                      <BacklogItemActions
                        {item}
                        iterations={assignableIterations}
                        disabled={pendingActionItemIds.has(item.id)}
                        onMoveToBoundary={onMoveItemToBoundary}
                        onAssignIteration={onAssignItemToIteration}
                      />
                    {/snippet}
                  </WorkItemRow>
                {/snippet}
              </LazyRender>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .iteration-header:hover {
    background-color: rgba(0, 0, 0, 0.05);
  }
  :global(.dark) .iteration-header:hover {
    background-color: rgba(255, 255, 255, 0.05);
  }

  .iteration-header-highlight {
    background-color: var(--ctx-active-bg, rgba(59, 130, 246, 0.1));
    box-shadow: 0 0 0 2px var(--ctx-border-focused, rgb(96, 165, 250));
  }

  .iteration-drop-zone-highlight {
    border-color: var(--ctx-border-focused, rgb(96, 165, 250));
    background-color: var(--ctx-active-bg, rgba(59, 130, 246, 0.05));
  }

  .iteration-action-btn {
    border-color: var(--ctx-border-focused, currentColor);
    color: var(--ctx-text-interactive, currentColor);
    background-color: transparent;
  }
  .iteration-action-start {
    border-color: var(--ctx-border-focused, rgb(96, 165, 250));
    color: var(--ctx-text-interactive, rgb(59, 130, 246));
  }
  .iteration-action-start:hover {
    background-color: var(--ctx-active-bg, rgba(59, 130, 246, 0.05));
  }
  .iteration-action-complete {
    border-color: var(--ctx-border-focused, rgb(74, 222, 128));
    color: var(--ctx-text-interactive, rgb(34, 197, 94));
  }
  .iteration-action-complete:hover {
    background-color: var(--ctx-active-bg, rgba(34, 197, 94, 0.05));
  }
</style>
