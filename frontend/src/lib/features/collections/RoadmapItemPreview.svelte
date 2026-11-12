<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import BoardItemCard from './BoardItemCard.svelte';

  let {
    item,
    workspace,
    itemTypes = [],
    cardFields = [],
    priorities = [],
    statuses = [],
    iterations = [],
    projects = [],
    labels = [],
    customFieldDefinitions = [],
    users = [],
    onpointerdown = null,
    onopen = null,
    children,
  } = $props();

  const {
    elements: { trigger, content },
    states: { open },
  } = createPopover({
    forceVisible: true,
    positioning: {
      strategy: 'fixed',
      placement: 'bottom-start',
      gutter: 8,
      flip: true,
    },
    portal: 'body',
  });

  function openItem(itemId, event) {
    event?.stopPropagation();
    $open = false;
    onopen?.(itemId);
  }
</script>

<div use:melt={$trigger} onpointerdown={onpointerdown}>
  {@render children()}
</div>

{#if $open}
  <div
    use:melt={$content}
    class="roadmap-item-preview z-[60] w-[min(20rem,calc(100vw-2rem))]"
    data-testid={`roadmap-item-preview-${item.id}`}
  >
    <BoardItemCard
      {item}
      {workspace}
      {itemTypes}
      {cardFields}
      {priorities}
      {statuses}
      {iterations}
      {projects}
      {labels}
      {customFieldDefinitions}
      {users}
      showMoveMenu={false}
      onopen={openItem}
    />
  </div>
{/if}

<style>
  .roadmap-item-preview :global(.board-card) {
    box-shadow: var(--ds-shadow-raised);
  }
</style>
