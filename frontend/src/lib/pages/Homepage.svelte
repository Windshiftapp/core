<script>
  import { onMount } from 'svelte';
  import { authStore, homepageStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import DashboardOnboarding from './DashboardOnboarding.svelte';
  import ColorDot from '../components/ColorDot.svelte';
  import ItemCard from '../features/items/ItemCard.svelte';
  import WorkItemRow from '../features/items/WorkItemRow.svelte';
  import Text from '../components/Text.svelte';
  import { Clock, Eye, Edit, MessageSquare, Bookmark, Bell, Briefcase, Calendar, Target, Search, Grip, Info, CheckCircle, AlertCircle, XCircle } from 'lucide-svelte';
  import { workspaceIconMap } from '../utils/icons.js';
  import { useEventListener } from 'runed';

  // Use store values with aliases for easier template access
  let greeting = $derived(homepageStore.greeting);
  let currentDate = $derived(homepageStore.currentDate);
  let recentWorkspaces = $derived(homepageStore.recentWorkspaces);
  let totalWorkspaceCount = $derived(homepageStore.totalWorkspaceCount);
  let totalItemCount = $derived(homepageStore.totalItemCount);
  let watchedItems = $derived(homepageStore.watchedItems);
  let loading = $derived(homepageStore.loading);
  let recentlyViewed = $derived(homepageStore.recentlyViewed);
  let recentlyEdited = $derived(homepageStore.recentlyEdited);
  let recentlyCommented = $derived(homepageStore.recentlyCommented);
  let upcomingMilestones = $derived(homepageStore.upcomingMilestones);
  let notifications = $derived(homepageStore.notifications);
  let activeTab = $derived(homepageStore.activeTab);
  let isOnboarding = $derived(homepageStore.isOnboarding);

  function handleOnboardingDismiss() {
    homepageStore.dismissOnboarding();
  }

  function setActiveTab(tab) {
    homepageStore.setActiveTab(tab);
  }

  onMount(async () => {
    const userTimeZone = authStore.currentUser?.timezone || 'UTC';
    await homepageStore.init(userTimeZone);
  });

  function handleRefreshWorkspaces() {
    homepageStore.refresh();
  }

  function handleRefreshWorkItems() {
    homepageStore.refresh();
  }

  // Listen for workspace/work-item refresh events using runed
  useEventListener(() => window, 'refresh-workspaces', handleRefreshWorkspaces);
  useEventListener(() => window, 'refresh-work-items', handleRefreshWorkItems);

  function getNotificationIcon(type) {
    switch(type) {
      case 'assignment': return Bell;
      case 'comment': return MessageSquare;
      case 'status_change': return Edit;
      case 'milestone': return Calendar;
      case 'reminder': return Clock;
      case 'info': return Info;
      case 'success': return CheckCircle;
      case 'warning': return AlertCircle;
      case 'error': return XCircle;
      default: return Bell;
    }
  }

  function formatRelativeTime(timestamp) {
    return homepageStore.formatRelativeTime(timestamp);
  }

  function calculateDaysUntil(targetDate) {
    return homepageStore.calculateDaysUntil(targetDate);
  }
</script>

<div class="min-h-screen max-w-7xl mx-auto px-6 pt-8 pb-6" style="background-color: var(--ds-surface);">
  <!-- Greeting Section with Animated Gradient Hero (only when NOT onboarding) -->
  {#if !isOnboarding}
    <div class="mb-8 relative overflow-hidden rounded-xl p-6 hero-section">
      <!-- Content -->
      <div class="relative z-10">
        <Text as="h1" size="2xl" weight="semibold">
          {greeting}, {authStore.currentUser?.first_name || 'there'}!
        </Text>
        <Text as="p" size="sm" variant="subtle">
          {currentDate}
        </Text>
      </div>
    </div>
  {/if}

  <!-- Onboarding Section -->
  {#if isOnboarding}
    <div class="max-w-2xl mx-auto">
      <DashboardOnboarding
        workspaceCount={totalWorkspaceCount}
        itemCount={totalItemCount}
        userName={authStore.currentUser?.first_name || 'there'}
        ondismiss={handleOnboardingDismiss}
      />
    </div>
  {:else}
    <DashboardOnboarding
      workspaceCount={totalWorkspaceCount}
      itemCount={totalItemCount}
      userName={authStore.currentUser?.first_name || 'there'}
      ondismiss={handleOnboardingDismiss}
    />
  {/if}

  <!-- Main content (only when NOT onboarding) -->
  {#if !isOnboarding}
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-5 mt-5">
    <!-- Main Content (Left 2/3) -->
    <div class="lg:col-span-2 space-y-5">
      <!-- What's New / Updates -->
      <div class="rounded-lg border hover-lift homepage-card" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="px-4 py-3 border-b flex items-center" style="border-color: var(--ds-border);">
          <Bell class="w-4 h-4 mr-2" style="color: var(--ds-text-subtle);" />
          <h2 class="text-sm font-semibold" style="color: var(--ds-text);">{t('dashboard.whatsNew')}</h2>
        </div>
        <div class="p-5 space-y-4">
          {#if notifications.length === 0}
            <div class="text-center py-6" style="color: var(--ds-text-subtle);">
              <Bell class="w-10 h-10 mx-auto mb-2 opacity-40" />
              <p class="text-sm">{t('dashboard.noNotifications')}</p>
            </div>
          {:else}
            {#each notifications as notification}
              <a
                href={notification.action_url || '#'}
                class="flex items-start p-3 rounded transition-colors cursor-pointer"
                class:hover:bg-gray-50={notification.action_url}
                style="border: 1px solid var(--ds-border);"
              >
                <div class="flex-shrink-0 mr-3 mt-1">
                  <svelte:component this={getNotificationIcon(notification.type)} class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                </div>
                <div class="flex-1 min-w-0">
                  <p class="text-base font-medium mb-1" style="color: var(--ds-text);">{notification.title}</p>
                  <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">{notification.message}</p>
                  <p class="text-xs" style="color: var(--ds-text-subtle);">{formatRelativeTime(notification.timestamp)}</p>
                </div>
                {#if !notification.read}
                  <div class="flex-shrink-0 ml-2">
                    <div class="w-2 h-2 rounded-full bg-blue-600"></div>
                  </div>
                {/if}
              </a>
            {/each}
          {/if}
        </div>
      </div>

      <!-- Your Activity -->
      <div class="rounded-lg border hover-lift homepage-card" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="px-4 py-3 border-b flex items-center" style="border-color: var(--ds-border);">
          <Eye class="w-4 h-4 mr-2" style="color: var(--ds-text-subtle);" />
          <h2 class="text-sm font-semibold" style="color: var(--ds-text);">{t('dashboard.yourActivity')}</h2>
        </div>

        <!-- Tabs -->
        <div class="px-4 py-2 flex gap-2" style="border-bottom: 1px solid var(--ds-border);">
          <button
            onclick={() => setActiveTab('viewed')}
            class="px-3 py-1.5 rounded-md text-xs font-medium transition-colors"
            style={activeTab === 'viewed'
              ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
              : 'color: var(--ds-text-subtle);'}
          >
            {t('dashboard.viewed')}
          </button>
          <button
            onclick={() => setActiveTab('edited')}
            class="px-3 py-1.5 rounded-md text-xs font-medium transition-colors"
            style={activeTab === 'edited'
              ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
              : 'color: var(--ds-text-subtle);'}
          >
            {t('dashboard.edited')}
          </button>
          <button
            onclick={() => setActiveTab('commented')}
            class="px-3 py-1.5 rounded-md text-xs font-medium transition-colors"
            style={activeTab === 'commented'
              ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
              : 'color: var(--ds-text-subtle);'}
          >
            {t('dashboard.commented')}
          </button>
        </div>

        <!-- Tab Content -->
        <div class="p-4">
          {#if activeTab === 'viewed'}
            {#if recentlyViewed.length === 0}
              <div class="text-center py-6" style="color: var(--ds-text-subtle);">
                <Eye class="w-10 h-10 mx-auto mb-2 opacity-40" />
                <p class="text-sm">{t('dashboard.noRecentlyViewed')}</p>
              </div>
            {:else}
              <div class="space-y-2">
                {#each recentlyViewed as item}
                  <WorkItemRow
                    {item}
                    href="/workspaces/{item.workspace_id}/items/{item.item_id}"
                    showIcon={false}
                    showPriority={true}
                    showStatus={true}
                    showTimestamp={true}
                    timestamp={item.last_activity}
                    formatTimestamp={formatRelativeTime}
                  />
                {/each}
              </div>
            {/if}
          {:else if activeTab === 'edited'}
            {#if recentlyEdited.length === 0}
              <div class="text-center py-6" style="color: var(--ds-text-subtle);">
                <Edit class="w-10 h-10 mx-auto mb-2 opacity-40" />
                <p class="text-sm">{t('dashboard.noRecentlyEdited')}</p>
              </div>
            {:else}
              <div class="space-y-2">
                {#each recentlyEdited as item}
                  <WorkItemRow
                    {item}
                    href="/workspaces/{item.workspace_id}/items/{item.item_id}"
                    showIcon={false}
                    showPriority={true}
                    showStatus={true}
                    showTimestamp={true}
                    timestamp={item.last_activity}
                    formatTimestamp={formatRelativeTime}
                  />
                {/each}
              </div>
            {/if}
          {:else if activeTab === 'commented'}
            {#if recentlyCommented.length === 0}
              <div class="text-center py-6" style="color: var(--ds-text-subtle);">
                <MessageSquare class="w-10 h-10 mx-auto mb-2 opacity-40" />
                <p class="text-sm">{t('dashboard.noRecentlyCommented')}</p>
              </div>
            {:else}
              <div class="space-y-2">
                {#each recentlyCommented as item}
                  <WorkItemRow
                    {item}
                    href="/workspaces/{item.workspace_id}/items/{item.item_id}"
                    showIcon={false}
                    showPriority={true}
                    showStatus={true}
                    showTimestamp={true}
                    timestamp={item.last_activity}
                    formatTimestamp={formatRelativeTime}
                  />
                {/each}
              </div>
            {/if}
          {/if}
        </div>
      </div>
    </div>

    <!-- Sidebar (Right 1/3) -->
    <div class="space-y-5">
      <!-- Command Palette Hint -->
      <div class="rounded-lg border p-4 hover-lift-sm homepage-card" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-start">
          <Search class="w-4 h-4 mr-3 mt-0.5 flex-shrink-0 text-blue-600" />
          <div>
            <p class="text-sm font-medium mb-1" style="color: var(--ds-text);">{t('dashboard.quickAccess')}</p>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              {t('dashboard.quickAccessHint', { shortcut: 'Space Space' })}
            </p>
          </div>
        </div>
      </div>

      <!-- Upcoming Milestones -->
      {#if upcomingMilestones && upcomingMilestones.length > 0}
        <div class="rounded-lg border hover-lift-sm homepage-card" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <div class="px-4 py-3 border-b flex items-center" style="border-color: var(--ds-border);">
            <Target class="w-4 h-4 mr-2" style="color: var(--ds-text-subtle);" />
            <h2 class="text-sm font-semibold" style="color: var(--ds-text);">{t('dashboard.upcomingMilestones')}</h2>
          </div>
          <div class="p-3 space-y-3">
            {#each upcomingMilestones as milestone}
              <div class="p-2 rounded border" style="border-color: var(--ds-border);">
                <div class="flex items-start justify-between mb-2">
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2 mb-0.5">
                      {#if milestone.category_color}
                        <ColorDot color={milestone.category_color} />
                      {/if}
                      <h3 class="text-sm font-medium truncate" style="color: var(--ds-text);">{milestone.milestone_name}</h3>
                    </div>
                    <p class="text-xs" style="color: var(--ds-text-subtle);">
                      {#if milestone.target_date}
                        {#if calculateDaysUntil(milestone.target_date) !== null}
                          {#if calculateDaysUntil(milestone.target_date) > 0}
                            {t('dashboard.dueIn', { days: calculateDaysUntil(milestone.target_date) })}
                          {:else if calculateDaysUntil(milestone.target_date) === 0}
                            {t('dashboard.dueToday')}
                          {:else}
                            {t('dashboard.overdue', { days: Math.abs(calculateDaysUntil(milestone.target_date)) })}
                          {/if}
                          {' · '}
                        {/if}
                      {/if}
                      {t('dashboard.done', { done: milestone.done_items, total: milestone.total_items })}
                    </p>
                  </div>
                  <div class="text-right ml-2 flex-shrink-0">
                    <div class="text-lg font-bold" style="color: var(--ds-text);">{Math.round(milestone.percent_complete)}%</div>
                  </div>
                </div>
                <!-- Progress Bar -->
                <div class="w-full h-1.5 rounded-full" style="background-color: var(--ds-border);">
                  <div
                    class="h-1.5 rounded-full transition-all"
                    style="width: {milestone.percent_complete}%; background-color: {milestone.category_color || 'rgb(59, 130, 246)'};"
                  ></div>
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Watched Items -->
      {#if watchedItems.length > 0}
        <div class="rounded-lg border hover-lift-sm homepage-card" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <div class="px-4 py-3 border-b flex items-center" style="border-color: var(--ds-border);">
            <Bookmark class="w-4 h-4 mr-2" style="color: var(--ds-text-subtle);" />
            <h2 class="text-sm font-semibold" style="color: var(--ds-text);">{t('dashboard.watching')}</h2>
          </div>
          <div class="p-3 space-y-2">
            {#each watchedItems as item}
              <ItemCard href="/workspaces/{item.workspace_id}/items/{item.item_id}" compact={true}>
                {#snippet children()}
                  <div class="flex items-center mb-2">
                    <span class="font-mono text-xs px-2 py-0.5 rounded mr-2" style="background-color: rgba(59, 130, 246, 0.1); color: var(--ds-text);">
                      {item.workspace_key}-{item.workspace_item_number}
                    </span>
                    {#if item.priority_id && item.priority_name}
                      <span class="inline-flex px-2 py-0.5 text-xs font-medium rounded-md"
                            style="background-color: {item.priority_color}20; color: {item.priority_color};">
                        {item.priority_name}
                      </span>
                    {/if}
                  </div>
                  <h4 class="text-sm font-medium mb-1" style="color: var(--ds-text);">{item.title}</h4>
                  <div class="flex items-center justify-between text-xs" style="color: var(--ds-text-subtle);">
                    <span>{item.status}</span>
                    <span>{formatRelativeTime(item.last_activity)}</span>
                  </div>
                {/snippet}
              </ItemCard>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Recent Workspaces -->
      <div class="rounded-lg border hover-lift-sm homepage-card" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="px-4 py-3 border-b flex items-center" style="border-color: var(--ds-border);">
          <Briefcase class="w-4 h-4 mr-2" style="color: var(--ds-text-subtle);" />
          <h2 class="text-sm font-semibold" style="color: var(--ds-text);">{t('dashboard.recentWorkspaces')}</h2>
        </div>
        <div class="p-3 space-y-2">
          {#if loading}
            <div class="p-4 text-center text-sm" style="color: var(--ds-text-subtle);">
              {t('common.loading')}
            </div>
          {:else if recentWorkspaces.length === 0}
            <div class="p-4 text-center text-sm" style="color: var(--ds-text-subtle);">
              {t('dashboard.noRecentWorkspaces')}
            </div>
          {:else}
            {#each recentWorkspaces as workspace}
              <ItemCard href="/workspaces/{workspace.workspace_id}" compact={true}>
                {#snippet children()}
                  <div class="flex items-start justify-between gap-2 mb-1">
                    <div class="flex items-center gap-2">
                      <!-- Workspace Icon -->
                      <div class="w-6 h-6 rounded flex items-center justify-center flex-shrink-0" style="background-color: {workspace.color || '#3b82f6'};">
                        <svelte:component this={workspaceIconMap[workspace.icon] || Grip} size={14} color="white" />
                      </div>
                      <div>
                        <h3 class="text-sm font-medium" style="color: var(--ds-text);">
                          {workspace.workspace_name}
                        </h3>
                        <p class="text-xs" style="color: var(--ds-text-subtle);">
                          {formatRelativeTime(workspace.last_visited)}
                        </p>
                      </div>
                    </div>
                    <span class="font-mono text-xs px-1.5 py-0.5 rounded flex-shrink-0" style="background-color: rgba(59, 130, 246, 0.1); color: var(--ds-text);">
                      {workspace.workspace_key}
                    </span>
                  </div>
                {/snippet}
              </ItemCard>
            {/each}
          {/if}
        </div>
      </div>
    </div>
  </div>
  {:else}
    <!-- During onboarding: just Quick Access below -->
    <div class="max-w-2xl mx-auto mt-6">
      <div class="rounded-lg border p-4 flex items-start" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <Search class="w-4 h-4 mr-3 mt-0.5 flex-shrink-0 text-blue-600" />
        <div>
          <p class="text-sm font-medium mb-1" style="color: var(--ds-text);">{t('dashboard.quickAccess')}</p>
          <p class="text-xs" style="color: var(--ds-text-subtle);">
            {t('dashboard.quickAccessHint', { shortcut: 'Space Space' })}
          </p>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  /* Hero section with animated gradient */
  .hero-section {
    animation: fade-up var(--duration-slow, 300ms) var(--ease-smooth, ease) forwards;
  }

  /* Card entrance animation - staggered */
  :global(.homepage-card) {
    animation: fade-up var(--duration-normal, 200ms) var(--ease-smooth, ease) forwards;
  }

  :global(.homepage-card:nth-child(1)) { animation-delay: 0ms; }
  :global(.homepage-card:nth-child(2)) { animation-delay: 50ms; }
  :global(.homepage-card:nth-child(3)) { animation-delay: 100ms; }
  :global(.homepage-card:nth-child(4)) { animation-delay: 150ms; }

  /* Progress bar animation */
  :global(.progress-bar-animated) {
    transition: width var(--duration-slow, 300ms) var(--ease-smooth, ease);
  }

  /* Reduced motion support */
  @media (prefers-reduced-motion: reduce) {
    .hero-section,
    :global(.homepage-card) {
      animation: none;
    }
  }
</style>
