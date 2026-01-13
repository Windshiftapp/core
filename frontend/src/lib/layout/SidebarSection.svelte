<script>
  import { ChevronDown } from 'lucide-svelte';

  let {
    title = '',
    expanded = true,
    collapsible = false,
    class: className = '',
    children
  } = $props();

  let isExpanded = $state(expanded);

  function toggle() {
    if (collapsible) {
      isExpanded = !isExpanded;
    }
  }
</script>

<div class="py-2 {className}">
  {#if title}
    <button
      class="w-full flex items-center justify-between px-3 py-2 text-sm font-medium transition-colors"
      class:cursor-pointer={collapsible}
      class:cursor-default={!collapsible}
      style="color: var(--ds-text);"
      onclick={toggle}
      type="button"
    >
      <span class="truncate">{title}</span>
      {#if collapsible}
        <ChevronDown
          class="w-4 h-4 flex-shrink-0 transition-transform duration-200 {isExpanded ? 'rotate-180' : ''}"
          style="color: var(--ds-text-subtle);"
        />
      {/if}
    </button>
  {/if}

  {#if isExpanded}
    <div class="space-y-1 px-2">
      {@render children()}
    </div>
  {/if}
</div>
