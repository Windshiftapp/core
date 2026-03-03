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
    `w-full flex items-center gap-2 px-3 py-2 rounded-lg transition-colors select-none` +
    ` hover:bg-black/5 dark:hover:bg-white/5` +
    (sectionHighlight ? ' bg-blue-100 dark:bg-blue-900/30 ring-2 ring-blue-400' : '')
  );

  let dropZoneClass = $derived(
    `flex items-center justify-center py-6 px-4 rounded-lg border-2 border-dashed transition-colors` +
    (sectionHighlight ? ' border-blue-400 bg-blue-50 dark:bg-blue-900/10' : '')
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
    <span class="flex-shrink-0" style={styles.subtleTextStyle}>
      {#if collapsed}
        <ChevronRight class="w-4 h-4" />
      {:else}
        <ChevronDown class="w-4 h-4" />
      {/if}
    </span>

    <!-- Section name -->
    <span class="font-semibold text-sm" style={styles.textStyle}>
      {sectionName}
    </span>

    <!-- Status lozenge -->
    {#if iteration && lozengeColor}
      <Lozenge color={lozengeColor} text={iteration.status} />
    {/if}

    <!-- Date range -->
    {#if dateRange}
      <span class="text-xs" style={styles.subtleTextStyle}>
        {dateRange}
      </span>
    {/if}

    <!-- Item count -->
    <span class="text-xs tabular-nums ml-auto" style={styles.subtleTextStyle}>
      {items.length} {items.length === 1 ? t('common.item') : t('common.items')}
    </span>

    <!-- Action buttons -->
    {#if canStart}
      <button
        class="ml-2 px-2 py-0.5 text-xs font-medium rounded border border-blue-400 text-blue-500 hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-colors"
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
        class="ml-2 px-2 py-0.5 text-xs font-medium rounded border border-green-400 text-green-500 hover:bg-green-50 dark:hover:bg-green-900/20 transition-colors"
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
        <X class="w-3.5 h-3.5" style={styles.subtleTextStyle} />
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
                hasGradient={styles.hasCustomBackground}
              >
                {#snippet leading()}
                  <div class="cursor-grab active:cursor-grabbing" style={styles.dragHandleStyle}>
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
