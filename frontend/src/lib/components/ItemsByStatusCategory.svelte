<script>
  import { IconChevronDown, IconChevronRight } from '@tabler/icons-svelte-runes';
  import EmptyState from './EmptyState.svelte';
  import { itemUrl } from '../utils/urls.js';

  let {
    statusBreakdown = [],
    itemsByCategory = {},
    expandedCategories = {},
    title = '',
    emptyIcon,
    emptyTitle = '',
    emptyDescription = '',
    ontoggle,
  } = $props();
</script>

<div class="space-y-4">
  <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{title}</h2>

  {#if statusBreakdown && statusBreakdown.length > 0}
    {#each statusBreakdown as category}
      {@const items = itemsByCategory[category.category_name] || []}
      <div class="rounded-xl border overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <button
          onclick={() => ontoggle(category.category_name)}
          class="w-full px-4 py-3 flex items-center justify-between hover:bg-opacity-50 transition-colors"
          style="background-color: var(--ds-background-neutral);"
        >
          <div class="flex items-center gap-3">
            {#if expandedCategories[category.category_name]}
              <IconChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            {:else}
              <IconChevronRight class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            {/if}
            <div
              class="w-3 h-3 rounded-full"
              style="background-color: {category.category_color || '#9ca3af'};"
            ></div>
            <span class="font-medium" style="color: var(--ds-text);">{category.category_name}</span>
            <span class="text-sm" style="color: var(--ds-text-subtle);">({category.item_count} item{category.item_count !== 1 ? 's' : ''})</span>
          </div>
        </button>

        {#if expandedCategories[category.category_name] && items.length > 0}
          <div class="divide-y" style="border-color: var(--ds-border);">
            {#each items as item}
              <a
                href={itemUrl({ workspaceId: item.workspace_id, itemId: item.id })}
                class="w-full px-4 py-3 flex items-center justify-between hover:bg-opacity-50 transition-colors text-left no-underline"
                style="background-color: var(--ds-surface-raised); color: inherit;"
              >
                <div class="flex items-center gap-3 min-w-0">
                  <span class="text-sm font-mono shrink-0" style="color: var(--ds-text-subtle);">
                    {item.workspace_key}-{item.item_number}
                  </span>
                  <span class="truncate" style="color: var(--ds-text);">{item.title}</span>
                </div>
                <div class="flex items-center gap-3 shrink-0">
                  {#if item.priority_name}
                    <span
                      class="text-xs px-2 py-0.5 rounded"
                      style="background-color: {item.priority_color ? item.priority_color + '20' : 'var(--ds-background-neutral)'}; color: {item.priority_color || 'var(--ds-text-subtle)'};"
                    >
                      {item.priority_name}
                    </span>
                  {/if}
                  {#if item.assignee_name}
                    <span class="text-sm" style="color: var(--ds-text-subtle);">{item.assignee_name}</span>
                  {/if}
                </div>
              </a>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  {:else}
    <div class="rounded-xl border p-8" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <EmptyState
        icon={emptyIcon}
        title={emptyTitle}
        description={emptyDescription}
      />
    </div>
  {/if}
</div>
