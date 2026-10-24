<script>
  import { sla } from '../api/sla.js';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import WorkingCalendarEditor from '../features/sla/WorkingCalendarEditor.svelte';
  import Clock from '@lucide/svelte/icons/clock';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Edit from '@lucide/svelte/icons/edit';
  import Eye from '@lucide/svelte/icons/eye';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  let { team, canEdit = false } = $props();

  let calendars = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let showEditor = $state(false);
  let editing = $state(null);
  let impact = $state(null);
  let impactLoading = $state(false);

  async function loadCalendars() {
    try {
      loading = true;
      error = null;
      calendars = (await sla.getTeamCalendars(team.id)) ?? [];
    } catch (err) {
      error = err?.message || t('teams.serviceHours.loadFailed');
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (!team?.id) return;
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
      await sla.updateTeamCalendar(team.id, editing.id, payload);
      successToast(t('teams.serviceHours.updated'));
    } else {
      await sla.createTeamCalendar(team.id, payload);
      successToast(t('teams.serviceHours.created'));
    }
    await loadCalendars();
  }

  async function remove(calendar) {
    const ok = await confirm({
      title: t('teams.serviceHours.deleteCalendar'),
      message: t('teams.serviceHours.deleteConfirm', { name: calendar.name }),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await sla.deleteTeamCalendar(team.id, calendar.id);
      successToast(t('teams.serviceHours.deleted'));
      await loadCalendars();
    } catch (err) {
      errorToast(err?.message || t('teams.serviceHours.deleteBlocked'));
    }
  }

  async function previewImpact(calendar) {
    impact = { calendar, workspaces: [] };
    impactLoading = true;
    try {
      impact = { calendar, ...(await sla.getTeamCalendarImpact(team.id, calendar.id)) };
    } catch (err) {
      errorToast(err?.message || t('teams.serviceHours.impactFailed'));
      impact = null;
    } finally {
      impactLoading = false;
    }
  }

  const columns = $derived([
    { key: 'name', label: t('common.name') },
    { key: 'timezone', label: t('teams.serviceHours.timezone') },
    { key: 'is_default', label: t('common.default'), width: '100px', slot: 'is_default' },
    { key: 'actions', label: t('common.actions') },
  ]);

  function buildActions(calendar) {
    if (!canEdit) return [];
    return [
      {
        id: 'impact',
        testid: `team-calendar-impact-${calendar.id}`,
        type: 'regular',
        icon: Eye,
        title: t('teams.serviceHours.previewImpact'),
        onClick: () => previewImpact(calendar),
      },
      {
        id: 'edit',
        testid: `team-calendar-edit-${calendar.id}`,
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        onClick: () => startEdit(calendar),
      },
      {
        id: 'delete',
        testid: `team-calendar-delete-${calendar.id}`,
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

<div class="space-y-4" data-testid="team-service-hours">
  <div class="flex items-center justify-between">
    <div>
      <h3 class="text-base font-semibold" style="color: var(--ds-text)">
        {t('teams.serviceHours.title')}
      </h3>
      <p class="text-sm" style="color: var(--ds-text-subtle)">
        {t('teams.serviceHours.subtitle')}
      </p>
    </div>
    {#if canEdit}
      <Button
        variant="primary"
        icon={Plus}
        onclick={startCreate}
        dataTestid="team-calendar-add"
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('serviceLevels', 'add'), guard: () => !showEditor }}
      >
        {t('teams.serviceHours.createCalendar')}
      </Button>
    {/if}
  </div>

  {#if error}
    <div class="text-sm" style="color: var(--ds-text-danger)" data-testid="team-service-hours-error">
      {error}
    </div>
  {/if}

  <DataTable
    columns={columns}
    data={calendars}
    keyField="id"
    {loading}
    emptyMessage={t('teams.serviceHours.noCalendars')}
    emptyIcon={Clock}
    actionItems={buildActions}
    actionTriggerTestid={(calendar) => `team-calendar-actions-${calendar.id}`}
    rowAttrs={(calendar) => ({ 'data-testid': `team-calendar-row-${calendar.id}` })}
  >
    {#snippet is_default(calendar)}
      {#if calendar.is_default}
        <Lozenge color="green" text={t('common.default')} />
      {/if}
    {/snippet}
  </DataTable>
</div>

<WorkingCalendarEditor
  bind:isOpen={showEditor}
  calendar={editing}
  onSave={save}
  onClose={() => {
    showEditor = false;
    editing = null;
  }}
/>

<Modal
  isOpen={impact !== null}
  onclose={() => (impact = null)}
  maxWidth="max-w-2xl"
  dataTestid="team-calendar-impact-dialog"
>
  {#snippet children()}
    <ModalHeader title={t('teams.serviceHours.impactTitle', { name: impact?.calendar?.name ?? '' })} showCloseButton={false} />
    <div class="px-6 py-4 max-h-[60vh] overflow-y-auto">
      {#if impactLoading}
        <div style="color: var(--ds-text-subtle)">{t('common.loading')}</div>
      {:else if !impact?.workspaces?.length}
        <div style="color: var(--ds-text-subtle)">{t('teams.serviceHours.noImpact')}</div>
      {:else}
        {#each impact.workspaces as workspace (workspace.workspace_id)}
          <div class="mb-4 rounded border p-3" style="border-color: var(--ds-border)">
            <div class="font-medium" data-testid="team-calendar-impact-workspace-{workspace.workspace_id}">
              {workspace.workspace_name}
            </div>
            <div class="text-xs mt-1" style="color: var(--ds-text-subtle)">
              {t('teams.serviceHours.impactCycles', { count: workspace.ongoing_cycles })}
            </div>
            {#if workspace.goal_targets?.length}
              <ul class="mt-2 text-sm list-disc list-inside">
                {#each workspace.goal_targets as target (target.target_id)}
                  <li>{target.metric_name} · {target.target_ms} ms</li>
                {/each}
              </ul>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
    <div class="px-6 py-4 border-t flex justify-end" style="border-color: var(--ds-border)">
      <Button variant="default" onclick={() => (impact = null)}>{t('common.close')}</Button>
    </div>
  {/snippet}
</Modal>
