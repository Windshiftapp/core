<script>
  import { createEventDispatcher } from 'svelte';
  import { X, MessageCircle, UserPlus, Activity, Clock, Target } from 'lucide-svelte';
  import { notificationActions } from '../../stores/notifications.js';
  import { navigate } from '../../router.js';

  export let notification;

  const dispatch = createEventDispatcher();

  function getNotificationIcon(type) {
    const iconMap = {
      assignment: UserPlus,
      comment: MessageCircle,
      status_change: Activity,
      reminder: Clock,
      milestone: Target
    };
    return iconMap[type] || Activity;
  }

  function getNotificationIconStyle(type) {
    const colorMap = {
      assignment: 'color: var(--ds-accent-blue);',
      comment: 'color: var(--ds-accent-green);',
      status_change: 'color: var(--ds-text-warning);',
      reminder: 'color: var(--ds-text-danger);',
      milestone: 'color: var(--ds-accent-purple);'
    };
    return colorMap[type] || 'color: var(--ds-text-subtle);';
  }

  function handleClick() {
    // Mark as read when clicked
    if (!notification.read) {
      notificationActions.markAsRead(notification.id);
    }
    
    // Navigate to the action URL if provided
    if (notification.actionUrl) {
      navigate(notification.actionUrl);
      dispatch('close'); // Close the notification dropdown
    }
  }

  function handleDismiss(e) {
    e.stopPropagation(); // Prevent triggering the click handler
    notificationActions.dismiss(notification.id);
  }
</script>

<div
  class="notification-card p-4 cursor-pointer transition-colors relative group"
  class:unread={!notification.read}
  style="border-bottom: 1px solid var(--ds-border);"
  onclick={handleClick}
  role="button"
  tabindex="0"
  onkeydown={(e) => e.key === 'Enter' && handleClick()}
>
  <div class="flex items-start gap-3">
    <!-- Avatar or Icon -->
    <div class="flex-shrink-0">
      {#if notification.avatar}
        <div class="w-8 h-8 text-white rounded-full flex items-center justify-center text-sm font-medium" style="background-color: var(--ds-background-brand-bold);">
          {notification.avatar}
        </div>
      {:else}
        <div class="w-8 h-8 rounded-full flex items-center justify-center" style="background-color: var(--ds-interactive-subtle);">
          <svelte:component this={getNotificationIcon(notification.type)} class="w-4 h-4" style={getNotificationIconStyle(notification.type)} />
        </div>
      {/if}
    </div>

    <!-- Content -->
    <div class="flex-1 min-w-0">
      <div class="flex items-start justify-between gap-2">
        <div class="flex-1 min-w-0">
          <h4 class="text-sm font-medium mb-1 line-clamp-1" style="color: var(--ds-text);">
            {notification.title}
          </h4>
          <p class="text-sm line-clamp-2 leading-relaxed" style="color: var(--ds-text-subtle);">
            {notification.message}
          </p>
          <div class="mt-2 text-xs" style="color: var(--ds-text-subtlest);">
            {notificationActions.formatTimestamp(notification.timestamp)}
          </div>
        </div>

        <!-- Dismiss button (shows on hover) -->
        <button
          onclick={handleDismiss}
          class="opacity-0 group-hover:opacity-100 transition-opacity p-1 rounded dismiss-btn"
          title="Dismiss notification"
          tabindex="-1"
        >
          <X class="w-3 h-3" style="color: var(--ds-text-subtle);" />
        </button>
      </div>
    </div>

    <!-- Unread indicator -->
    {#if !notification.read}
      <div class="absolute left-0 top-1/2 transform -translate-y-1/2 w-1 h-8 rounded-r" style="background-color: var(--ds-background-brand-bold);"></div>
    {/if}
  </div>
</div>

<style>
  .notification-card:hover {
    background-color: var(--ds-surface-hovered);
  }

  .notification-card.unread {
    background-color: var(--ds-background-selected);
  }

  .dismiss-btn:hover {
    background-color: var(--ds-surface-hovered);
  }

  .line-clamp-1 {
    display: -webkit-box;
    -webkit-line-clamp: 1;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
</style>