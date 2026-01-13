<script>
  import Spinner from './Spinner.svelte';

  /**
   * LoadingState - Centered loading spinner with optional message
   *
   * @example
   * <LoadingState />
   *
   * @example with custom message
   * <LoadingState message="Loading items..." />
   *
   * @example inline
   * <LoadingState inline size="sm" message="Saving..." />
   */
  let {
    message = 'Loading...',
    size = 'md', // 'sm' | 'md' | 'lg'
    inline = false, // horizontal layout
    class: className = ''
  } = $props();
</script>

{#if inline}
  <div class="flex items-center gap-2 {className}">
    <Spinner {size} />
    {#if message}
      <span class="text-sm" style="color: var(--ds-text-subtle);">{message}</span>
    {/if}
  </div>
{:else}
  <div class="flex flex-col items-center justify-center py-8 {className}">
    <Spinner {size} />
    {#if message}
      <p class="mt-3 text-sm" style="color: var(--ds-text-subtle);">{message}</p>
    {/if}
  </div>
{/if}
