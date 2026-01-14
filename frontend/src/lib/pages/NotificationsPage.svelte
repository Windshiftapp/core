<script>
  import { onMount } from 'svelte';
  import { notifications, notificationActions } from '../stores/notifications.js';
  import { Bell, Check, Filter, Calendar, MoreHorizontal, X } from 'lucide-svelte';
  import NotificationCard from '../features/notifications/NotificationCard.svelte';
  import Button from '../components/Button.svelte';
  import Select from '../components/Select.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import { slide } from 'svelte/transition';

  let filteredNotifications = [];
  let unreadCount = 0;
  let searchQuery = '';
  let selectedType = 'all'; // all, assignment, comment, status_change, reminder, milestone
  let selectedStatus = 'all'; // all, read, unread
  let showFilters = false;

  // Filter options
  const typeOptions = [
    { value: 'all', label: 'All Types' },
    { value: 'assignment', label: 'Assignments' },
    { value: 'comment', label: 'Comments' },
    { value: 'status_change', label: 'Status Changes' },
    { value: 'reminder', label: 'Reminders' },
    { value: 'milestone', label: 'Milestones' }
  ];

  const statusOptions = [
    { value: 'all', label: 'All' },
    { value: 'unread', label: 'Unread Only' },
    { value: 'read', label: 'Read Only' }
  ];

  // Subscribe to notifications store
  onMount(() => {
    const unsubscribe = notifications.subscribe(items => {
      applyFilters(items);
      unreadCount = notificationActions.getUnreadCount(items);
    });

    return unsubscribe;
  });

  function applyFilters(items) {
    filteredNotifications = items.filter(notification => {
      // Search filter
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        if (!notification.title.toLowerCase().includes(query) && 
            !notification.message.toLowerCase().includes(query)) {
          return false;
        }
      }

      // Type filter
      if (selectedType !== 'all' && notification.type !== selectedType) {
        return false;
      }

      // Status filter
      if (selectedStatus === 'read' && !notification.read) {
        return false;
      }
      if (selectedStatus === 'unread' && notification.read) {
        return false;
      }

      return true;
    });
  }

  function handleMarkAllRead() {
    notificationActions.markAllAsRead();
  }

  function handleClearAll() {
    if (confirm('Are you sure you want to dismiss all notifications? This action cannot be undone.')) {
      notifications.set([]);
    }
  }

  function toggleFilters() {
    showFilters = !showFilters;
  }

  function clearFilters() {
    searchQuery = '';
    selectedType = 'all';
    selectedStatus = 'all';
    applyFilters($notifications);
  }

  // Build action menu items
  function buildActionItems() {
    return [
      {
        id: 'mark-all-read',
        type: 'regular',
        icon: Check,
        title: 'Mark All as Read',
        onClick: handleMarkAllRead,
        disabled: unreadCount === 0
      },
      { type: 'divider' },
      {
        id: 'clear-all',
        type: 'regular',
        icon: X,
        title: 'Clear All Notifications',
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50 hover:text-red-700',
        onClick: handleClearAll,
        disabled: $notifications.length === 0
      }
    ];
  }

  // Watch for changes to trigger filtering
  $: applyFilters($notifications);
  $: if (searchQuery !== undefined || selectedType || selectedStatus) {
    applyFilters($notifications);
  }
</script>

