<script>
  import { MessageSquare, Clock, Play, Info, History } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import Comments from '../items/Comments.svelte';
  import ItemHistory from '../items/ItemHistory.svelte';
  import { createEventDispatcher } from 'svelte';
  import { formatDateTimeLocale } from '../../utils/dateFormatter.js';

  const dispatch = createEventDispatcher();

  export let item;
  export let workspace;
  export let tab = 'comments';
  export let moduleSettings = { time_tracking_enabled: true };
  export let timeWorklogs = [];
  export let activeTimer = null;
  export let statusOptions = [];

  function getStatusName(statusId) {
    if (!statusId) return '';
    const status = statusOptions.find(s => s.id === statusId);
    return status?.name || '';
  }

  let commentCount = 0;

  function switchTab(newTab) {
    dispatch('switch-tab', { tab: newTab });
  }
  
  function getDefaultProjectForTimeLogging() {
    // Priority order for project resolution:
    // 1. Item-specific time tracking project override
    if (item?.time_project_id) {
      return item.time_project_id;
    }
    // 2. Effective project (inherited or direct project_id)
    if (item?.effective_project_id) {
      return item.effective_project_id;
    }
    // 3. Workspace default time tracking project
    if (workspace?.time_project_id) {
      return workspace.time_project_id;
    }
    return null;
  }
  
  function handleStartTimer() {
    dispatch('start-timer');
  }

  function handleLogTime() {
    dispatch('log-time');
  }
  
  function handleCommentsLoaded(event) {
    commentCount = event.detail.count;
  }
</script>

