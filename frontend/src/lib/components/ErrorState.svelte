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
   */
  let {
    title = '',
    message = '',
    onRetry = null,
    retryLabel = '',
    icon: IconComponent = AlertCircle,
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
