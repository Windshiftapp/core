<script>
  import { MessageSquare, Clock, Play, Info, History, Edit, Trash2, MoreHorizontal } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Comments from '../items/Comments.svelte';
  import ItemHistory from '../items/ItemHistory.svelte';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import { createEventDispatcher } from 'svelte';
  import { formatDateTimeLocale, formatDateShort } from '../../utils/dateFormatter.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { toHotkeyString, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';

  const dispatch = createEventDispatcher();

  // Delete confirmation state
  let showDeleteConfirmation = false;
  let worklogToDelete = null;

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
  
  function handleCommentsLoaded(data) {
    commentCount = data.count;
  }

  function handleEditWorklog(worklog) {
    dispatch('edit-worklog', worklog);
  }

  function handleDeleteWorklog(worklog) {
    worklogToDelete = worklog;
    showDeleteConfirmation = true;
  }

  function buildWorklogDropdownItems(worklog) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        onClick: () => handleEditWorklog(worklog)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        onClick: () => handleDeleteWorklog(worklog)
      }
    ];
  }

  function confirmDeleteWorklog() {
    if (worklogToDelete) {
      dispatch('delete-worklog', worklogToDelete);
    }
    showDeleteConfirmation = false;
    worklogToDelete = null;
  }

  function cancelDeleteWorklog() {
    showDeleteConfirmation = false;
    worklogToDelete = null;
  }
</script>

