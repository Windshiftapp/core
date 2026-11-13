<script>
  import GripVertical from '@lucide/svelte/icons/grip-vertical';
  import LazyRender from '../../components/LazyRender.svelte';
  import DropIndicator from '../../layout/DropIndicator.svelte';
  import WorkItemRow from '../items/WorkItemRow.svelte';
  import BacklogItemActions from './BacklogItemActions.svelte';

  let {
    item, sectionId, workspace, itemTypes, statuses, statusCategories, styles, dragState,
    backlogRowGap = 2, assignableIterations = [], pendingActionItemIds = new Set(),
    visibleFields = [], onOpenItem, onMoveItemToBoundary, onAssignItemToIteration,
    storyPointsConfiguredForItem = null, storyPointsPendingItemIds = new Set(),
    onUpdateStoryPoints, itemDndAction = () => {}, deferred = true, virtualRow = null,
  } = $props();
  const visibleFieldSet = $derived(new Set(visibleFields));
  function virtualAttachment(node) { return virtualRow?.attach(node); }
</script>

{#snippet content()}
  {#if dragState.get(item.id)?.closestEdge}
    <DropIndicator edge={dragState.get(item.id)?.closestEdge} gap={backlogRowGap} />
  {/if}
  <WorkItemRow {item} {workspace} {itemTypes} {statuses} {statusCategories}
    dataTestid={`backlog-row-${item.id}`}
    onclick={(event) => onOpenItem?.(item.id, event)}
    showStatus={visibleFieldSet.has('status')} showPriority={visibleFieldSet.has('priority')}
    showParent={visibleFieldSet.has('parent')} showAssignee={visibleFieldSet.has('assignee')}
    showDueDate={visibleFieldSet.has('due_date')} showMilestones={visibleFieldSet.has('milestone')}
    showStoryPoints={visibleFieldSet.has('story_points') && (storyPointsConfiguredForItem?.(item) ?? false)}
    storyPointsSaving={storyPointsPendingItemIds.has(item.id)}
    onStoryPointsChange={(value) => onUpdateStoryPoints?.(item, value)}
  >
    {#snippet leading()}
      <div data-testid={`backlog-drag-${item.id}`} class="cursor-grab active:cursor-grabbing" style={styles.dragHandleStyle}>
        <GripVertical class="w-4 h-4" />
      </div>
    {/snippet}
    {#snippet trailing()}
      <BacklogItemActions {item} iterations={assignableIterations}
        disabled={pendingActionItemIds.has(item.id)}
        onMoveToBoundary={onMoveItemToBoundary} onAssignIteration={onAssignItemToIteration} />
    {/snippet}
  </WorkItemRow>
{/snippet}

<div use:virtualAttachment use:itemDndAction={item.id} class="relative"
  style={`padding-bottom: ${backlogRowGap}px;`}
  data-testid="backlog-item" data-item-card data-item-id={item.id} data-section-id={sectionId}>
  {#if deferred}
    <LazyRender height={36}>{@render content()}</LazyRender>
  {:else}
    {@render content()}
  {/if}
</div>
