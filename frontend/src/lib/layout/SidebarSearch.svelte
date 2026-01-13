<script>
  import { Search, X } from 'lucide-svelte';

  let {
    value = $bindable(''),
    placeholder = 'Search...',
    autofocus = false,
    class: className = ''
  } = $props();

  let inputElement = $state(null);

  function clear() {
    value = '';
    inputElement?.focus();
  }

  $effect(() => {
    if (autofocus && inputElement) {
      inputElement.focus();
    }
  });
</script>

<div class="px-3 py-2 {className}">
  <div class="relative">
    <Search
      class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 pointer-events-none"
      style="color: var(--ds-text-subtle);"
    />
    <input
      bind:this={inputElement}
      bind:value
      type="text"
      {placeholder}
      class="w-full pl-10 pr-8 py-2 text-sm rounded-md border focus:outline-none focus:ring-2 focus:ring-blue-500"
      style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
    />
    {#if value}
      <button
        onclick={clear}
        class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded transition-colors"
        style="background: transparent;"
        onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered, rgba(0,0,0,0.05))'}
        onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
        type="button"
        aria-label="Clear search"
      >
        <X class="w-4 h-4" style="color: var(--ds-text-subtle);" />
      </button>
    {/if}
  </div>
</div>
