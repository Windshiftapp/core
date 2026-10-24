<script>
  import CalendarDays from '@lucide/svelte/icons/calendar-days';
  import MoreHorizontal from '@lucide/svelte/icons/more-horizontal';
  import Avatar from '../../components/Avatar.svelte';
  import DropIndicator from '../../layout/DropIndicator.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { formatDateOnly } from '../../utils/dateFormatter.js';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import ItemKey from '../../components/ItemKey.svelte';
  import CardFieldChip from './CardFieldChip.svelte';
  import DependencySummary from './DependencySummary.svelte';
  import SLABadge from '../sla/SLABadge.svelte';

  let {
    item,
    workspace = null,
    itemTypes = [],
    cardFields = [],
    priorities = [],
    statuses = [],
    iterations = [],
    projects = [],
    labels = [],
    customFieldDefinitions = [],
    users = [],
    dependencyLinks = [],
    moveMenuItems = [],
    closestEdge = null,
    swimlaneDimension = '',
    swimlaneKey = '',
    cardStyle = 'background-color: var(--ds-surface-raised); border-color: var(--ds-border);',
    textStyle = 'color: var(--ds-text);',
    showMoveMenu = true,
    // Registered with pragmatic-drag-and-drop only when the user may edit the
    // item; the action re-runs when this flips (e.g. permissions finish loading).
    canDrag = true,
    dndAction = () => {},
    refreshToken = 0,
    onopen = null,
  } = $props();

  const resolvedMoveMenuItems = $derived(
    moveMenuItems.length > 0
      ? moveMenuItems
      : [{ id: `no-moves-${item.id}`, type: 'text', text: 'No available moves' }],
  );

  const dndConfig = $derived([item.id, canDrag]);

  // Real href so the key keeps new-tab affordances; the modal handles plain clicks.
  const itemHref = $derived(
    item.workspace_id ?? workspace?.id
      ? `/workspaces/${item.workspace_id ?? workspace?.id}/items/${item.id}`
      : null,
  );

  // Board-only fields rendered outside the body chip row.
  const FOOTER_CARD_FIELDS = new Set(['due_date', 'sla']);
  const bodyCardFields = $derived(
    cardFields.filter(
      (field) =>
        !(field.field_type === 'system' && FOOTER_CARD_FIELDS.has(field.field_identifier)),
    ),
  );
  // SLA state is only read when the board configures the SLA card field.
  const showsSLA = $derived(
    cardFields.some(
      (field) => field.field_type === 'system' && field.field_identifier === 'sla',
    ),
  );
  const itemType = $derived(
    item.item_type_id ? itemTypes.find((type) => type.id === item.item_type_id) : null,
  );
  const showsDueDate = $derived(
    Boolean(
      item.due_date &&
        cardFields.some(
          (field) => field.field_type === 'system' && field.field_identifier === 'due_date',
        ),
    ),
  );

  const avatarVariants = ['blue', 'teal', 'purple', 'green', 'orange'];

  function avatarVariant(userId) {
    return avatarVariants[Math.abs(Number(userId) || 0) % avatarVariants.length];
  }

  function open(event) {
    onopen?.(item.id, event);
  }

  function handleKeydown(event) {
    if (event.key === 'Enter' || event.key === ' ') {
      open(event);
    }
  }
</script>

<div
  use:dndAction={dndConfig}
  class="board-card relative rounded-[4px] border px-3 py-2.5"
  style={cardStyle}
  data-testid={`board-item-${item.id}`}
  data-item-card
  data-item-id={item.id}
  data-swimlane-dimension={swimlaneDimension}
  data-swimlane-key={swimlaneKey}
  role="button"
  tabindex="0"
  onclick={open}
  onkeydown={handleKeydown}
