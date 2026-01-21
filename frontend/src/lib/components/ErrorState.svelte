<script>
  import { AlertCircle } from 'lucide-svelte';
  import StateDisplay from './StateDisplay.svelte';

  /**
   * ErrorState - Error display with optional retry button
   *
   * @example
   * <ErrorState title="Failed to load" message="Please try again" />
   *
   * @example with retry
   * <ErrorState
   *   title="Connection error"
   *   message="Could not reach the server"
   *   onRetry={() => loadData()}
   * />
   *
   * @example custom icon
   * <ErrorState title="Not found" icon={FileX} />
   *
   * @example fill viewport with custom action
   * <ErrorState title="Not found" fill={true}>
   *   {#snippet action()}
   *     <button onclick={handleClose}>Close</button>
   *   {/snippet}
   * </ErrorState>
   */
  let {
    title = '',
    message = '',
    onRetry = null,
    retryLabel = '',
    icon: IconComponent = AlertCircle,
    fill = false,
    action = null,
    class: className = ''
  } = $props();
</script>

<StateDisplay
  type="error"
  icon={IconComponent}
  {title}
  {message}
  {onRetry}
  {retryLabel}
  class={className}
/>
