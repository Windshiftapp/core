<script>
  import { onMount } from 'svelte';
  import { notifications, notificationActions } from '../stores/notifications.js';
  import { Bell, Check, Filter, Calendar, MoreHorizontal, X } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';
  import NotificationCard from '../features/notifications/NotificationCard.svelte';
  import Button from '../components/Button.svelte';
  import Select from '../components/Select.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import Card from '../components/Card.svelte';
  import { slide } from 'svelte/transition';
  import PageHeader from '../layout/PageHeader.svelte';

  let filteredNotifications = [];
  let unreadCount = 0;
  let searchQuery = '';
  let selectedType = 'all'; // all, assignment, comment, status_change, reminder, milestone
  let selectedStatus = 'all'; // all, read, unread
  let showFilters = false;

  // Filter options
  const typeOptions = [
    { value: 'all', label: t('notifications.filters.allTypes') },
    { value: 'assignment', label: t('notifications.filters.assignments') },
    { value: 'comment', label: t('notifications.filters.comments') },
    { value: 'status_change', label: t('notifications.filters.statusChanges') },
    { value: 'reminder', label: t('notifications.filters.reminders') },
    { value: 'milestone', label: t('notifications.filters.milestones') }
  ];

  const statusOptions = [
    { value: 'all', label: t('notifications.filters.all') },
    { value: 'unread', label: t('notifications.filters.unreadOnly') },
    { value: 'read', label: t('notifications.filters.readOnly') }
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
    if (confirm(t('dialogs.confirmations.dismissAllNotifications'))) {
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
        title: t('notifications.markAllAsRead'),
        onClick: handleMarkAllRead,
        disabled: unreadCount === 0
      },
      { type: 'divider' },
      {
        id: 'clear-all',
        type: 'regular',
        icon: X,
        title: t('notifications.clearAll'),
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
    <PageHeader
      title={t('notifications.title')}
      subtitle={unreadCount > 0 ? `${t('notifications.subtitle')} • ${unreadCount} unread` : t('notifications.subtitle')}
    >
      {#snippet actions()}
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
      {/snippet}
    </PageHeader>

    <!-- Filters Section -->
    {#if showFilters}
      <div class="mb-6 p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);" transition:slide={{ duration: 200 }}>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <!-- Search -->
          <SearchInput
            bind:value={searchQuery}
            placeholder={t('notifications.searchNotifications')}
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
          <div class="mt-4 pt-4 border-t flex items-center justify-between" style="border-color: var(--ds-border);">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-sm" style="color: var(--ds-text-subtle);">Active filters:</span>
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
              {t('common.clear')}
            </Button>
          </div>
        {/if}
      </div>
    {/if}

    <!-- Results Summary -->
    <div class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      {#if filteredNotifications.length === $notifications.length}
        Showing all {$notifications.length} notification{$notifications.length !== 1 ? 's' : ''}
      {:else}
        Showing {filteredNotifications.length} of {$notifications.length} notification{$notifications.length !== 1 ? 's' : ''}
      {/if}
    </div>

    <!-- Notifications List -->
    {#if filteredNotifications.length === 0}
      <Card rounded="xl" shadow padding="generous" class="text-center">
        <Bell class="w-12 h-12 mx-auto mb-4 opacity-50" style="color: var(--ds-text-subtle);" />
        <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">
          {$notifications.length === 0 ? t('notifications.noNotifications') : t('common.noResults')}
        </h3>
        <p style="color: var(--ds-text-subtle);">
          {#if $notifications.length === 0}
            {t('dashboard.allCaughtUp')}
          {:else if searchQuery || selectedType !== 'all' || selectedStatus !== 'all'}
            {t('search.configureFilter')}
          {:else}
            {t('notifications.noNotifications')}
          {/if}
        </p>
        {#if searchQuery || selectedType !== 'all' || selectedStatus !== 'all'}
          <Button
            onclick={clearFilters}
            variant="primary"
            size="sm"
            class="mt-4"
          >
            {t('common.clear')}
          </Button>
        {/if}
      </Card>
    {:else}
      <!-- Notifications Cards -->
      <Card rounded="xl" shadow padding="none" class="overflow-hidden">
        {#each filteredNotifications as notification, index (notification.id)}
          <div class="notification-wrapper" class:border-b={index < filteredNotifications.length - 1} style={index < filteredNotifications.length - 1 ? 'border-color: var(--ds-border);' : ''}>
            <NotificationCard {notification} />
          </div>
        {/each}
      </Card>

      <!-- Footer Actions -->
      {#if $notifications.length > 10}
        <div class="mt-6 p-4 rounded text-center" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
          <p class="text-sm mb-3" style="color: var(--ds-text-subtle);">
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
              {t('notifications.markAllAsRead')} ({unreadCount})
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