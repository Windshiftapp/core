<script>
  import { onMount } from 'svelte';
  import { publicBoard } from '../api/publicBoard.js';
  import { formatRelativeTime, formatDateShort } from '../utils/dateFormatter.js';
  import { itemTypeIconMap } from '../utils/icons.js';
  import { CheckSquare } from '@lucide/svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';
  import Text from '../components/Text.svelte';

  let { slug, itemKey, onclose } = $props();

  let item = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let LazyMilkdownEditor = $state(null);

  let itemTypeIcon = $derived(
    item?.item_type_icon ? (itemTypeIconMap[item.item_type_icon] || CheckSquare) : CheckSquare
  );

  onMount(async () => {
    // Load editor component lazily
    try {
      const mod = await import('../editors/LazyMilkdownEditor.svelte');
      LazyMilkdownEditor = mod.default;
    } catch {
      // Editor failed to load, we'll show plain text fallback
    }

    try {
      item = await publicBoard.getItem(slug, itemKey);
    } catch (err) {
      error = err.status === 404 ? 'not_found' : 'error';
    } finally {
      loading = false;
    }
  });

  function getInitials(name) {
    if (!name) return '?';
    return name.split(' ').map(n => n[0]).join('').slice(0, 2).toUpperCase();
  }
</script>

