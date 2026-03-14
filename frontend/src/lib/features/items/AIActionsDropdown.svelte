<script>
  import { Sparkles, BookOpen, Search, GitBranch, ChevronDown } from 'lucide-svelte';
  import { onClickOutside } from 'runed';

  let {
    item = null,
    availableSubIssueTypes = [],
    canCreate = false,
    onaiaction,
  } = $props();

  let showMenu = $state(false);
  let menuRef = $state(null);

  onClickOutside(
    () => menuRef,
    () => { showMenu = false; }
  );

  function handleAction(action) {
    onaiaction?.({ action });
    showMenu = false;
  }

  let showDecompose = $derived(
    item?.description && availableSubIssueTypes.length > 0 && canCreate
  );
</script>

<div class="relative" bind:this={menuRef}>
  <button
    class="action-btn inline-flex items-center gap-1.5 px-2 py-1.5 rounded text-xs transition-all"
    style="color: var(--ds-text-subtle);"
    onclick={(e) => { e.stopPropagation(); showMenu = !showMenu; }}
    title="AI Actions"
  >
    <Sparkles class="w-4 h-4 flex-shrink-0" />
    <span class="action-label">AI</span>
    <ChevronDown class="w-3 h-3 ml-0.5" />
  </button>

  {#if showMenu}
    <div class="absolute left-0 top-full mt-1 z-50 min-w-[220px] rounded-md shadow-lg py-1" style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);">
      <button
        class="w-full px-3 py-2 text-left text-sm flex items-center gap-2 transition-colors"
        style="color: var(--ds-text);"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
        onclick={() => handleAction('catch-me-up')}
      >
        <BookOpen class="w-4 h-4 flex-shrink-0" style="color: var(--ds-interactive);" />
        Catch me up
      </button>

      <button
        class="w-full px-3 py-2 text-left text-sm flex items-center gap-2 transition-colors"
        style="color: var(--ds-text);"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
        onclick={() => handleAction('find-similar')}
      >
        <Search class="w-4 h-4 flex-shrink-0" style="color: var(--ds-interactive);" />
        Find similar items
      </button>

      {#if showDecompose}
        <button
          class="w-full px-3 py-2 text-left text-sm flex items-center gap-2 transition-colors"
          style="color: var(--ds-text);"
          onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
          onclick={() => handleAction('decompose')}
        >
          <GitBranch class="w-4 h-4 flex-shrink-0" style="color: var(--ds-interactive);" />
          Break down into sub-tasks
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .action-btn {
    overflow: hidden;
    color: var(--ds-text-subtle);
  }
  .action-btn:hover {
    color: var(--ds-text-subtle);
  }
  .action-label {
    max-width: 0;
    opacity: 0;
    overflow: hidden;
    white-space: nowrap;
    transition: max-width 0.2s ease, opacity 0.2s ease;
  }
  .action-btn:hover .action-label {
    max-width: 80px;
    opacity: 1;
  }
</style>
