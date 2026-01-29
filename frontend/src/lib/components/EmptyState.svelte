<script>
  import { Inbox } from 'lucide-svelte';
  import StateDisplay from './StateDisplay.svelte';

  /**
   * EmptyState - Empty data display with optional action
   *
   * @example
   * <EmptyState title="No items" description="Add your first item" />
   *
   * @example with action
   * <EmptyState title="No results">
   *   {#snippet action()}<Button>Add item</Button>{/snippet}
   * </EmptyState>
   */
  let {
    icon: IconComponent = Inbox,
    title = '',
    description = '',
    action = null,
    hasGradient = false,
    class: className = ''
  } = $props();
</script>

<StateDisplay
  type="empty"
  icon={IconComponent}
  {title}
  {description}
  {action}
  {hasGradient}
  class={className}
/>
