<script>
  import { onMount, onDestroy } from 'svelte';
  import { Bell, Check, X } from 'lucide-svelte';
  import { notifications, notificationActions } from '../../stores/notifications.js';
  import NotificationCard from '../notifications/NotificationCard.svelte';
  import { scale, fly } from 'svelte/transition';
  import { quintOut } from 'svelte/easing';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { createPopover, melt } from '@melt-ui/svelte';

  let {
    expanded = false,
    label = ''
  } = $props();

  let unreadCount = $state(0);

  // Create popover with portal to body to escape sidebar stacking context
  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    forceVisible: true,
    positioning: {
      placement: 'right-start'
    },
    portal: 'body'
  });

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

  function closeDropdown() {
    open.set(false);
  }

  function handleMarkAllRead() {
    notificationActions.markAllAsRead();
  }

  // Handle escape key
  function handleKeydown(event) {
    if (event.key === 'Escape' && $open) {
      closeDropdown();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="notification-tray relative">
  <!-- Notification Bell Button -->
  <button
    use:melt={$trigger}
    class="{expanded ? 'w-full px-3' : 'w-10 justify-center'} h-10 rounded flex items-center cursor-pointer nav-button {$open ? 'nav-button-selected' : ''}"
    title={t('notifications.title')}
    aria-label={t('notifications.title')}
  >
    <span class="relative">
      <Bell class="w-5 h-5 flex-shrink-0" />

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
    </span>
    {#if expanded && label}
      <span class="ml-3 text-sm whitespace-nowrap">{label}</span>
    {/if}
  </button>

  <!-- Notification Dropdown -->
  {#if $open}
    <div
      use:melt={$content}
      class="notification-dropdown w-96 rounded shadow-xl max-h-[500px] overflow-hidden"
      style="background-color: var(--ds-surface-overlay); border: 1px solid var(--ds-border); color: var(--ds-text);"
      in:fly={{ x: -10, duration: 200, easing: quintOut }}
      out:fly={{ x: -10, duration: 150 }}
    >
      <!-- Header -->
      <div class="p-4 flex items-center justify-between" style="border-bottom: 1px solid var(--ds-border); background-color: var(--ds-interactive-subtle);">
        <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{t('notifications.title')}</h3>
        <div class="flex items-center gap-2">
          {#if unreadCount > 0}
            <button
              onclick={handleMarkAllRead}
              class="text-sm font-medium flex items-center gap-1"
              style="color: var(--ds-link);"
              title={t('notifications.markAllAsRead')}
            >
              <Check class="w-3 h-3" />
              {t('notifications.markAllRead')}
            </button>
          {/if}
          <button
            onclick={closeDropdown}
            class="p-1 rounded transition-colors close-btn"
            title={t('notifications.closeNotifications')}
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
            <p class="text-sm">{t('notifications.noNotifications')}</p>
            <p class="text-xs mt-1">{t('notifications.allCaughtUp')}</p>
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
            {t('notifications.viewAll')}
          </button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
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