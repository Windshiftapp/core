<script>
  import { X, Check, Square, CheckSquare } from 'lucide-svelte';
  import { fade, scale } from 'svelte/transition';
  import { backOut } from 'svelte/easing';
  import Spinner from '../components/Spinner.svelte';
  import Button from '../components/Button.svelte';

  let {
    show = $bindable(false),
    title = '',
    icon = null,
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

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) {
      close();
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') {
      close();
    }
  }

  let allSelected = $derived(selected.size === subTasks.length && subTasks.length > 0);
  let selectedCount = $derived(selected.size);
</script>

{#if show}
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 flex items-start justify-center pt-8 overflow-y-auto z-50"
    style="background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(4px);"
    tabindex="-1"
    onclick={handleBackdropClick}
    onkeydown={handleKeydown}
    role="dialog"
    aria-modal="true"
    aria-labelledby="ai-confirm-title"
  >
    <div
      transition:scale={{ duration: 200, start: 0.95, easing: backOut }}
      class="relative rounded-xl overflow-hidden max-w-2xl w-full mx-4 mb-8"
      style="background-color: var(--ds-surface-raised); box-shadow: 0 20px 50px rgba(0, 0, 0, 0.18);"
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-3">
          {#if icon}
            <svelte:component this={icon} class="w-5 h-5" style="color: var(--ds-interactive);" />
          {/if}
          <h3 id="ai-confirm-title" class="text-lg font-semibold" style="color: var(--ds-text);">{title}</h3>
        </div>
        <button
          onclick={close}
          class="p-1.5 rounded transition-colors"
          style="color: var(--ds-text-subtle);"
          onmouseenter={(e) => { e.currentTarget.style.color = 'var(--ds-text)'; e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'; }}
          onmouseleave={(e) => { e.currentTarget.style.color = 'var(--ds-text-subtle)'; e.currentTarget.style.backgroundColor = 'transparent'; }}
          aria-label="Close"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Body -->
      <div class="px-6 py-5 max-h-[60vh] overflow-y-auto">
        {#if loading}
          <div class="flex flex-col items-center justify-center py-12 gap-3">
            <Spinner />
            <p class="text-sm" style="color: var(--ds-text-subtle);">Analyzing...</p>
          </div>
        {:else if error}
          <div class="py-8 text-center">
            <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
          </div>
        {:else}
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
                        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{task.description}</p>
                      {/if}
                    </div>
                  </div>
                </button>
              {/each}
            </div>
          {:else}
            <p class="text-sm py-4 text-center" style="color: var(--ds-text-subtle);">No sub-tasks suggested.</p>
          {/if}
        {/if}
      </div>

      <!-- Footer -->
      {#if !loading && !error && subTasks.length > 0}
        <div class="px-6 py-3 border-t flex justify-end gap-2" style="border-color: var(--ds-border);">
          <button
            onclick={close}
            class="px-4 py-2 text-sm font-medium rounded-md transition-colors"
            style="color: var(--ds-text); background-color: var(--ds-surface); border: 1px solid var(--ds-border);"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
          >
            Cancel
          </button>
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
            class="px-4 py-2 text-sm font-medium rounded-md transition-colors"
            style="color: var(--ds-text); background-color: var(--ds-surface); border: 1px solid var(--ds-border);"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
          >
            Close
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}
