<script>
  import { IconInbox as Inbox } from '@tabler/icons-svelte-runes';
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
  /**
   * @type {{
   *   icon?: import('svelte').Component<any, any, any>,
   *   title?: string,
   *   description?: string,
   *   message?: string,
   *   action?: import('svelte').Snippet | null,
   *   class?: string,
   * }}
   */
  let {
    icon: IconComponent = Inbox,
    title = '',
    description = '',
    message = '',
    action = null,
    class: className = ''
  } = $props();
</script>

<StateDisplay
  type="empty"
  icon={IconComponent}
  {title}
  description={description || message}
  {action}
  class={className}
/>
