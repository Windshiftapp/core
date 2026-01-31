<script>
  import { X } from 'lucide-svelte';

  let {
    gradients = [],
    selectedIndex = 0,
    hasBackgroundImage = false,
    onSelect = () => {},
    columns = 6,
    size = 25
  } = $props();
</script>

<div class="grid gap-3" style="grid-template-columns: repeat({columns}, minmax(0, 1fr));">
  {#each gradients as gradient, index}
    <button
      onclick={() => onSelect(index)}
      class="group relative rounded overflow-hidden transition-all hover:scale-110"
      style="width: {size}px; height: {size}px; {selectedIndex === index && !hasBackgroundImage ? 'box-shadow: 0 0 0 2px var(--ds-border-focused); outline-offset: 2px;' : ''}"
      title={gradient.name}
    >
      {#if index === 0 || !gradient.value}
        <!-- None option -->
        <div class="w-full h-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
          <X class="w-3 h-3" style="color: var(--ds-text-subtle);" />
        </div>
      {:else}
        <div class="w-full h-full" style="background: {gradient.value};"></div>
      {/if}

      <!-- Selected Indicator -->
      {#if selectedIndex === index && !hasBackgroundImage}
        <div class="absolute inset-0 flex items-center justify-center bg-black/20">
          <div class="w-3 h-3 bg-white rounded-full flex items-center justify-center">
            <svg class="w-2 h-2" style="color: var(--ds-icon-accent-blue);" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
            </svg>
          </div>
        </div>
      {/if}

      <!-- Dimmed state when background image is selected -->
      {#if hasBackgroundImage && index !== 0}
        <div class="absolute inset-0 bg-white/50 dark:bg-black/50"></div>
      {/if}
    </button>
  {/each}
</div>
