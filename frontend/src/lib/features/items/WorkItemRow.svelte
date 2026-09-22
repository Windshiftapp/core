<script>
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import { formatDateSimple } from '../../utils/dateFormatter.js';
  import ItemCard from './ItemCard.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { getStatusCategory } from '../../utils/statusColors.js';
  import { t } from '../../stores/i18n.svelte.js';

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

  let editingStoryPoints = $state(false);
  let storyPointsEditValue = $state('');
  let storyPointsError = $state(false);

  // Compute the display key - prefer item.workspace_key, fallback to workspace.key
  const displayKey = $derived.by(() => {
    const key = item.workspace_key || workspace?.key;
    return key ? `${key}-${item.workspace_item_number}` : `ITEM-${item.workspace_item_number}`;
  });

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

  function startEditingStoryPoints(event) {
    event?.stopPropagation();
    if (storyPointsSaving) return;
    storyPointsEditValue = item.story_points == null ? '' : String(item.story_points);
    storyPointsError = false;
    editingStoryPoints = true;
  }

  function cancelStoryPointsEdit(event) {
    event?.stopPropagation();
    editingStoryPoints = false;
    storyPointsError = false;
  }

  function saveStoryPoints(event) {
    event?.stopPropagation();
    const raw = String(storyPointsEditValue ?? '').trim();
    const parsed = raw === '' ? null : Number(raw);
    if (parsed !== null && (!Number.isFinite(parsed) || parsed < 0)) {
      storyPointsError = true;
      return;
    }

    if (parsed === (item.story_points ?? null)) {
      editingStoryPoints = false;
      storyPointsError = false;
      return;
    }

    editingStoryPoints = false;
    storyPointsError = false;
    onStoryPointsChange?.(parsed);
  }

  function handleStoryPointsKeydown(event) {
    event.stopPropagation();
    if (event.key === 'Enter') {
      event.preventDefault();
      saveStoryPoints(event);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      cancelStoryPointsEdit(event);
    }
  }
</script>

<ItemCard href={itemHref} {onclick} {compact} {dataTestid}>
  {#snippet children()}
    <div class="flex items-center gap-3">
      {#if leading}{@render leading()}{/if}

      <!-- Item Type Icon -->
      {#if showIcon}
        <ItemTypeIcon itemType={itemType} />
      {/if}

      <!-- Item Key -->
      {#if showKey}
        <span class="font-mono text-xs px-2 py-0.5 rounded flex-shrink-0" style="background-color: rgba(59, 130, 246, 0.1); color: var(--ds-text);">
          {displayKey}
        </span>
      {/if}

      <!-- Priority Badge -->
      {#if showPriority && priority}
        <span class="inline-flex px-2 py-0.5 text-xs font-medium rounded-md flex-shrink-0"
              style="background-color: {priority.color}20; color: {priority.color};">
          {priority.name}
        </span>
      {/if}

      <!-- Title -->
      <h4 class="text-sm flex-1 min-w-0 truncate" style="color: var(--ds-text);">{item.title}</h4>

      <!-- Optional Workspace Name -->
      {#if showWorkspace && item.workspace_name}
        <span class="text-xs min-w-0 truncate" style="color: var(--ds-text-subtle);">{item.workspace_name}</span>
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
          {#if editingStoryPoints}
            <input
              type="number"
              min="0"
              step="0.5"
              aria-label={t('items.storyPoints')}
              aria-invalid={storyPointsError}
              aria-describedby={storyPointsError ? `backlog-story-points-error-${item.id}` : undefined}
              data-testid={`backlog-story-points-input-${item.id}`}
              class="w-16 rounded border px-2 py-1 text-xs tabular-nums outline-none"
              style="background-color: var(--ds-surface-card); border-color: {storyPointsError ? 'var(--ds-text-danger)' : 'var(--ds-border-focused)'}; color: var(--ds-text);"
              bind:value={storyPointsEditValue}
              disabled={storyPointsSaving}
              onclick={(event) => event.stopPropagation()}
              onblur={saveStoryPoints}
              onkeydown={handleStoryPointsKeydown}
            />
            {#if storyPointsError}
              <span id={`backlog-story-points-error-${item.id}`} class="sr-only">
                {t('items.enterField', { field: t('items.storyPoints') })}
              </span>
            {/if}
          {:else}
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded px-2 py-1 text-xs tabular-nums transition-colors hover:bg-black/5 dark:hover:bg-white/10 disabled:cursor-wait disabled:opacity-60"
              style="color: var(--ds-text-subtle);"
              title={t('items.setField', { field: t('items.storyPoints') })}
              aria-label={t('items.setField', { field: t('items.storyPoints') })}
              data-testid={`backlog-story-points-button-${item.id}`}
              disabled={storyPointsSaving}
              onclick={startEditingStoryPoints}
            >
              <span style="color: var(--ds-text);">{item.story_points ?? t('items.notSet')}</span>
              <span>SP</span>
            </button>
          {/if}
        </div>
      {/if}

      {#if trailing}{@render trailing()}{/if}
    </div>
  {/snippet}
</ItemCard>
