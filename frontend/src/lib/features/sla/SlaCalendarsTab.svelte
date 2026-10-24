<script>
  import { sla } from '../../api/sla.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import WorkingCalendarEditor from './WorkingCalendarEditor.svelte';
  import Calendar from '@lucide/svelte/icons/calendar';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Edit from '@lucide/svelte/icons/edit';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';

  let { workspaceId = null } = $props();

  let calendars = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let showEditor = $state(false);
  let editing = $state(null);

  async function loadCalendars() {
    try {
      loading = true;
      error = null;
      calendars = await sla.getAvailableCalendars(workspaceId);
    } catch (err) {
      error = err?.message || t('workspaceSettings.serviceLevels.loadFailed');
    } finally {
      loading = false;
    }
  }

  // WorkspaceSettings keeps one component mounted across /settings/* routes,
  // so follow the prop instead of loading once on mount.
  let loadedWorkspaceId = null;
  $effect(() => {
    if (!workspaceId || workspaceId === loadedWorkspaceId) return;
    loadedWorkspaceId = workspaceId;
    void loadCalendars();
  });

  function startCreate() {
    editing = null;
    showEditor = true;
  }

  function startEdit(calendar) {
    editing = calendar;
    showEditor = true;
  }

  async function save(payload) {
    if (editing) {
      await sla.updateCalendar(workspaceId, editing.id, payload);
      successToast(t('workspaceSettings.serviceLevels.calendarUpdated'));
    } else {
      await sla.createCalendar(workspaceId, payload);
      successToast(t('workspaceSettings.serviceLevels.calendarCreated'));
    }
    await loadCalendars();
  }

  async function remove(calendar) {
    const ok = await confirm({
      title: t('workspaceSettings.serviceLevels.deleteCalendar'),
      message: t('workspaceSettings.serviceLevels.deleteCalendarConfirm', { name: calendar.name }),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await sla.deleteCalendar(workspaceId, calendar.id);
      successToast(t('workspaceSettings.serviceLevels.calendarDeleted'));
      await loadCalendars();
    } catch (err) {
      errorToast(err?.message || t('workspaceSettings.serviceLevels.deleteCalendarBlocked'));
    }
  }

  const columns = $derived([
    { key: 'name', label: t('common.name') },
    { key: 'timezone', label: t('workspaceSettings.serviceLevels.timezone') },
    { key: 'owner', label: t('workspaceSettings.serviceLevels.owner'), width: '140px', slot: 'owner' },
    { key: 'is_default', label: t('common.default'), width: '100px', slot: 'is_default' },
    { key: 'actions', label: t('common.actions') },
  ]);

  function ownerLabel(calendar) {
    return calendar.team_id
      ? calendar.team_name || t('workspaceSettings.serviceLevels.sharedByTeam')
      : t('workspaceSettings.serviceLevels.ownedByWorkspace');
  }

  function buildActions(calendar) {
    // Team-owned calendars are managed by the team's admins elsewhere.
    if (calendar.team_id) return [];
    return [
      {
        id: 'edit',
        testid: `sla-calendar-edit-${calendar.id}`,
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        onClick: () => startEdit(calendar),
      },
      {
        id: 'delete',
        testid: `sla-calendar-delete-${calendar.id}`,
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => remove(calendar),
      },
    ];
  }
</script>

<PageHeader
  icon={Calendar}
  title={t('workspaceSettings.serviceLevels.calendarsTitle')}
  subtitle={t('workspaceSettings.serviceLevels.calendarsSubtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={loading}
      dataTestid="sla-calendar-add"
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('serviceLevels', 'add'), guard: () => !showEditor }}
    >
      {t('workspaceSettings.serviceLevels.createCalendar')}
    </Button>
  {/snippet}
</PageHeader>

{#if error}
  <div class="mb-3 text-sm" style="color: var(--ds-text-danger)" data-testid="sla-calendars-error">
    {error}
  </div>
{/if}

<DataTable
  columns={columns}
  data={calendars}
  keyField="id"
  {loading}
  emptyMessage={t('workspaceSettings.serviceLevels.noCalendars')}
  emptyIcon={Calendar}
  actionItems={buildActions}
  actionTriggerTestid={(calendar) => `sla-calendar-actions-${calendar.id}`}
  rowAttrs={(calendar) => ({ 'data-testid': `sla-calendar-row-${calendar.id}` })}
>
  {#snippet owner(calendar)}
    <span class="text-xs" style="color: var(--ds-text-subtle)">{ownerLabel(calendar)}</span>
  {/snippet}
  {#snippet is_default(calendar)}
    {#if calendar.is_default}
      <Lozenge color="green" text={t('common.default')} />
    {/if}
  {/snippet}
</DataTable>

<WorkingCalendarEditor
  bind:isOpen={showEditor}
  calendar={editing}
  {workspaceId}
  onSave={save}
  onClose={() => {
    showEditor = false;
    editing = null;
  }}
/>