<Modal isOpen={true} maxWidth="max-w-5xl" {onclose}>
  {#if loading}
    <div class="py-12 text-center">
      <div class="spinner w-7 h-7 border-3 rounded-full mx-auto mb-3" style="border-color: var(--ds-border); border-top-color: #2874BB; animation: spin 0.8s linear infinite;"></div>
      <p class="text-sm" style="color: var(--ds-text-subtle);">Loading...</p>
    </div>
  {:else if error === 'not_found'}
    <div class="py-12 text-center">
      <p class="text-base font-medium mb-1">Item not found</p>
      <p class="text-xs" style="color: var(--ds-text-subtle);">This item doesn't exist or isn't part of this board.</p>
    </div>
  {:else if error}
    <div class="py-12 text-center">
      <p class="text-base font-medium mb-1">Failed to load item</p>
      <p class="text-xs" style="color: var(--ds-text-subtle);">Something went wrong. Please try again.</p>
    </div>
  {:else if item}
    <ModalHeader title={item.title} subtitle={item.key} onClose={onclose} />

    <div class="flex overflow-hidden" style="min-height: 60vh; max-height: 70vh;">
      <!-- Left column: Description + Comments -->
      <div class="flex-1 min-w-0 overflow-y-auto pt-5 pb-5 px-6">
        <!-- Description -->
        <div class="mb-6">
          <h4 class="text-xs font-semibold uppercase tracking-wider mb-2" style="color: var(--ds-text-subtle);">Description</h4>
          {#if item.description}
            {#if LazyMilkdownEditor}
              <div class="rounded-md p-3" style="border: 1px solid var(--ds-border);">
                <LazyMilkdownEditor content={item.description} readonly={true} showToolbar={false} />
              </div>
            {:else}
              <div class="rounded-md p-3 text-sm leading-relaxed whitespace-pre-wrap" style="border: 1px solid var(--ds-border); color: var(--ds-text);">
                {item.description}
              </div>
            {/if}
          {:else}
            <p class="text-xs italic" style="color: var(--ds-text-disabled);">No description</p>
          {/if}
        </div>

        <!-- Comments -->
        <div>
          <h4 class="text-xs font-semibold uppercase tracking-wider mb-3" style="color: var(--ds-text-subtle);">
            Comments {#if item.comments.length > 0}<span class="font-normal">({item.comments.length})</span>{/if}
          </h4>

          {#if item.comments.length === 0}
            <p class="text-xs italic" style="color: var(--ds-text-disabled);">No comments yet</p>
          {:else}
            <div class="flex flex-col gap-4">
              {#each item.comments as comment}
                <div class="flex gap-2.5">
                  <!-- Avatar -->
                  {#if comment.author_avatar}
                    <img src={comment.author_avatar} alt={comment.author_name} class="w-7 h-7 rounded-full object-cover flex-shrink-0" />
                  {:else}
                    <div class="w-7 h-7 rounded-full flex items-center justify-center flex-shrink-0 text-[10px] font-semibold" style="background: #2874BB; color: white;">
                      {getInitials(comment.author_name)}
                    </div>
                  {/if}

                  <div class="flex-1 min-w-0">
                    <!-- Author + time -->
                    <div class="flex items-baseline gap-2 mb-1">
                      <span class="text-[13px] font-semibold">{comment.author_name}</span>
                      <span class="text-[11px]" style="color: var(--ds-text-disabled);">{formatRelativeTime(comment.created_at)}</span>
                    </div>
                    <!-- Content -->
                    {#if LazyMilkdownEditor}
                      <div class="comment-content">
                        <LazyMilkdownEditor content={comment.content} readonly={true} showToolbar={false} compact={true} />
                      </div>
                    {:else}
                      <div class="text-[13px] leading-normal whitespace-pre-wrap" style="color: var(--ds-text);">
                        {comment.content}
                      </div>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </div>

      <!-- Right column: Properties sidebar -->
      <div class="w-72 flex-shrink-0 overflow-y-auto px-4 py-4" style="border-left: 1px solid var(--ds-border); background-color: var(--ds-surface);">
        {#if item.status_name}
          <div class="mb-3">
            <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm rounded">
              <Text variant="subtle" size="sm">Status</Text>
              <div class="flex items-center gap-2">
                <StatusBadge status={{ label: item.status_name, categoryColor: item.status_color }} />
              </div>
            </div>
          </div>
        {/if}

        {#if item.priority_name}
          <div class="mb-3">
            <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm rounded">
              <Text variant="subtle" size="sm">Priority</Text>
              <div class="flex items-center gap-2">
                <span class="text-[13px] px-2 py-0.5 rounded" style="background: {item.priority_color || 'var(--ds-surface-sunken)'}20; color: {item.priority_color || 'var(--ds-text-subtle)'}; border: 1px solid {item.priority_color || 'var(--ds-border)'}40;">
                  {item.priority_name}
                </span>
              </div>
            </div>
          </div>
        {/if}

        {#if item.item_type_name}
          <div class="mb-3">
            <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm rounded">
              <Text variant="subtle" size="sm">Type</Text>
              <div class="flex items-center gap-2">
                <div
                  class="w-5 h-5 rounded flex items-center justify-center flex-shrink-0"
                  style="background-color: {item.item_type_color || 'var(--ds-accent-blue)'};"
                >
                  <svelte:component this={itemTypeIcon} class="w-3 h-3" style="color: white;" />
                </div>
                <span class="text-[13px]">{item.item_type_name}</span>
              </div>
            </div>
          </div>
        {/if}

        {#if item.assignee_name}
          <div class="mb-3">
            <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm rounded">
              <Text variant="subtle" size="sm">Assignee</Text>
              <div class="flex items-center gap-2">
                {#if item.assignee_avatar}
                  <img src={item.assignee_avatar} alt={item.assignee_name} class="w-5.5 h-5.5 rounded-full object-cover" />
                {:else}
                  <div class="w-5.5 h-5.5 rounded-full flex items-center justify-center text-[10px] font-semibold" style="background: #2874BB; color: white;">
                    {getInitials(item.assignee_name)}
                  </div>
                {/if}
                <span class="text-[13px]">{item.assignee_name}</span>
              </div>
            </div>
          </div>
        {/if}

        {#if item.due_date}
          {@const isOverdue = new Date(item.due_date) < new Date()}
          <div class="mb-3">
            <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm rounded">
              <Text variant="subtle" size="sm">Due Date</Text>
              <div class="flex items-center gap-2">
                <span class="text-[13px]" class:text-red-500={isOverdue} class:font-medium={isOverdue}>
                  {formatDateShort(item.due_date)}
                </span>
              </div>
            </div>
          </div>
        {/if}

        {#if item.story_points != null}
          <div class="mb-3">
            <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm rounded">
              <Text variant="subtle" size="sm">Points</Text>
              <div class="flex items-center gap-2">
                <span class="text-[13px]">{item.story_points} SP</span>
              </div>
            </div>
          </div>
        {/if}

        {#if item.labels?.length}
          <div class="mb-3">
            <div class="w-full flex items-start px-2 py-1.5 text-sm rounded">
              <Text variant="subtle" size="sm" class="flex-shrink-0 pt-0.5">Labels</Text>
              <div class="flex flex-wrap gap-1 justify-end ml-auto">
                {#each item.labels as label}
                  <span class="text-[11px] px-2 py-0.5 rounded" style="background: {label.color}20; color: {label.color}; border: 1px solid {label.color}40;">
                    {label.name}
                  </span>
                {/each}
              </div>
            </div>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</Modal>

<style>
  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .comment-content :global(.milkdown-editor) {
    min-height: auto !important;
  }
</style>
