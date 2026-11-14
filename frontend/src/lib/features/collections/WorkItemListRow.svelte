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
    selectionMode = false,
    selected = false,
    onselect = null,
    onshiftselect = null,
  } = $props();

  // In selection mode the whole row toggles its selection; otherwise
  // shift-click extends the view's selection and a plain click is left to the
  // cell editors. Mirrors BoardItemCard.
  function handleRowClick(event) {
    if (selectionMode && onselect) {
      event.preventDefault();
      onselect(item.id, event);
      return;
    }
    if (event.shiftKey && onshiftselect) {
      event.preventDefault();
      onshiftselect(item.id, event);
    }
  }

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

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div
  use:virtualAttachment
  use:dragRow
  class="px-4 py-3 list-row transition-colors"
  style="border-top: 1px solid var(--ds-border);"
  data-item-row
  data-item-id={item.id}
  data-testid={rowTestid}
  data-selected={selected ? 'true' : 'false'}
  data-selection-mode={selectionMode ? 'true' : 'false'}
  onclick={handleRowClick}
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

  /* Selection mode dims every row so the selected subset stands out. The
     selected row stays fully opaque and carries the focus accent. */
  .list-row[data-selection-mode='true']:not([data-selected='true']) {
    opacity: 0.5;
  }

  .list-row[data-selection-mode='true']:not([data-selected='true']):hover {
    opacity: 0.8;
  }

  .list-row[data-selected='true'] {
    opacity: 1;
    box-shadow: inset 2px 0 0 var(--ds-border-focused);
  }
</style>
