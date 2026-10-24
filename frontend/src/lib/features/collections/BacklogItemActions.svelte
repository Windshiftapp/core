<script>
  import ArrowDownToLine from '@lucide/svelte/icons/arrow-down-to-line';
  import ArrowUpToLine from '@lucide/svelte/icons/arrow-up-to-line';
  import CalendarDays from '@lucide/svelte/icons/calendar-days';
  import MoreHorizontal from '@lucide/svelte/icons/more-horizontal';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    item,
    iterations = [],
    disabled = false,
    onMoveToBoundary,
    onAssignIteration,
  } = $props();

  let iterationOptions = $derived(
    iterations
      .filter((iteration) => iteration.id !== item.iteration_id)
      .map((iteration) => ({
        id: `assign-${item.id}-${iteration.id}`,
        title: iteration.name,
        subtitle: iteration.status,
        onClick: () => onAssignIteration?.(item, iteration),
        testid: `backlog-assign-iteration-${item.id}-${iteration.id}`,
      })),
  );

  let menuItems = $derived.by(() => {
    /** @type {any[]} */
    const actions = [
      {
        id: `move-start-${item.id}`,
        title: t('collections.toBeginningOfBacklog'),
        icon: ArrowUpToLine,
        onClick: () => onMoveToBoundary?.(item, 'start'),
        testid: `backlog-move-start-${item.id}`,
      },
      {
        id: `move-end-${item.id}`,
        title: t('collections.sendToEndOfBacklog'),
        icon: ArrowDownToLine,
        onClick: () => onMoveToBoundary?.(item, 'end'),
        testid: `backlog-move-end-${item.id}`,
      },
    ];

    if (iterationOptions.length > 0) {
      actions.push(
        { id: `iteration-divider-${item.id}`, type: 'divider' },
        {
          id: `assign-iteration-${item.id}`,
          type: 'accordion',
          title: t('collections.assignToIteration'),
          icon: CalendarDays,
          subItems: iterationOptions,
          testid: `backlog-assign-iteration-menu-${item.id}`,
        },
      );
    }

    return actions;
  });
</script>

<DropdownMenu
  items={menuItems}
  placement="bottom-end"
  maxWidth="max-w-xs"
  triggerIcon={MoreHorizontal}
  triggerClass="p-1 rounded hover:bg-[var(--ds-background-neutral-hovered)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ds-border-focused)]"
  triggerStyle="color: var(--ds-text-subtle);"
  iconOnly
  showChevron={false}
  {disabled}
  triggerLabel={t('collections.backlogItemActions', { title: item.title })}
  triggerTestid={`backlog-item-menu-${item.id}`}
/>
