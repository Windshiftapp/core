<script>
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import ItemKey from '../../components/ItemKey.svelte';
  import ParentChip from '../../components/ParentChip.svelte';
  import { formatDateSimple } from '../../utils/dateFormatter.js';
  import ItemCard from './ItemCard.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { getStatusCategory } from '../../utils/statusColors.js';
  import { t } from '../../stores/i18n.svelte.js';
  import EditableField from '../../editors/EditableField.svelte';
  import Avatar from '../../components/Avatar.svelte';
  import Chip from '../../components/Chip.svelte';
  import CalendarDays from '@lucide/svelte/icons/calendar-days';

  /**
   * Reusable list-row props. Lookup arrays enrich the item; leading and trailing
   * snippets add controls. `onclick` replaces navigation when supplied.
   */
  let {
    item,
    workspace = null,
    itemTypes = [],
    statuses = [],
    priorities = [],
    statusCategories = [],
    href = null,
    onclick = null,
    showIcon = true,
    showKey = true,
    showWorkspace = false,
    showTimestamp = false,
    showStatus = false,
    showPriority = false,
    showParent = false,
    showAssignee = false,
    showDueDate = false,
    showMilestones = false,
    timestamp = null,
    formatTimestamp = null,
    compact = false,
    dataTestid = undefined,
    showStoryPoints = false,
    storyPointsSaving = false,
    onStoryPointsChange = null,
    leading = null,
    trailing = null,
  } = $props();

  // Look up the item type for icon and color
  const itemType = $derived(item.item_type_id ? itemTypes.find(t => t.id === item.item_type_id) : null);

  // Build the href if not provided (disabled if onclick is set)
  const itemHref = $derived(onclick ? null : (href || `/workspaces/${item.workspace_id}/items/${item.id}`));

  // Format the timestamp
  const formattedTimestamp = $derived.by(() => {
    if (!timestamp) return null;
    if (formatTimestamp) return formatTimestamp(timestamp);
    // Default formatting
    return formatDateSimple(timestamp);
  });

  // Look up status - supports pre-resolved status_name, status string, or lookup by status_id
  const status = $derived.by(() => {
    // If item already has status info from JOIN
    if (item.status_name) {
      return { name: item.status_name, id: item.status_id };
    }
    // If item has status as a string (e.g., from Homepage activity API)
    if (typeof item.status === 'string' && item.status) {
      return { name: item.status, id: null };
    }
    // Otherwise lookup from statuses array
    if (item.status_id && statuses.length > 0) {
      return statuses.find(s => s.id === item.status_id) || null;
    }
    return null;
  });

  // Look up priority - supports both pre-resolved priority info and lookup by priority_id
  const priority = $derived.by(() => {
    // If item already has priority info from JOIN
    if (item.priority_name) {
      return { name: item.priority_name, color: item.priority_color, id: item.priority_id };
    }
    // Otherwise lookup from priorities array
    if (item.priority_id && priorities.length > 0) {
      return priorities.find(p => p.id === item.priority_id) || null;
    }
    return null;
  });

  // Get status category for color
  const statusCategory = $derived.by(() => {
    if (!status?.name) return null;
    return getStatusCategory(status.name, statuses, statusCategories);
  });

  // Resolve the status badge color, preferring a pre-resolved color on the item
  // (e.g. status_color from the homepage activity API where statuses arrays aren't loaded)
  const statusColor = $derived(item.status_color || statusCategory?.color || '#6b7280');

  // List responses include a permission-checked parent_key; ParentChip owns
  // the parent-key fallback for callers that provide the older hydrated shape.
  const firstMilestone = $derived(item.milestones?.[0] ?? null);
  const remainingMilestones = $derived(Math.max(0, (item.milestones?.length ?? 0) - 1));

  function commitStoryPoints(draft) {
    const raw = String(draft ?? '').trim();
    const parsed = raw === '' ? null : Number(raw);
    if (parsed !== null && (!Number.isFinite(parsed) || parsed < 0)) {
      throw new Error(t('items.enterField', { field: t('items.storyPoints') }));
    }
    if (parsed === (item.story_points ?? null)) return;
    onStoryPointsChange?.(parsed);
  }
</script>

