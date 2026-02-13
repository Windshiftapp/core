<script>
  import { FileText, Link2, Trash2, Plus, GripVertical } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import Button from '../../components/Button.svelte';
  import LinkComponent from '../../components/Link.svelte';
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import DropIndicator from '../../layout/DropIndicator.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  export let item;
  export let workspace;
  export let workspaceId;
  export let itemId;
  export let itemLinks = [];
  export let loadingLinks = false;
  export let availableSubIssueTypes = [];
  export let childItems = [];
  export let loadingChildItems = false;
  export let itemTypes = [];
  export let isModal = false;
  export let isLowestLevel = false;

  const TEST_LINK_TYPE_ID = 1;

  // Use centralized icon map for item types
  $: currentItemId = parseInt(itemId);
  const iconMap = itemTypeIconMap;

  function getLinkLabel(link) {
    const isCurrentSource = currentItemId === link.source_id;
    if (link.link_type_id === TEST_LINK_TYPE_ID && isCurrentSource && link.source_type === 'item' && link.target_type === 'test_case') {
      return link.link_type_reverse_label;
    }
    return isCurrentSource ? link.link_type_forward_label : link.link_type_reverse_label;
  }

  function handleLinkClick(event, linkedItemType, linkedItemId, linkedItemWorkspaceId, linkedItemHref) {
    if (linkedItemType === 'test_case') {
      event.preventDefault();
      dispatch('view-test-case', { testCaseId: linkedItemId });
      return;
    }

    if (isModal) {
      event.preventDefault();
      const targetWorkspaceId = linkedItemWorkspaceId || workspaceId;
      const destination = linkedItemHref || `/workspaces/${targetWorkspaceId}/items/${linkedItemId}`;
      dispatch('navigate', { path: destination });
    }
  }

  function startCreateSubIssue() {
    dispatch('create-sub-issue');
  }

  function removeLink(linkId) {
    dispatch('remove-link', { linkId });
  }

  function handleShowLinkModal() {
    dispatch('show-link-modal');
  }

  // Drag and drop state for child items
  let dragState = new Map();
  let setupElements = new Map();
  let pendingDrops = new Set();
  let setupTimeout;
  const childRowGap = 8; // space-y-2 = 8px

  $: if (isLowestLevel && childItems.length > 0 && typeof document !== 'undefined') {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupTimeout = setTimeout(() => {
      setupDragAndDrop();
    }, 100);
  }

  function setupDragAndDrop() {
    if (setupTimeout) clearTimeout(setupTimeout);

    // Clean up existing registrations
    setupElements.forEach((cleanup) => {
      if (typeof cleanup === 'function') cleanup();
    });
    setupElements.clear();
    dragState = new Map();

    const itemCards = document.querySelectorAll('[data-child-item-card]');

    itemCards.forEach(element => {
      const childItemId = parseInt(element.dataset.itemId);
      const elementId = `child-${childItemId}`;

      const childItem = childItems.find(i => i.id === childItemId);
      if (!childItem) return;

      dragState.set(childItemId, { isDragging: false, closestEdge: null });

      const draggableCleanup = draggable({
        element,
        getInitialData: () => ({
          item: childItem,
          type: 'child-item'
        }),
        onDragStart: () => {
          element.style.opacity = '0.5';
          document.body.classList.add('is-dragging');
          const newMap = new Map(dragState);
          newMap.set(childItemId, { ...(dragState.get(childItemId) || {}), isDragging: true });
          dragState = newMap;
        },
        onDrop: () => {
          element.style.opacity = '';
          document.body.classList.remove('is-dragging');
          const newMap = new Map();
          dragState.forEach((state, id) => {
            newMap.set(id, { isDragging: false, closestEdge: null });
          });
          dragState = newMap;
        }
      });

      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          return source.data.type === 'child-item' && source.data.item.id !== childItemId;
        },
        getData: ({ input, element }) => {
          return attachClosestEdge({}, {
            input,
            element,
            allowedEdges: ['top', 'bottom']
          });
        },
        onDragEnter: ({ self, source }) => {
          if (source.data.type === 'child-item' && source.data.item.id !== childItemId) {
            const closestEdge = extractClosestEdge(self.data);
            const newMap = new Map(dragState);
            newMap.set(childItemId, { ...(dragState.get(childItemId) || {}), closestEdge });
            dragState = newMap;
          }
        },
        onDragLeave: () => {
          const newMap = new Map(dragState);
          newMap.set(childItemId, { ...(dragState.get(childItemId) || {}), closestEdge: null });
          dragState = newMap;
        },
        onDrop: ({ self, source }) => {
          const closestEdge = extractClosestEdge(self.data);
          if (source.data.type === 'child-item' && closestEdge) {
            handleEdgeBasedDrop(source.data.item, childItem, closestEdge);
          }
        }
      });

      setupElements.set(elementId, () => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });
  }

  async function handleEdgeBasedDrop(draggedItem, targetItem, closestEdge) {
    const dropId = `${draggedItem.id}-edge-${targetItem.id}-${closestEdge}`;

    try {
      if (pendingDrops.has(dropId)) return;
      pendingDrops.add(dropId);

      const targetIndex = childItems.findIndex(i => i.id === targetItem.id);
      const draggedIndex = childItems.findIndex(i => i.id === draggedItem.id);

      const otherItems = childItems.filter(i => i.id !== draggedItem.id);
      const adjustedTargetIndex = otherItems.findIndex(i => i.id === targetItem.id);

      const isDroppingSamePosition = (
        (closestEdge === 'top' && draggedIndex === targetIndex - 1) ||
        (closestEdge === 'bottom' && draggedIndex === targetIndex + 1)
      );

      if (isDroppingSamePosition) return;

      let prevItemId = null;
      let nextItemId = null;

      if (closestEdge === 'top') {
        if (adjustedTargetIndex > 0) {
          const prevItem = otherItems[adjustedTargetIndex - 1];
          if (prevItem) prevItemId = prevItem.id;
        }
        if (targetItem) nextItemId = targetItem.id;
      } else if (closestEdge === 'bottom') {
        if (targetItem) prevItemId = targetItem.id;
        if (adjustedTargetIndex < otherItems.length - 1) {
          const nextItem = otherItems[adjustedTargetIndex + 1];
          if (nextItem) nextItemId = nextItem.id;
        }
      }

      await api.items.updateFracIndex(draggedItem.id, {
        prev_item_id: prevItemId,
        next_item_id: nextItemId
      });

      dispatch('reorder-children');
    } catch (error) {
      console.error('Failed to reorder child item:', error);
      dispatch('reorder-children');
    } finally {
      setTimeout(() => pendingDrops.delete(dropId), 500);
    }
  }

  onDestroy(() => {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupElements.forEach((cleanup) => {
      if (typeof cleanup === 'function') cleanup();
    });
    setupElements.clear();
  });
