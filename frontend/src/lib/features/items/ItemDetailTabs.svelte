<script>
  import { MessageSquare, Clock, Play, Info, History, Edit, Trash2, MoreHorizontal } from '@lucide/svelte';
  import Button from '../../components/Button.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Comments from '../items/Comments.svelte';
  import ItemHistory from '../items/ItemHistory.svelte';
  import { confirm } from '../../composables/useConfirm.js';
  import { formatDateTimeLocale, formatDateShort } from '../../utils/dateFormatter.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { toHotkeyString, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import Badge from '../../components/Badge.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import DataTable from '../../components/DataTable.svelte';

  let {
    item,
    workspace,
    tab = 'comments',
    moduleSettings = { time_tracking_enabled: true },
    timeWorklogs = [],
    activeTimer = null,
    statusOptions = [],
    onswitchtab = undefined,
    onstarttimer = undefined,
    onlogtime = undefined,
    oneditworklog = undefined,
    ondeleteworklog = undefined,
  } = $props();

  function getStatusName(statusId) {
    if (!statusId) return '';
    const status = statusOptions.find(s => s.id === statusId);
    return status?.name || '';
  }

  let commentCount = $state(0);

  function switchTab(newTab) {
    onswitchtab?.({ tab: newTab });
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
    onstarttimer?.();
  }

  function handleLogTime() {
    onlogtime?.();
  }
  
  function handleCommentsLoaded(data) {
    commentCount = data.count;
  }

  function handleEditWorklog(worklog) {
    oneditworklog?.(worklog);
  }

  async function handleDeleteWorklog(worklog) {
    const ok = await confirm({
      title: t('items.deleteTimeEntry'),
      message: t('items.deleteTimeEntryConfirm'),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!ok) return;
    ondeleteworklog?.(worklog);
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
        color: 'var(--ds-text-danger)',
        onClick: () => handleDeleteWorklog(worklog)
      }
    ];
  }

  const worklogColumns = [
    { key: 'date', label: t('common.date'), render: (w) => formatDateShort(new Date(w.date * 1000)), textColor: 'var(--ds-text-subtle)' },
    { key: 'description', label: t('common.description'), render: (w) => w.description || t('items.noDescription') },
    { key: 'user_name', label: t('common.user'), render: (w) => w.user_name || '—' },
    { key: 'start_time', label: t('time.start'), render: (w) => w.start_time ? new Date(w.start_time * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '—', textColor: 'var(--ds-text-subtle)' },
    { key: 'end_time', label: t('time.end'), render: (w) => w.end_time ? new Date(w.end_time * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '—', textColor: 'var(--ds-text-subtle)' },
    { key: 'duration_minutes', label: t('time.duration'), render: (w) => `${Math.floor(w.duration_minutes / 60)}h ${w.duration_minutes % 60}m` },
    { key: 'project_name', label: t('common.project'), textColor: 'var(--ds-text-subtle)' },
    { key: 'actions', label: '', width: 'w-12' },
  ];
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
          <Badge variant="neutral" size="xs">{commentCount}</Badge>
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
            <Badge variant="neutral" size="xs">{timeWorklogs.length}</Badge>
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
        <Comments itemId={item.id} isPersonalWorkspace={workspace?.is_personal} isPortalRequest={!!item.request_type_id} enableInternalComments={workspace?.internal_comments_enabled} onCommentsLoaded={handleCommentsLoaded} />
      {:else if tab === 'details'}
        <div class="space-y-4">
          <div class="grid grid-cols-2 gap-6">
            <div>
              <h4 class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('items.created')}</h4>
              <p class="text-sm" style="color: var(--ds-text);">{formatDateTimeLocale(item.created_at) || '-'}</p>
              {#if item.creator_name}
                <DescriptionText>{t('items.by')} {item.creator_name}</DescriptionText>
              {/if}
            </div>
            <div>
              <h4 class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('items.lastUpdated')}</h4>
              <p class="text-sm" style="color: var(--ds-text);">{formatDateTimeLocale(item.updated_at) || '-'}</p>
              {#if item.updated_by_name}
                <DescriptionText>{t('items.by')} {item.updated_by_name}</DescriptionText>
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
                  title={t('items.logTimeTitle')}
                  keyboardHint={getShortcutDisplay('itemDetail', 'logTime')}
                  hotkeyConfig={{ key: toHotkeyString('itemDetail', 'logTime'), guard: () => tab === 'time' && moduleSettings?.time_tracking_enabled }}
                >
                  {t('items.logTime')}
                </Button>
              </div>
            </div>
            <DataTable
              columns={worklogColumns}
              data={timeWorklogs}
              keyField="id"
              actionItems={buildWorklogDropdownItems}
            />
          </div>
        {:else}
          <EmptyState icon={Clock} title={t('items.noTimeLogged')}>
            {#snippet action()}
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
                title={t('items.logTimeTitle')}
                keyboardHint={getShortcutDisplay('itemDetail', 'logTime')}
                hotkeyConfig={{ key: toHotkeyString('itemDetail', 'logTime'), guard: () => tab === 'time' && moduleSettings?.time_tracking_enabled }}
              >
                {t('items.logTime')}
              </Button>
            </div>
            {/snippet}
          </EmptyState>
        {/if}
      {:else if tab === 'history'}
        <ItemHistory itemId={item.id} />
      {/if}
    </div>
  </div>
</div>

