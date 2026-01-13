<script>
  import { onMount, onDestroy } from 'svelte';
  import { Bell, Check, X } from 'lucide-svelte';
  import { notifications, notificationActions } from '../../stores/notifications.js';
  import NotificationCard from '../notifications/NotificationCard.svelte';
  import { scale, fly } from 'svelte/transition';
  import { quintOut } from 'svelte/easing';
  import { navigate } from '../../router.js';
  import { onClickOutside } from 'runed';

  let showDropdown = $state(false);
  let unreadCount = $state(0);
  let dropdownElement = $state();
  let buttonElement = $state();
  let shouldShowAbove = $state(false);

  // Subscribe to notifications store
  let unsubscribe;
  onMount(() => {
    unsubscribe = notifications.subscribe(items => {
      unreadCount = notificationActions.getUnreadCount(items);
    });
  });

  onDestroy(() => {
    if (unsubscribe) unsubscribe();
  });

  // Handle click outside using runed
  onClickOutside(
    () => dropdownElement,
    () => { showDropdown = false; }
  );

  function calculatePosition() {
    if (!buttonElement) return;

    const buttonRect = buttonElement.getBoundingClientRect();
    const viewportHeight = window.innerHeight;
    const dropdownHeight = 500; // max-height of dropdown

    // Check if dropdown would go below viewport
    const spaceBelow = viewportHeight - buttonRect.bottom;
    const spaceAbove = buttonRect.top;

    // Show above if there's not enough space below but enough space above
    shouldShowAbove = spaceBelow < dropdownHeight && spaceAbove > dropdownHeight;
  }

  function toggleDropdown() {
    if (!showDropdown) {
      calculatePosition();
    }
    showDropdown = !showDropdown;
  }

  function closeDropdown() {
    showDropdown = false;
  }

  function handleMarkAllRead() {
    notificationActions.markAllAsRead();
  }

  // Handle escape key
  function handleKeydown(event) {
    if (event.key === 'Escape') {
      closeDropdown();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="notification-tray relative" bind:this={dropdownElement}>
  <!-- Notification Bell Button -->
  <button
    bind:this={buttonElement}
    onclick={toggleDropdown}
    class="relative p-2 rounded-full nav-button {showDropdown ? 'nav-button-selected' : ''}"
    title="Notifications"
    aria-label="Notifications"
    aria-expanded={showDropdown}
  >
    <Bell class="w-5 h-5" />
    
    <!-- Unread count badge -->
    {#if unreadCount > 0}
      <span 
        class="absolute -top-1 -right-1 bg-red-500 text-white text-xs font-bold rounded-full min-w-[18px] h-[18px] flex items-center justify-center"
        in:scale={{ duration: 200, easing: quintOut }}
        out:scale={{ duration: 150 }}
      >
        {unreadCount > 99 ? '99+' : unreadCount}
      </span>
    {/if}
  </button>

  <!-- Notification Dropdown -->
  {#if showDropdown}
    <div
      class="notification-dropdown absolute left-full ml-2 w-96 rounded shadow-xl z-[9999] max-h-[500px] overflow-hidden {shouldShowAbove ? 'bottom-0' : 'top-0'}"
      style="background-color: var(--ds-surface-overlay); border: 1px solid var(--ds-border); color: var(--ds-text);"
      in:fly={{ x: -10, duration: 200, easing: quintOut }}
      out:fly={{ x: -10, duration: 150 }}
    >
      <!-- Header -->
      <div class="p-4 flex items-center justify-between" style="border-bottom: 1px solid var(--ds-border); background-color: var(--ds-interactive-subtle);">
        <h3 class="text-lg font-semibold" style="color: var(--ds-text);">Notifications</h3>
        <div class="flex items-center gap-2">
          {#if unreadCount > 0}
            <button
              onclick={handleMarkAllRead}
              class="text-sm font-medium flex items-center gap-1"
              style="color: var(--ds-link);"
              title="Mark all as read"
            >
              <Check class="w-3 h-3" />
              Mark all read
            </button>
          {/if}
          <button
            onclick={closeDropdown}
            class="p-1 rounded transition-colors close-btn"
            title="Close notifications"
          >
            <X class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          </button>
        </div>
      </div>

      <!-- Notifications List -->
      <div class="max-h-96 overflow-y-auto">
        {#if $notifications.length === 0}
          <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
            <Bell class="w-12 h-12 mx-auto mb-3 opacity-30" />
            <p class="text-sm">No notifications</p>
            <p class="text-xs mt-1">You're all caught up!</p>
          </div>
        {:else}
          {#each $notifications as notification (notification.id)}
            <div
              in:fly={{ x: 20, duration: 200, easing: quintOut }}
              out:fly={{ x: -20, duration: 150 }}
            >
              <NotificationCard
                {notification}
                on:close={closeDropdown}
              />
            </div>
          {/each}
        {/if}
      </div>

      <!-- Footer -->
      {#if $notifications.length > 0}
        <div class="p-3 text-center" style="border-top: 1px solid var(--ds-border); background-color: var(--ds-interactive-subtle);">
          <button
            class="text-sm font-medium view-all-btn"
            onclick={() => {
              navigate('/notifications');
              closeDropdown();
            }}
          >
            View all notifications
          </button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .notification-dropdown {
    /* Ensure dropdown appears above everything */
    position: absolute;
    z-index: 9999;
  }

  /* Custom scrollbar for notifications list */
  .notification-dropdown .max-h-96::-webkit-scrollbar {
    width: 6px;
  }

  .notification-dropdown .max-h-96::-webkit-scrollbar-track {
    background: var(--ds-interactive-subtle);
  }

  .notification-dropdown .max-h-96::-webkit-scrollbar-thumb {
    background: var(--ds-border);
    border-radius: 3px;
  }

  .notification-dropdown .max-h-96::-webkit-scrollbar-thumb:hover {
    background: var(--ds-border-bold);
  }

  .close-btn:hover {
    background-color: var(--ds-surface-hovered);
  }

  .view-all-btn {
    color: var(--ds-link);
  }

  .view-all-btn:hover {
    color: var(--ds-link-pressed);
  }
</style>