<div class="mt-6">
  <div>
    <!-- Tab Navigation -->
    <div class="flex border-b" style="border-color: var(--ds-border);">
      <button
        class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative"
        style="{tab === 'comments' ? 'background-color: var(--ds-surface-raised); color: var(--ds-interactive); margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : 'color: var(--ds-text-subtle);'}"
        onclick={() => switchTab('comments')}
      >
        <MessageSquare class="w-4 h-4" />
        Comments
        {#if commentCount > 0}
          <span class="text-xs px-2 py-0.5 rounded-full" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">{commentCount}</span>
        {/if}
      </button>
      {#if moduleSettings.time_tracking_enabled}
        <button
          class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative"
          style="{tab === 'time' ? 'background-color: var(--ds-surface-raised); color: var(--ds-interactive); margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : 'color: var(--ds-text-subtle);'}"
          onclick={() => switchTab('time')}
        >
          <Clock class="w-4 h-4" />
          Time Tracking
          {#if timeWorklogs && timeWorklogs.length > 0}
            <span class="text-xs px-2 py-0.5 rounded-full" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">{timeWorklogs.length}</span>
          {/if}
        </button>
      {/if}
      <button
        class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative"
        style="{tab === 'details' ? 'background-color: var(--ds-surface-raised); color: var(--ds-interactive); margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : 'color: var(--ds-text-subtle);'}"
        onclick={() => switchTab('details')}
      >
        <Info class="w-4 h-4" />
        Details
      </button>
      <button
        class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative"
        style="{tab === 'history' ? 'background-color: var(--ds-surface-raised); color: var(--ds-interactive); margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : 'color: var(--ds-text-subtle);'}"
        onclick={() => switchTab('history')}
      >
        <History class="w-4 h-4" />
        History
      </button>
    </div>

    <!-- Tab Content -->
    <div class="pt-4">
      {#if tab === 'comments'}
        <Comments itemId={item.id} isPersonalWorkspace={workspace?.is_personal} oncommentsLoaded={handleCommentsLoaded} />
      {:else if tab === 'details'}
        <div class="space-y-4">
          <div class="grid grid-cols-2 gap-6">
            <div>
              <h4 class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Created</h4>
              <p class="text-sm" style="color: var(--ds-text);">{formatDateTimeLocale(item.created_at) || '-'}</p>
              {#if item.creator_name}
                <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">by {item.creator_name}</p>
              {/if}
            </div>
            <div>
              <h4 class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Last Updated</h4>
              <p class="text-sm" style="color: var(--ds-text);">{formatDateTimeLocale(item.updated_at) || '-'}</p>
              {#if item.updated_by_name}
                <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">by {item.updated_by_name}</p>
              {/if}
            </div>
          </div>

          <!-- Additional metadata can be added here -->
          <div class="pt-2">
            <h4 class="text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">Work Item Information</h4>
            <div class="space-y-2">
              <div class="flex justify-between">
                <span class="text-xs" style="color: var(--ds-text-subtle);">ID:</span>
                <span class="text-xs font-mono" style="color: var(--ds-text);">{workspace?.key || 'WORK'}-{item.id}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-xs" style="color: var(--ds-text-subtle);">Type:</span>
                <span class="text-xs" style="color: var(--ds-text);">{item.item_type_name || 'Work Item'}</span>
              </div>
              {#if item.parent_id}
                <div class="flex justify-between">
                  <span class="text-xs" style="color: var(--ds-text-subtle);">Parent:</span>
                  <span class="text-xs" style="color: var(--ds-text);">{workspace?.key || 'WORK'}-{item.parent_id}</span>
                </div>
              {/if}
            </div>
          </div>
        </div>
      {:else if tab === 'time' && moduleSettings.time_tracking_enabled}
        {#if !getDefaultProjectForTimeLogging()}
          <div class="text-center py-8">
            <div class="text-sm mb-2" style="color: #ca8a04;">No project configured for time tracking</div>
            <div class="text-xs" style="color: var(--ds-text-subtle);">Set a default project in workspace or item settings to log time</div>
          </div>
        {:else}
          <!-- Time Entries List -->
          {#if timeWorklogs && timeWorklogs.length > 0}
            <div class="space-y-3">
              <div class="flex items-center justify-between">
                <h4 class="text-sm font-medium" style="color: var(--ds-text);">Time Entries ({timeWorklogs.length})</h4>
                <div class="flex gap-2">
                  {#if !activeTimer && getDefaultProjectForTimeLogging()}
                    <Button
                      variant="primary"
                      icon={Play}
                      onclick={handleStartTimer}
                      size="small"
                      title="Start tracking time for this work item"
                    >
                      Start Timer
                    </Button>
                  {/if}
                  <Button
                    variant="default"
                    size="small"
                    onclick={handleLogTime}
                    disabled={!getDefaultProjectForTimeLogging()}
                    title="Manually log time worked on this item"
                  >
                    Log Time
                  </Button>
                </div>
              </div>
              <div class="space-y-2">
                {#each timeWorklogs as worklog}
                  <div class="flex justify-between items-center p-3 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
                    <div class="flex-1">
                      <div class="text-sm font-medium" style="color: var(--ds-text);">
                        {worklog.description}
                      </div>
                      <div class="text-xs" style="color: var(--ds-text-subtle);">
                        {new Date(worklog.date * 1000).toLocaleDateString()} • 
                        {Math.floor(worklog.duration_minutes / 60)}h {worklog.duration_minutes % 60}m • 
                        {worklog.project_name}
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
            </div>
          {:else}
            <div class="text-center py-8">
              <div class="text-sm mb-4" style="color: var(--ds-text-subtle);">No time logged yet</div>
              <div class="flex justify-center gap-2">
                {#if !activeTimer && getDefaultProjectForTimeLogging()}
                  <Button
                    variant="primary"
                    icon={Play}
                    onclick={handleStartTimer}
                    size="small"
                    title="Start tracking time for this work item"
                  >
                    Start Timer
                  </Button>
                {/if}
                <Button
                  variant="default"
                  size="small"
                  onclick={handleLogTime}
                  disabled={!getDefaultProjectForTimeLogging()}
                  title="Manually log time worked on this item"
                >
                  Log Time
                </Button>
              </div>
            </div>
          {/if}
        {/if}
      {:else if tab === 'history'}
        <ItemHistory itemId={item.id} />
      {/if}
    </div>
  </div>
</div>