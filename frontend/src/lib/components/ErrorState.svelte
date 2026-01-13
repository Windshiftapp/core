<script>
  import { AlertCircle, RefreshCw } from 'lucide-svelte';
  import Button from './Button.svelte';

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
    title = 'Something went wrong',
    message = '',
    onRetry = null,
    retryLabel = 'Try Again',
    icon: IconComponent = AlertCircle,
    class: className = ''
  } = $props();
</script>

<div class="text-center py-8 {className}">
  <svelte:component this={IconComponent} class="w-10 h-10 mx-auto mb-3" style="color: var(--ds-icon-danger);" />
  <h3 class="text-lg font-medium mb-1" style="color: var(--ds-text);">{title}</h3>
  {#if message}
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{message}</p>
  {/if}
  {#if onRetry}
    <Button variant="default" onclick={onRetry}>
      <RefreshCw class="w-4 h-4 mr-2" />
      {retryLabel}
    </Button>
  {/if}
</div>
