<script>
  import Square from '@lucide/svelte/icons/square';
  import CheckSquare from '@lucide/svelte/icons/square-check';
  import Button from '../components/Button.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';
  import AIModalShell from '../components/AIModalShell.svelte';

  let {
    show = $bindable(false),
    title = '',
    icon: Icon = null,
    loading = false,
    error = null,
    subTasks = [],
    reasoning = '',
    creating = false,
    onclose = null,
    oncreate = null,
  } = $props();

  let selected = $state(new Set());

  // Reset selection when subTasks change
  $effect(() => {
    if (subTasks.length > 0) {
      selected = new Set(subTasks.map((_, i) => i));
    }
  });

  function toggleItem(index) {
    const next = new Set(selected);
    if (next.has(index)) {
      next.delete(index);
    } else {
      next.add(index);
    }
    selected = next;
  }

  function toggleAll() {
    if (selected.size === subTasks.length) {
      selected = new Set();
    } else {
      selected = new Set(subTasks.map((_, i) => i));
    }
  }

  function close() {
    show = false;
    onclose?.();
  }

  function handleCreate() {
    const selectedTasks = subTasks.filter((_, i) => selected.has(i));
    oncreate?.(selectedTasks);
  }

  let allSelected = $derived(selected.size === subTasks.length && subTasks.length > 0);
  let selectedCount = $derived(selected.size);
</script>

<AIModalShell
  bind:show
  {title}
  icon={Icon}
  {loading}
  {error}
  titleId="ai-confirm-title"
  {onclose}
>
  {#snippet body()}
    {#if reasoning}
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{reasoning}</p>
    {/if}

    {#if subTasks.length > 0}
      <!-- Select all toggle -->
      <div class="flex items-center gap-2 mb-3 pb-2 border-b" style="border-color: var(--ds-border);">
        <button
          class="inline-flex items-center gap-2 text-xs font-medium transition-colors"
          style="color: var(--ds-text-subtle);"
          onclick={toggleAll}
        >
          {#if allSelected}
            <CheckSquare class="w-4 h-4" style="color: var(--ds-interactive);" />
          {:else}
            <Square class="w-4 h-4" />
          {/if}
          {allSelected ? 'Deselect all' : 'Select all'}
        </button>
        <span class="text-xs" style="color: var(--ds-text-subtle);">({selectedCount} of {subTasks.length} selected)</span>
      </div>

      <!-- Task list -->
      <div class="space-y-2">
        {#each subTasks as task, i}
          <button
            class="w-full text-left p-3 rounded-lg border transition-colors"
            style="border-color: {selected.has(i) ? 'var(--ds-interactive)' : 'var(--ds-border)'}; background-color: {selected.has(i) ? 'var(--ds-surface-selected)' : 'var(--ds-surface)'};"
            onclick={() => toggleItem(i)}
          >
            <div class="flex items-start gap-3">
              <div class="flex-shrink-0 mt-0.5">
                {#if selected.has(i)}
                  <CheckSquare class="w-4 h-4" style="color: var(--ds-interactive);" />
                {:else}
                  <Square class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                {/if}
              </div>
              <div class="flex-1 min-w-0">
                <p class="text-sm font-medium" style="color: var(--ds-text);">{task.title}</p>
                {#if task.description}
                  <DescriptionText>{task.description}</DescriptionText>
                {/if}
              </div>
            </div>
          </button>
        {/each}
      </div>
    {:else}
      <p class="text-sm py-4 text-center" style="color: var(--ds-text-subtle);">No sub-tasks suggested.</p>
    {/if}
  {/snippet}

  {#snippet footer()}
    {#if !loading && !error && subTasks.length > 0}
      <div class="px-6 py-3 border-t flex justify-end gap-2" style="border-color: var(--ds-border);">
        <button
          onclick={close}
          class="secondary-btn px-4 py-2 text-sm font-medium rounded-md transition-colors"
        >
          Cancel
        </button>
        <!-- shortcut-guard-exempt: rendered inside AIModalShell's ModalBackdrop, which lives in another file so the modal-range scan can't see it. -->
        <Button
          variant="primary"
          onclick={handleCreate}
          disabled={selectedCount === 0}
          loading={creating}
        >
          Create Selected ({selectedCount})
        </Button>
      </div>
    {:else}
      <div class="px-6 py-3 border-t flex justify-end" style="border-color: var(--ds-border);">
        <button
          onclick={close}
          class="secondary-btn px-4 py-2 text-sm font-medium rounded-md transition-colors"
        >
          Close
        </button>
      </div>
    {/if}
  {/snippet}
</AIModalShell>

<style>
  /* Secondary modal action: neutral surface that lifts on hover. */
  .secondary-btn {
    color: var(--ds-text);
    background-color: var(--ds-surface);
    border: 1px solid var(--ds-border);
  }

  .secondary-btn:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
