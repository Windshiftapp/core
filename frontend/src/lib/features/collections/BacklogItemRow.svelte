<script>
  import GripVertical from '@lucide/svelte/icons/grip-vertical';
  import CheckSquare from '@lucide/svelte/icons/square-check';
  import Square from '@lucide/svelte/icons/square';
  import LazyRender from '../../components/LazyRender.svelte';
  import DropIndicator from '../../layout/DropIndicator.svelte';
  import WorkItemRow from '../items/WorkItemRow.svelte';
  import BacklogItemActions from './BacklogItemActions.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    item, sectionId, workspace, itemTypes, statuses, statusCategories, styles, dragState,
    backlogRowGap = 2, assignableIterations = [], pendingActionItemIds = new Set(),
    visibleFields = [], onOpenItem, onMoveItemToBoundary, onAssignItemToIteration,
    storyPointsConfiguredForItem = null, storyPointsPendingItemIds = new Set(),
    onUpdateStoryPoints, itemDndAction = () => {}, deferred = true, virtualRow = null,
    selectionMode = false, selected = false, onSelect = null, onShiftSelect = null,
  } = $props();
  const visibleFieldSet = $derived(new Set(visibleFields));
  function virtualAttachment(node) { return virtualRow?.attach(node); }

  // In selection mode the whole row toggles its selection; otherwise
  // shift-click extends the backlog selection and a plain click opens the item.
  // Mirrors BoardItemCard and WorkItemListRow.
  function handleRowClick(event) {
    if (selectionMode && onSelect) {
      event.preventDefault();
      onSelect(item.id, event);
      return;
    }
    if (event.shiftKey && onShiftSelect) {
      event.preventDefault();
      onShiftSelect(item.id, event);
      return;
    }
    onOpenItem?.(item.id, event);
  }
</script>

{#snippet content()}
  {#if dragState.get(item.id)?.closestEdge}
    <DropIndicator edge={dragState.get(item.id)?.closestEdge} gap={backlogRowGap} />
  {/if}
  <WorkItemRow {item} {workspace} {itemTypes} {statuses} {statusCategories}
    dataTestid={`backlog-row-${item.id}`}
    onclick={handleRowClick}
    showStatus={visibleFieldSet.has('status')} showPriority={visibleFieldSet.has('priority')}
    showParent={visibleFieldSet.has('parent')} showAssignee={visibleFieldSet.has('assignee')}
    showDueDate={visibleFieldSet.has('due_date')} showMilestones={visibleFieldSet.has('milestone')}
    showStoryPoints={visibleFieldSet.has('story_points') && (storyPointsConfiguredForItem?.(item) ?? false)}
    storyPointsSaving={storyPointsPendingItemIds.has(item.id)}
    onStoryPointsChange={(value) => onUpdateStoryPoints?.(item, value)}
  >
    {#snippet leading()}
      {#if selectionMode}
        <!-- inline-style-guard-exempt: selection checkbox mirrors the board card's dynamic accent. -->
        <button
          type="button"
          class="-ml-0.5 shrink-0 rounded p-0.5"
          style="color: {selected ? 'var(--ds-border-focused)' : 'var(--ds-icon-subtle)'};"
          data-testid={`backlog-row-select-${item.id}`}
          aria-pressed={selected}
          aria-label={t('collections.toggleSelection')}
          title={t('collections.toggleSelection')}
          onclick={(event) => {
            event.stopPropagation();
            onSelect?.(item.id, event);
          }}
          onkeydown={(event) => event.stopPropagation()}
        >
          {#if selected}
            <CheckSquare class="h-4 w-4" />
          {:else}
            <Square class="h-4 w-4" />
          {/if}
        </button>
      {:else}
        <div data-testid={`backlog-drag-${item.id}`} class="cursor-grab active:cursor-grabbing" style={styles.dragHandleStyle}>
          <GripVertical class="w-4 h-4" />
        </div>
      {/if}
    {/snippet}
    {#snippet trailing()}
      <BacklogItemActions {item} iterations={assignableIterations}
        disabled={pendingActionItemIds.has(item.id)}
        onMoveToBoundary={onMoveItemToBoundary} onAssignIteration={onAssignItemToIteration} />
    {/snippet}
  </WorkItemRow>
{/snippet}

<div use:virtualAttachment use:itemDndAction={item.id} class="relative backlog-item"
  style={`padding-bottom: ${backlogRowGap}px;`}
  data-testid="backlog-item" data-item-card data-item-id={item.id} data-section-id={sectionId}
  data-selected={selected ? 'true' : 'false'}
  data-selection-mode={selectionMode ? 'true' : 'false'}
>
  {#if deferred}
    <LazyRender height={36}>{@render content()}</LazyRender>
  {:else}
    {@render content()}
  {/if}
</div>

<style>
  /* Selection mode dims every row so the selected subset stands out. The
     selected row stays fully opaque and carries the focus accent. */
  .backlog-item[data-selection-mode='true']:not([data-selected='true']) :global(.item-card) {
    opacity: 0.5;
  }

  .backlog-item[data-selection-mode='true']:not([data-selected='true']):hover :global(.item-card) {
    opacity: 0.8;
  }

  .backlog-item[data-selected='true'] :global(.item-card) {
    border-color: var(--ds-border-focused) !important;
    box-shadow: 0 0 0 1px var(--ds-border-focused);
  }
</style>
