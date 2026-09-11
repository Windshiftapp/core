<script>
  import ColorDot from '../../components/ColorDot.svelte';

  let { milestone = null, milestones = [], interactive = false } = $props();

  const values = $derived(milestone ? [milestone] : milestones);
</script>

<span class="flex items-center gap-1 flex-wrap text-sm {interactive ? 'cursor-pointer' : ''}" style="color: var(--ds-text);">
  {#each values as value (value.id)}
    <span class="inline-flex items-center gap-1">
      <ColorDot color={value.category_color || '#9CA3AF'} />{value.name}
    </span>
  {/each}
</span>
