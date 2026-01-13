<script>
  let {
    variant = 'text',     // 'text' | 'circle' | 'rect'
    width = null,         // CSS width (e.g., '100%', '200px')
    height = null,        // CSS height
    lines = 1,            // Number of lines for text variant
    animate = true,       // Enable pulse animation
    class: className = ''
  } = $props();

  const baseClass = $derived(animate ? 'animate-pulse' : '');
</script>

{#if variant === 'text'}
  <div class="space-y-2 {className}">
    {#each Array(lines) as _, i}
      <div
        class="rounded {baseClass}"
        style="
          background-color: var(--ds-background-neutral);
          height: {height || '1rem'};
          width: {i === lines - 1 && lines > 1 ? '60%' : width || '100%'};
        "
      ></div>
    {/each}
  </div>
{:else if variant === 'circle'}
  <div
    class="rounded-full {baseClass} {className}"
    style="
      background-color: var(--ds-background-neutral);
      width: {width || '2.5rem'};
      height: {height || width || '2.5rem'};
    "
  ></div>
{:else if variant === 'rect'}
  <div
    class="rounded {baseClass} {className}"
    style="
      background-color: var(--ds-background-neutral);
      width: {width || '100%'};
      height: {height || '4rem'};
    "
  ></div>
{/if}