<div class="mt-6">
  <div>
    <!-- Tab Navigation -->
    <div class="flex border-b" style="border-color: var(--ds-border);">
      <button
        class="flex items-center gap-2 pl-0 pr-4 py-3 text-sm font-medium transition-all relative"
        style="{tab === 'comments' ? 'background-color: var(--ds-surface-raised); color: var(--ds-interactive); margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : 'color: var(--ds-text-subtle);'}"
        onclick={() => switchTab('comments')}
      >
        <MessageSquare class="w-4 h-4" />
        {t('items.comments')}
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
          {t('items.timeTracking')}
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
        {t('items.details')}
      </button>
      <button
        class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative"
        style="{tab === 'history' ? 'background-color: var(--ds-surface-raised); color: var(--ds-interactive); margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : 'color: var(--ds-text-subtle);'}"
        onclick={() => switchTab('history')}
      >
        <History class="w-4 h-4" />
        {t('items.history')}
      </button>
    </div>

    <!-- Tab Content -->
    <div class="pt-6">
      {#if tab === 'comments'}
        <Comments itemId={item.id} isPersonalWorkspace={workspace?.is_personal} isPortalRequest={!!item.request_type_id} onCommentsLoaded={handleCommentsLoaded} />
      {:else if tab === 'details'}
        <div class="space-y-4">
          <div class="grid grid-cols-2 gap-6">
            <div>
              <h4 class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('items.created')}</h4>
              <p class="text-sm" style="color: var(--ds-text);">{formatDateTimeLocale(item.created_at) || '-'}</p>
              {#if item.creator_name}
                <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('items.by')} {item.creator_name}</p>
              {/if}
            </div>
            <div>
              <h4 class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('items.lastUpdated')}</h4>
              <p class="text-sm" style="color: var(--ds-text);">{formatDateTimeLocale(item.updated_at) || '-'}</p>
              {#if item.updated_by_name}
                <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('items.by')} {item.updated_by_name}</p>
              {/if}
            </div>
          </div>

          <!-- Additional metadata can be added here -->
          <div class="pt-2">
            <h4 class="text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('items.workItemInformation')}</h4>
            <div class="space-y-2">
              <div class="flex justify-between">
                <span class="text-xs" style="color: var(--ds-text-subtle);">{t('items.id')}</span>
                <span class="text-xs font-mono" style="color: var(--ds-text);">{workspace?.key || 'WORK'}-{item.id}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-xs" style="color: var(--ds-text-subtle);">{t('items.type')}</span>
                <span class="text-xs" style="color: var(--ds-text);">{item.item_type_name || t('items.workItem')}</span>
              </div>
              {#if item.parent_id}
                <div class="flex justify-between">
                  <span class="text-xs" style="color: var(--ds-text-subtle);">{t('items.parent')}</span>
                  <span class="text-xs" style="color: var(--ds-text);">{workspace?.key || 'WORK'}-{item.parent_id}</span>
                </div>
              {/if}
            </div>
          </div>
        </div>
      {:else if tab === 'time' && moduleSettings.time_tracking_enabled}
        {#if !getDefaultProjectForTimeLogging()}
          <div class="text-center py-8">
            <div class="text-sm mb-2" style="color: #ca8a04;">{t('items.noProjectConfigured')}</div>
            <div class="text-xs" style="color: var(--ds-text-subtle);">{t('items.setDefaultProject')}</div>
          </div>
        {:else}
          <!-- Time Entries List -->
          {#if timeWorklogs && timeWorklogs.length > 0}
            <div class="space-y-3">
              <div class="flex items-center justify-between">
                <h4 class="text-sm font-medium" style="color: var(--ds-text);">{t('items.timeEntries')} ({timeWorklogs.length})</h4>
                <div class="flex gap-2">
                  {#if !activeTimer && getDefaultProjectForTimeLogging()}
                    <Button
                      variant="primary"
                      icon={Play}
                      onclick={handleStartTimer}
                      size="small"
                      title={t('items.startTimerTitle')}
                      keyboardHint={getShortcutDisplay('itemDetail', 'startTimer')}
                      hotkeyConfig={{ key: toHotkeyString('itemDetail', 'startTimer'), guard: () => tab === 'time' && moduleSettings?.time_tracking_enabled && !!getDefaultProjectForTimeLogging() }}
                    >
                      {t('items.startTimer')}
                    </Button>
                  {/if}
                  <Button
                    variant="default"
                    size="small"
                    onclick={handleLogTime}
                    disabled={!getDefaultProjectForTimeLogging()}
                    title={t('items.logTimeTitle')}
                    keyboardHint={getShortcutDisplay('itemDetail', 'logTime')}
                    hotkeyConfig={{ key: toHotkeyString('itemDetail', 'logTime'), guard: () => tab === 'time' && moduleSettings?.time_tracking_enabled && !!getDefaultProjectForTimeLogging() }}
                  >
                    {t('items.logTime')}
                  </Button>
                </div>
              </div>
              <div class="space-y-2">
                {#each timeWorklogs as worklog}
                  <div class="flex justify-between items-center p-3 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
                    <div class="flex-1">
                      <div class="text-sm font-medium" style="color: var(--ds-text);">
                        {worklog.description || t('items.noDescription')}
                      </div>
                      <div class="text-xs" style="color: var(--ds-text-subtle);">
                        {formatDateShort(new Date(worklog.date * 1000))} •
                        {Math.floor(worklog.duration_minutes / 60)}h {worklog.duration_minutes % 60}m •
                        {worklog.project_name}
                      </div>
                    </div>
                    <div class="ml-2">
                      <DropdownMenu
                        items={buildWorklogDropdownItems(worklog)}
                        triggerIcon={MoreHorizontal}
                        showChevron={false}
                        iconOnly={true}
                        triggerClass="p-1.5 rounded-md transition-colors duration-150"
                        triggerStyle="color: var(--ds-text-subtle);"
                      />
                    </div>
                  </div>
                {/each}
              </div>
            </div>
          {:else}
            <div class="text-center py-8">
              <div class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('items.noTimeLogged')}</div>
              <div class="flex justify-center gap-2">
                {#if !activeTimer && getDefaultProjectForTimeLogging()}
                  <Button
                    variant="primary"
                    icon={Play}
                    onclick={handleStartTimer}
                    size="small"
                    title={t('items.startTimerTitle')}
                    keyboardHint={getShortcutDisplay('itemDetail', 'startTimer')}
                    hotkeyConfig={{ key: toHotkeyString('itemDetail', 'startTimer'), guard: () => tab === 'time' && moduleSettings?.time_tracking_enabled && !!getDefaultProjectForTimeLogging() }}
                  >
                    {t('items.startTimer')}
                  </Button>
                {/if}
                <Button
                  variant="default"
                  size="small"
                  onclick={handleLogTime}
                  disabled={!getDefaultProjectForTimeLogging()}
                  title={t('items.logTimeTitle')}
                  keyboardHint={getShortcutDisplay('itemDetail', 'logTime')}
                  hotkeyConfig={{ key: toHotkeyString('itemDetail', 'logTime'), guard: () => tab === 'time' && moduleSettings?.time_tracking_enabled && !!getDefaultProjectForTimeLogging() }}
                >
                  {t('items.logTime')}
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

<!-- Delete Time Entry Confirmation Dialog -->
<ConfirmDialog
  bind:show={showDeleteConfirmation}
  title={t('items.deleteTimeEntry')}
  message={t('items.deleteTimeEntryConfirm')}
  confirmText={t('common.delete')}
  cancelText={t('common.cancel')}
  variant="danger"
  onconfirm={confirmDeleteWorklog}
  oncancel={cancelDeleteWorklog}
/>