<ItemCard href={itemHref} {onclick} {compact} {dataTestid}>
  {#snippet children()}
    <div class="flex items-center gap-3">
      {#if leading}{@render leading()}{/if}

      <!-- Item Type Icon -->
      {#if showIcon}
        <ItemTypeIcon
          itemType={itemType}
          icon={item.item_type_icon}
          color={item.item_type_color}
          title={item.item_type_name || undefined}
          testId={`work-item-type-icon-${item.id}`}
        />
      {/if}

      <!-- Item Key -->
      {#if showKey}
        <ItemKey {item} {workspace} variant="badge" clickable={false} />
      {/if}

      <!-- Title -->
      <h4 class="text-sm flex-1 min-w-0 truncate" dir="auto" style="color: var(--ds-text);" title={item.title}>{item.title}</h4>

      <!-- Optional Parent -->
      {#if showParent}
        <ParentChip {item} />
      {/if}

      <!-- Optional Milestone -->
      {#if showMilestones && firstMilestone}
        <Chip
          appearance="metadata"
          dotColor={firstMilestone.category_color || null}
          class="max-w-40 min-w-0 shrink"
          title={(item.milestones ?? []).map((milestone) => milestone.name).join(', ')}
        >
          <span class="truncate" data-testid={`work-item-milestone-${item.id}`}>{firstMilestone.name}</span>
          {#if remainingMilestones > 0}
            <span class="shrink-0">+{remainingMilestones}</span>
          {/if}
        </Chip>
      {/if}

      <!-- Optional Priority -->
      {#if showPriority && priority}
        <span
          class="inline-flex flex-shrink-0 rounded-md px-2 py-0.5 text-xs font-medium"
          style="background-color: {priority.color}20; color: {priority.color};"
          data-testid={`work-item-priority-${item.id}`}
        >
          {priority.name}
        </span>
      {/if}

      <!-- Optional Assignee -->
      {#if showAssignee && item.assignee_name}
        <span
          class="inline-flex max-w-36 min-w-0 items-center gap-1.5 text-xs text-ds-text-subtle"
          title={`${t('common.assignee')}: ${item.assignee_name}`}
          data-testid={`work-item-assignee-${item.id}`}
        >
          <Avatar src={item.assignee_avatar || null} name={item.assignee_name} size="2xs" variant="neutral" />
          <span class="truncate">{item.assignee_name}</span>
        </span>
      {/if}

      <!-- Optional Due Date -->
      {#if showDueDate && item.due_date}
        <span
          class="inline-flex flex-shrink-0 items-center gap-1 text-xs text-ds-text-subtle"
          title={`${t('common.dueDate')}: ${formatDateSimple(item.due_date)}`}
          data-testid={`work-item-due-date-${item.id}`}
        >
          <CalendarDays class="h-3 w-3" aria-hidden="true" />
          {formatDateSimple(item.due_date)}
        </span>
      {/if}

      <!-- Optional Workspace Name -->
      {#if showWorkspace && item.workspace_name}
        <span class="text-xs min-w-0 truncate" dir="auto" style="color: var(--ds-text-subtle);">{item.workspace_name}</span>
      {/if}

      <!-- Optional Timestamp -->
      {#if showTimestamp && formattedTimestamp}
        <span class="text-xs flex-shrink-0" style="color: var(--ds-text-subtle);">{formattedTimestamp}</span>
      {/if}

      <!-- Status Badge -->
      {#if showStatus && status}
        <Lozenge text={status.name.replace(/_/g, ' ')} customBg={statusColor} />
      {/if}

      {#if showStoryPoints}
        <div
          class="flex-shrink-0"
          data-testid={`backlog-story-points-${item.id}`}
        >
          <EditableField
            value={item.story_points ?? null}
            editable={!storyPointsSaving}
            oncommit={commitStoryPoints}
            stopPropagation
          >
            {#snippet display()}
              <span
                class="inline-flex items-center gap-1 rounded px-2 py-1 text-xs tabular-nums transition-colors hover:bg-black/5 dark:hover:bg-white/10"
                style="color: var(--ds-text-subtle);"
                title={t('items.setField', { field: t('items.storyPoints') })}
                aria-label={t('items.setField', { field: t('items.storyPoints') })}
                data-testid={`backlog-story-points-button-${item.id}`}
              >
                <span style="color: var(--ds-text);">{item.story_points ?? t('items.notSet')}</span>
                <span>SP</span>
              </span>
            {/snippet}
            {#snippet editor({ draft, setDraft, error })}
              <!-- svelte-ignore a11y_autofocus -->
              <input
                type="number"
                min="0"
                step="0.5"
                aria-label={t('items.storyPoints')}
                aria-invalid={error ? 'true' : 'false'}
                aria-describedby={error ? `backlog-story-points-error-${item.id}` : undefined}
                data-testid={`backlog-story-points-input-${item.id}`}
                class="w-16 rounded border px-2 py-1 text-xs tabular-nums outline-none"
                style="background-color: var(--ds-surface-card); border-color: {error ? 'var(--ds-text-danger)' : 'var(--ds-border-focused)'}; color: var(--ds-text);"
                value={draft ?? ''}
                oninput={(e) => setDraft(e.currentTarget.value)}
                onclick={(event) => event.stopPropagation()}
                autofocus
              />
              {#if error}
                <span id={`backlog-story-points-error-${item.id}`} class="sr-only">{error}</span>
              {/if}
            {/snippet}
          </EditableField>
        </div>
      {/if}

      {#if trailing}{@render trailing()}{/if}
    </div>
  {/snippet}
</ItemCard>
