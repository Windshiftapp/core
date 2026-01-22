<script>
  import StateDisplay from './StateDisplay.svelte';

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
    message = '',
    size = 'md', // 'sm' | 'md' | 'lg'
    inline = false, // horizontal layout
    class: className = ''
  } = $props();
</script>

<StateDisplay
  type="loading"
  {message}
  {size}
  {inline}
  class={className}
/>
