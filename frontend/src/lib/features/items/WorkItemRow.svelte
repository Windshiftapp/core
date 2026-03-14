<script>
  import { CheckSquare } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import { formatDateSimple } from '../../utils/dateFormatter.js';
  import ItemCard from './ItemCard.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { getStatusCategory } from '../../utils/statusColors.js';

  /**
   * WorkItemRow - A reusable component for displaying work items in list views
   *
   * @prop {Object} item - The work item object (id, title, workspace_id, workspace_key, workspace_item_number, item_type_id, status_id, priority_id)
   * @prop {Object} workspace - Optional workspace object for key lookup (fallback if item.workspace_key is missing)
   * @prop {Array} itemTypes - Array of item types for icon lookup (optional)
   * @prop {Array} statuses - Array of statuses for status lookup (optional)
   * @prop {Array} priorities - Array of priorities for priority lookup (optional)
   * @prop {Array} statusCategories - Array of status categories for color lookup (optional)
   * @prop {string} href - Optional link URL (defaults to /workspaces/{workspace_id}/items/{id})
   * @prop {Function} onclick - Optional click handler (for modal pattern, disables href)
   * @prop {boolean} showIcon - Show item type icon (default: true)
   * @prop {boolean} showKey - Show item key (default: true)
   * @prop {boolean} showWorkspace - Show workspace name (default: false)
   * @prop {boolean} showTimestamp - Show timestamp (default: false)
   * @prop {boolean} showStatus - Show status badge (default: false)
   * @prop {boolean} showPriority - Show priority badge (default: false)
   * @prop {string|Date} timestamp - The timestamp to display
   * @prop {Function} formatTimestamp - Optional custom formatter for timestamp
   * @prop {boolean} compact - Use compact styling (default: false)
   * @prop {boolean} hasGradient - Enable glass effect (default: false)
   * @prop {Snippet} leading - Content to render before icon (e.g., drag handle, checkbox)
   * @prop {Snippet} trailing - Content to render after title (e.g., custom badges, actions)
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
    hasGradient = false,
    leading = null,
    trailing = null,
  } = $props();

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
</script>

<ItemCard href={itemHref} {onclick} {compact} {hasGradient}>
  {#snippet children()}
    <div class="flex items-center gap-3">
      {#if leading}{@render leading()}{/if}

      <!-- Item Type Icon -->
      {#if showIcon}
        {#if itemType}
          <div
            class="w-5 h-5 rounded flex items-center justify-center flex-shrink-0"
            style="background-color: {itemType.color};"
            title={itemType.name}
          >
            {@const RowTypeIcon = itemTypeIconMap[itemType.icon] || itemTypeIconMap.FileText}
            <RowTypeIcon class="w-3 h-3" style="color: white;" />
          </div>
        {:else}
          <div class="w-5 h-5 rounded flex items-center justify-center flex-shrink-0" style="background-color: var(--ds-accent-blue);">
            <CheckSquare class="w-3 h-3" style="color: white;" />
          </div>
        {/if}
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
        <span class="text-xs flex-shrink-0" style="color: var(--ds-text-subtle);">{item.workspace_name}</span>
      {/if}

      <!-- Optional Timestamp -->
      {#if showTimestamp && formattedTimestamp}
        <span class="text-xs flex-shrink-0" style="color: var(--ds-text-subtle);">{formattedTimestamp}</span>
      {/if}

      <!-- Status Badge -->
      {#if showStatus && status}
        <Lozenge text={status.name.replace(/_/g, ' ')} customBg={statusCategory?.color || '#6b7280'} />
      {/if}

      {#if trailing}{@render trailing()}{/if}
    </div>
  {/snippet}
</ItemCard>
