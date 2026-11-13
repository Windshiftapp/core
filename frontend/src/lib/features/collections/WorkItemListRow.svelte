<script>
  import { draggable as makeDraggable } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import MoreHorizontal from '@lucide/svelte/icons/more-horizontal';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import LazyRender from '../../components/LazyRender.svelte';
  import ListCellRenderer from './ListCellRenderer.svelte';

  let {
    item,
    columns = [],
    gridTemplateColumns = '',
    gridMinWidth = '',
    workspace = null,
    statuses = [],
    statusCategories = [],
    priorities = [],
    milestones = [],
    iterations = [],
    users = [],
    projects = [],
    itemTypes = [],
    customFieldDefinitions = [],
    canEdit = false,
    actions = [],
    onitemUpdated = null,
    onupdateError = null,
    draggable = false,
    rowTestid = undefined,
    leading = undefined,
    deferred = true,
    virtualRow = null,
  } = $props();

  function virtualAttachment(node) {
    return virtualRow?.attach(node);
  }

  // Only the collection list wires drag sources; the queue has no drop
  // targets. The row itself is the draggable element either way.
  function dragRow(element) {
    if (!draggable) return;
    const cleanup = makeDraggable({
      element,
      getInitialData: () => ({ item, type: 'work-item' }),
      onDragStart: () => {
        element.style.opacity = '0.5';
      },
      onDrop: () => {
        element.style.opacity = '';
      },
    });
    return {
      destroy() {
        element.style.opacity = '';
        cleanup();
      },
    };
  }
</script>

{#snippet cells()}
      <div
        class="grid gap-4 items-center"
        style="grid-template-columns: {gridTemplateColumns}; min-width: {gridMinWidth};"
      >
        {#if leading}{@render leading(item)}{/if}
        {#each columns as column (column.field_identifier)}
          <div class="min-w-0 overflow-hidden">
            <ListCellRenderer
              {item} {column} {workspace} {canEdit} {statuses} {statusCategories}
              {priorities} {milestones} {iterations} {users} {projects} {itemTypes}
              {customFieldDefinitions} {onitemUpdated} {onupdateError}
            />
          </div>
        {/each}
        <div>
          <DropdownMenu triggerText="" triggerIcon={MoreHorizontal}
            triggerClass="p-2 rounded action-btn transition-colors" items={actions} />
        </div>
      </div>
{/snippet}

<div
  use:virtualAttachment
  use:dragRow
  class="px-4 py-3 list-row transition-colors"
  style="border-top: 1px solid var(--ds-border);"
  data-item-row
  data-item-id={item.id}
  data-testid={rowTestid}
>
  {#if deferred}
    <LazyRender>{@render cells()}</LazyRender>
  {:else}
    {@render cells()}
  {/if}
</div>

<style>
  .list-row:hover {
    background-color: var(--ctx-background-neutral-hovered, var(--ds-background-neutral-hovered));
  }
</style>