</script>

<!-- Links Section -->
{#if itemLinks.length > 0}
  <div class="mt-6">
    <div class="pt-2">
      <!-- Header with icon, label, and add button -->
      <div class="flex items-center justify-between mb-4">
        <div class="flex items-center gap-2">
          <Link2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          <h3 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle); font-size: 11px;">{t('items.linkedItems')}</h3>
        </div>
        <button
          type="button"
          class="inline-flex items-center gap-1 px-2 py-1 text-xs font-medium rounded transition-colors cursor-pointer"
          style="color: var(--ds-text-subtle);"
          onmouseenter={(e) => { e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'; e.currentTarget.style.color = 'var(--ds-text)'; }}
          onmouseleave={(e) => { e.currentTarget.style.backgroundColor = ''; e.currentTarget.style.color = 'var(--ds-text-subtle)'; }}
          onclick={handleShowLinkModal}
        >
          <Plus class="w-3 h-3" />
          {t('common.add')}
        </button>
      </div>

      {#if loadingLinks}
        <div class="text-center py-4">
          <div class="text-sm text-gray-500">{t('items.loadingLinks')}</div>
        </div>
      {:else}
      <div class="space-y-2">
        {#each itemLinks as link}
          {@const isCurrentSource = link.source_id === currentItemId}
          {@const linkedItemType = isCurrentSource ? link.target_type : link.source_type}
          {@const linkedItemId = isCurrentSource ? link.target_id : link.source_id}
          {@const linkedItemWorkspaceId = isCurrentSource ? link.target_workspace_id : link.source_workspace_id}
          {@const linkedItemKeyPrefix = linkedItemType === 'test_case'
            ? 'TC'
            : (isCurrentSource
              ? (link.target_workspace_key || workspace?.key || 'WORK')
              : (link.source_workspace_key || workspace?.key || 'WORK'))}
          {@const linkedItemKey = `${linkedItemKeyPrefix}-${linkedItemId}`}
          {@const linkedItemTitle = isCurrentSource ? link.target_title : link.source_title}
          {@const linkedItemHref = linkedItemType === 'test_case'
            ? '#view-test-case'
            : `/workspaces/${linkedItemWorkspaceId || workspaceId}/items/${linkedItemId}`}
          {@const isLinkedTestCase = linkedItemType === 'test_case'}
          {@const linkedItemTypeIcon = isCurrentSource ? link.target_item_type_icon : link.source_item_type_icon}
          {@const linkedItemTypeColor = isCurrentSource ? link.target_item_type_color : link.source_item_type_color}
          {@const linkedItemStatusName = isCurrentSource ? link.target_status_name : link.source_status_name}
          <!-- Item row with card styling and hover-reveal delete -->
          <div
            class="group flex items-center justify-between px-4 py-3 rounded-lg border transition-colors"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
          >
            <div class="flex items-center gap-3 flex-1 min-w-0">
              <!-- Item type icon -->
              {#if linkedItemTypeIcon && iconMap[linkedItemTypeIcon]}
                <div
                  class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                  style="background-color: {linkedItemTypeColor || '#6b7280'}20; color: {linkedItemTypeColor || '#6b7280'};"
                >
                  <svelte:component this={iconMap[linkedItemTypeIcon]} class="w-3.5 h-3.5" />
                </div>
              {:else}
                <div
                  class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                  style="background-color: #6b728020; color: #6b7280;"
                >
                  <FileText class="w-3.5 h-3.5" />
                </div>
              {/if}
              <!-- Item key -->
              <LinkComponent
                href={linkedItemHref}
                class="text-xs font-mono whitespace-nowrap transition-colors cursor-pointer"
                style="color: var(--ds-text-subtle);"
                onClick={(event) => handleLinkClick(event, linkedItemType, linkedItemId, linkedItemWorkspaceId, linkedItemHref)}
              >
                {linkedItemKey}
              </LinkComponent>
              <!-- Item title -->
              <LinkComponent
                href={linkedItemHref}
                class="text-sm hover:text-blue-600 cursor-pointer truncate"
                onClick={(event) => handleLinkClick(event, linkedItemType, linkedItemId, linkedItemWorkspaceId, linkedItemHref)}
                style="color: var(--ds-text);"
              >
                {linkedItemTitle}
              </LinkComponent>
            </div>
            <!-- Right side: status badge + delete button -->
            <div class="flex items-center gap-2 flex-shrink-0">
              {#if linkedItemStatusName}
                <span class="text-xs px-2 py-0.5 rounded-full" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                  {linkedItemStatusName}
                </span>
              {/if}
              <button
                class="p-1 rounded hidden group-hover:flex cursor-pointer delete-button"
                style="color: var(--ds-text-subtle);"
                onmouseenter={(e) => e.currentTarget.style.color = '#dc2626'}
                onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
                onclick={() => removeLink(link.id)}
                title={t('items.removeLink')}
              >
                <Trash2 class="w-4 h-4" />
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
{/if}

<!-- Child Work Items Section -->
{#if childItems.length > 0}
  <div class="mt-4">
    <div class="pt-2">
      <div class="flex items-center gap-2 mb-4">
        <FileText class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        <h3 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle); font-size: 11px;">{t('items.childWorkItems')}</h3>
      </div>

      {#if loadingChildItems}
        <div class="text-center py-4">
          <div class="text-sm" style="color: var(--ds-text-subtle);">{t('items.loadingChildItems')}</div>
        </div>
      {:else}
        <div class="space-y-2">
          {#each childItems as childItem}
            {@const childItemType = itemTypes.find(type => type.id === childItem.item_type_id)}
            <div
              class="group flex items-center justify-between px-4 py-3 rounded-lg border transition-colors relative"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
              data-child-item-card
              data-item-id={childItem.id}
            >
              {#if isLowestLevel && dragState.get(childItem.id)?.closestEdge}
                <DropIndicator edge={dragState.get(childItem.id)?.closestEdge} gap={childRowGap} />
              {/if}
              <div class="flex items-center gap-3 flex-1 min-w-0">
                {#if isLowestLevel}
                  <div class="cursor-grab active:cursor-grabbing flex-shrink-0" style="color: var(--ds-text-subtle);">
                    <GripVertical class="w-4 h-4" />
                  </div>
                {/if}
                <!-- Item type icon -->
                {#if childItemType}
                  <div
                    class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                    style="background-color: {childItemType.color || '#6b7280'}20; color: {childItemType.color || '#6b7280'};"
                  >
                    <svelte:component this={iconMap[childItemType.icon] || FileText} class="w-3.5 h-3.5" />
                  </div>
                {:else}
                  <div
                    class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                    style="background-color: #6b728020; color: #6b7280;"
                  >
                    <FileText class="w-3.5 h-3.5" />
                  </div>
                {/if}
                <!-- Item key -->
                <LinkComponent
                  href="/workspaces/{childItem.workspace_id || workspaceId}/items/{childItem.id}"
                  class="text-xs font-mono whitespace-nowrap transition-colors cursor-pointer"
                  style="color: var(--ds-text-subtle);"
                >
                  {childItem.workspace_key || childItem.workspace?.key || workspace?.key || 'WORK'}-{childItem.id}
                </LinkComponent>
                <!-- Item title -->
                <LinkComponent
                  href="/workspaces/{childItem.workspace_id || workspaceId}/items/{childItem.id}"
                  class="text-sm hover:text-blue-600 cursor-pointer truncate"
                  style="color: var(--ds-text);"
                >
                  {childItem.title}
                </LinkComponent>
              </div>
              <!-- Right side: status badge -->
              <div class="flex items-center gap-2 flex-shrink-0">
                {#if childItem.status_name}
                  <span class="text-xs px-2 py-0.5 rounded-full" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    {childItem.status_name}
                  </span>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .delete-button {
    animation: fadeIn 150ms ease-out;
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }
</style>
