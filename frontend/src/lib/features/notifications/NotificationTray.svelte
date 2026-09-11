<script>
  import { onMount, onDestroy } from 'svelte';
  import { Bell, Check, X } from '@lucide/svelte';
  import { notifications, notificationActions } from '../../stores/notifications.js';
  import NotificationCard from '../notifications/NotificationCard.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { scale, fly } from 'svelte/transition';
  import { quintOut } from 'svelte/easing';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { createPopover, melt } from '@melt-ui/svelte';
  import { toStore } from 'svelte/store';

  const AUTO_SEEN_DELAY_MS = 5000;

  let {
    expanded = false,
    label = '',
    isOpen = $bindable(false),
    onOpenChange = null,
  } = $props();

  let unreadCount = $state(0);

  // Portal above the sidebar stacking context.
  const controlledOpen = toStore(
    () => isOpen,
    (value) => {
      if (isOpen === value) return;
      isOpen = value;
      onOpenChange?.(value);
    }
  );

  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    forceVisible: true,
    open: controlledOpen,
    positioning: {
      strategy: 'fixed',
      fitViewport: true,
      placement: 'right-start'
    },
    portal: 'body'
  });

  // Track unread notifications.
  let unsubscribe;
  onMount(() => {
    unsubscribe = notifications.subscribe(items => {
      unreadCount = notificationActions.getUnreadCount(items);
    });
  });

  // Mark seen after a sustained open, but never read: email batching still
  // depends on unread rows. Closing early cancels the timer.
  let autoSeenTimer;
  let unsubscribeOpen = open.subscribe((isOpen) => {
    clearTimeout(autoSeenTimer);
    autoSeenTimer = null;
    if (isOpen) {
      autoSeenTimer = setTimeout(() => {
        notificationActions.markAllAsSeen();
      }, AUTO_SEEN_DELAY_MS);
    }
  });

  onDestroy(() => {
    if (unsubscribe) unsubscribe();
    if (unsubscribeOpen) unsubscribeOpen();
    clearTimeout(autoSeenTimer);
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
    data-testid="notifications-trigger"
    class="w-full px-3 h-10 rounded flex items-center justify-start cursor-pointer nav-button {$open ? 'nav-button-selected' : ''}"
    title={t('notifications.title')}
    aria-label={t('notifications.title')}
  >
    <span class="relative">
      <Bell class="w-5 h-5 flex-shrink-0" />

      <!-- Unread count badge -->
      {#if unreadCount > 0}
        <span
          class="absolute -top-1 -right-1 text-xs font-bold rounded-full min-w-[18px] h-[18px] flex items-center justify-center"
          style="background-color: var(--ds-danger); color: var(--ds-text-inverse);"
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
      data-testid="notifications-menu"
      tabindex="-1"
      class="notification-dropdown z-[60] w-96 rounded shadow-xl flex flex-col overflow-hidden"
      style="background-color: var(--ds-surface-overlay); border: 1px solid var(--ds-border); color: var(--ds-text);"
      in:fly={{ x: -10, duration: 200, easing: quintOut }}
      out:fly={{ x: -10, duration: 150 }}
    >
      <!-- Header -->
      <div class="p-4 shrink-0 flex items-center justify-between" style="border-bottom: 1px solid var(--ds-border);">
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
      <div data-testid="notifications-scroll" class="min-h-0 max-h-96 overflow-y-auto overscroll-contain">
        {#if $notifications.length === 0}
          <EmptyState
            icon={Bell}
            title={t('notifications.noNotifications')}
            description={t('notifications.allCaughtUp')}
          />
        {:else}
          {#each $notifications as notification (notification.id)}
            <div
              in:fly={{ x: 20, duration: 200, easing: quintOut }}
              out:fly={{ x: -20, duration: 150 }}
            >
              <NotificationCard
                {notification}
                onclose={closeDropdown}
              />
            </div>
          {/each}
        {/if}
      </div>

      <!-- Footer -->
      {#if $notifications.length > 0}
        <div class="p-3 shrink-0 text-center" style="border-top: 1px solid var(--ds-border);">
          <button
            class="text-sm font-medium view-all-btn"
            data-testid="notifications-view-all"
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
    background-color: var(--ds-background-neutral-hovered);
  }

  .view-all-btn {
    color: var(--ds-link);
  }

  .view-all-btn:hover {
    color: var(--ds-link-pressed);
  }
</style>
