<script>
  import Building2 from '@lucide/svelte/icons/building-2';
  import Globe from '@lucide/svelte/icons/globe';

  let { iteration, interactive = false } = $props();
</script>

<span
  class="flex items-center gap-2 text-sm whitespace-nowrap {interactive ? 'cursor-pointer' : ''}"
  style="color: var(--ds-text);"
  data-testid="iteration-cell-display"
>
  {#if iteration.is_global}
    <Globe class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
  {:else}
    <Building2 class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
  {/if}
  <span data-testid="iteration-cell-value">{iteration.name}</span>
</span>