<div class="min-h-screen" style="background-color: var(--ds-surface);">
  <!-- Content Container -->
  <div class="p-6">
    <!-- Header -->
    <div class="mb-6 flex items-start justify-between">
      <div>
        <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">
          Notifications
        </h1>
        <p class="text-base" style="color: var(--ds-text-subtle);">
          Stay updated with your work items, comments, and activities
          {#if unreadCount > 0}
            • <span class="font-medium text-blue-600">{unreadCount} unread</span>
          {/if}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <Button
          onclick={toggleFilters}
          variant="secondary"
          size="sm"
        >
          <Filter class="w-4 h-4 mr-2" />
          Filters
        </Button>
        <DropdownMenu
          triggerText=""
          triggerIcon={MoreHorizontal}
          triggerClass="p-2 rounded hover-bg transition-colors"
          items={buildActionItems()}
          placement="bottom-end"
        />
      </div>
    </div>

    <!-- Filters Section -->
    {#if showFilters}
      <div class="mb-6 p-4 bg-gray-50 rounded border" style="border-color: var(--ds-border);" transition:slide={{ duration: 200 }}>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <!-- Search -->
          <SearchInput
            bind:value={searchQuery}
            placeholder="Search notifications..."
            class="w-full"
          />

          <!-- Type Filter -->
          <div>
            <Select bind:value={selectedType} size="small">
              {#each typeOptions as option}
                <option value={option.value}>{option.label}</option>
              {/each}
            </Select>
          </div>

          <!-- Status Filter -->
          <div>
            <Select bind:value={selectedStatus} size="small">
              {#each statusOptions as option}
                <option value={option.value}>{option.label}</option>
              {/each}
            </Select>
          </div>
        </div>

        <!-- Active Filters & Clear -->
        {#if searchQuery || selectedType !== 'all' || selectedStatus !== 'all'}
          <div class="mt-4 pt-4 border-t border-gray-200 flex items-center justify-between">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-sm text-gray-600">Active filters:</span>
              {#if searchQuery}
                <span class="inline-flex items-center gap-1 px-2 py-1 bg-blue-100 text-blue-800 text-xs rounded-md">
                  Search: "{searchQuery}"
                  <button onclick={() => searchQuery = ''} class="hover:bg-blue-200 rounded">
                    <X class="w-3 h-3" />
                  </button>
                </span>
              {/if}
              {#if selectedType !== 'all'}
                <span class="inline-flex items-center gap-1 px-2 py-1 bg-green-100 text-green-800 text-xs rounded-md">
                  {typeOptions.find(opt => opt.value === selectedType)?.label}
                  <button onclick={() => selectedType = 'all'} class="hover:bg-green-200 rounded">
                    <X class="w-3 h-3" />
                  </button>
                </span>
              {/if}
              {#if selectedStatus !== 'all'}
                <span class="inline-flex items-center gap-1 px-2 py-1 bg-purple-100 text-purple-800 text-xs rounded-md">
                  {statusOptions.find(opt => opt.value === selectedStatus)?.label}
                  <button onclick={() => selectedStatus = 'all'} class="hover:bg-purple-200 rounded">
                    <X class="w-3 h-3" />
                  </button>
                </span>
              {/if}
            </div>
            <Button
              onclick={clearFilters}
              variant="secondary"
              size="xs"
            >
              Clear All
            </Button>
          </div>
        {/if}
      </div>
    {/if}

    <!-- Results Summary -->
    <div class="mb-4 text-sm text-gray-600">
      {#if filteredNotifications.length === $notifications.length}
        Showing all {$notifications.length} notification{$notifications.length !== 1 ? 's' : ''}
      {:else}
        Showing {filteredNotifications.length} of {$notifications.length} notification{$notifications.length !== 1 ? 's' : ''}
      {/if}
    </div>

    <!-- Notifications List -->
    {#if filteredNotifications.length === 0}
      <div class="rounded-xl border shadow-sm p-12 text-center" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <Bell class="w-12 h-12 text-gray-400 mx-auto mb-4 opacity-50" />
        <h3 class="text-lg font-medium text-gray-900 mb-2">
          {$notifications.length === 0 ? 'No notifications' : 'No matching notifications'}
        </h3>
        <p class="text-gray-500">
          {#if $notifications.length === 0}
            You're all caught up! New notifications will appear here.
          {:else if searchQuery || selectedType !== 'all' || selectedStatus !== 'all'}
            Try adjusting your filters to see more notifications.
          {:else}
            Check back later for new notifications.
          {/if}
        </p>
        {#if searchQuery || selectedType !== 'all' || selectedStatus !== 'all'}
          <Button
            onclick={clearFilters}
            variant="primary"
            size="sm"
            class="mt-4"
          >
            Clear Filters
          </Button>
        {/if}
      </div>
    {:else}
      <!-- Notifications Cards -->
      <div class="rounded-xl border shadow-sm overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        {#each filteredNotifications as notification, index (notification.id)}
          <div class="notification-wrapper" class:border-b={index < filteredNotifications.length - 1} class:border-gray-100={index < filteredNotifications.length - 1}>
            <NotificationCard {notification} />
          </div>
        {/each}
      </div>

      <!-- Footer Actions -->
      {#if $notifications.length > 10}
        <div class="mt-6 p-4 bg-gray-50 rounded text-center" style="border-color: var(--ds-border);">
          <p class="text-sm text-gray-600 mb-3">
            Showing {Math.min(filteredNotifications.length, 50)} notifications. 
            {#if $notifications.length > 50}
              Older notifications are automatically archived.
            {/if}
          </p>
          {#if unreadCount > 0}
            <Button
              onclick={handleMarkAllRead}
              variant="secondary"
              size="sm"
            >
              <Check class="w-4 h-4 mr-2" />
              Mark All as Read ({unreadCount})
            </Button>
          {/if}
        </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .notification-wrapper {
    transition: all 0.2s ease;
  }

  .notification-wrapper:hover {
    background-color: var(--ds-background-neutral-hovered);
  }

</style>