>
  {#if closestEdge}
    <DropIndicator edge={closestEdge} />
  {/if}

  <div class={canDrag ? 'cursor-grab active:cursor-grabbing' : ''}>
    <div class="min-w-0">
      <div class="flex items-start gap-2">
        <h4 class="min-w-0 flex-1 break-words text-sm font-medium leading-5" style={textStyle}>
          {item.title}
        </h4>
        {#if showMoveMenu}
          <div class="board-card-menu -mr-1 -mt-1 shrink-0 transition-opacity">
            <DropdownMenu
              items={resolvedMoveMenuItems}
              placement="bottom-end"
              maxWidth="max-w-xs"
              triggerIcon={MoreHorizontal}
              triggerClass="p-1 rounded hover:bg-[var(--ds-background-neutral-hovered)] focus:ring-2 focus:ring-[var(--ds-border-focused)]"
              triggerStyle="color: var(--ds-text-subtle);"
              iconOnly
              showChevron={false}
              triggerTestid={`board-card-move-menu-${item.id}`}
            />
          </div>
        {/if}
      </div>

      {#if bodyCardFields.length > 0}
        <div class="mt-2 flex flex-wrap gap-1.5">
          {#each bodyCardFields as cardField}
            <CardFieldChip
              {cardField}
              {item}
              {priorities}
              {statuses}
              {iterations}
              {projects}
              {labels}
              {customFieldDefinitions}
              {users}
            />
          {/each}
        </div>
      {/if}

      <div
        class="mt-2.5 flex min-h-6 items-center gap-2 pt-2"
        data-testid={`board-card-footer-${item.id}`}
      >
        <span class="inline-flex shrink-0 items-center gap-1.5">
          {#if item.item_type_icon || itemType}
            <ItemTypeIcon
              {itemType}
              icon={item.item_type_icon}
              color={item.item_type_color}
              title={item.item_type_name || undefined}
              testId={`board-card-type-icon-${item.id}`}
            />
          {/if}
          <ItemKey
            {item}
            {workspace}
            href={itemHref}
            onClick={(event) => {
              event.preventDefault();
              open(event);
            }}
            size="compact"
            monospace={false}
            style="color: var(--ds-text-subtlest);"
          />
        </span>
        {#if showsDueDate}
          <span
            class="inline-flex items-center gap-1 text-[11px] leading-4"
            style="color: var(--ds-text-subtle);"
            title="Due date"
          >
            <CalendarDays class="h-3 w-3 shrink-0" />
            {formatDateOnly(item.due_date)}
          </span>
        {/if}
        <DependencySummary {item} links={dependencyLinks} />
        {#if showsSLA}
          <span class="inline-flex shrink-0" data-testid={`board-card-sla-${item.id}`}>
            <SLABadge itemId={item.id} workspaceId={item.workspace_id} {refreshToken} />
          </span>
        {/if}
        <span class="flex-1"></span>
        {#if item.assignee_id}
          {@const assignee = users.find((user) => user.id === item.assignee_id)}
          {#if assignee}
            <Avatar
              src={assignee.avatar_url || item.assignee_avatar}
              firstName={assignee.first_name}
              lastName={assignee.last_name}
              size="2xs"
              variant={avatarVariant(assignee.id)}
              title="{assignee.first_name} {assignee.last_name}"
            />
          {/if}
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .board-card {
    box-shadow: none;
    transition:
      border-color 140ms ease-in-out,
      background-color 140ms ease-in-out,
      box-shadow 140ms ease-in-out,
      opacity 200ms ease;
  }

  .board-card:hover {
    background-color: var(--ds-surface-raised-hovered) !important;
    border-color: var(--ds-border-bold) !important;
    box-shadow: var(--ds-shadow-raised);
  }

  .board-card:focus-visible {
    outline: 2px solid var(--ds-border-focused);
    outline-offset: 1px;
  }

  .board-card-menu {
    opacity: 0.35;
  }

  .board-card:hover .board-card-menu,
  .board-card:focus-within .board-card-menu {
    opacity: 1;
  }
</style>
