<script>
  import { Building2, Globe } from '@lucide/svelte';

  let { iteration, interactive = false } = $props();
</script>

<span class="flex items-center gap-2 text-sm {interactive ? 'cursor-pointer' : ''}" style="color: var(--ds-text);">
  {#if iteration.is_global}
    <Globe class="w-4 h-4" style="color: var(--ds-text-subtle);" />
  {:else}
    <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
  {/if}
  {iteration.name}
</span>
