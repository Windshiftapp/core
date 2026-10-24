<script>
  import { sla } from '../../api/sla.js';
  import { teams as teamsApi } from '../../api/teams.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Select from '../../components/Select.svelte';
  import Handshake from '@lucide/svelte/icons/handshake';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import AlertCircle from '@lucide/svelte/icons/alert-circle';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';

  let { workspaceId = null } = $props();

  let bindings = $state([]);
  let teams = $state([]);
  let calendars = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let showCreate = $state(false);
  let creating = $state(false);
  let createError = $state(null);
  let selectedTeamId = $state(null);

  const boundTeamIds = $derived(new Set(bindings.map((binding) => binding.team_id)));
  const teamOptions = $derived(
    teams
      .filter((team) => !boundTeamIds.has(team.id))
      .map((team) => ({ value: team.id, label: team.name }))
  );

  function calendarsForTeam(teamId) {
    return calendars.filter((calendar) => calendar.team_id === teamId).length;
  }

  async function load() {
    try {
      loading = true;
      error = null;
      const [bindingList, teamList, calendarList] = await Promise.all([
        sla.getWorkspaceTeamBindings(workspaceId),
        teamsApi.getAll().catch(() => []),
        sla.getAvailableCalendars(workspaceId).catch(() => []),
      ]);
      bindings = bindingList ?? [];
      teams = teamList ?? [];
      calendars = calendarList ?? [];
    } catch (err) {
      error = err?.message || t('workspaceSettings.serviceLevels.loadBindingsFailed');
    } finally {
      loading = false;
    }
  }

  let loadedWorkspaceId = null;
  $effect(() => {
    if (!workspaceId || workspaceId === loadedWorkspaceId) return;
    loadedWorkspaceId = workspaceId;
    void load();
  });

  function startCreate() {
    selectedTeamId = null;
    createError = null;
    showCreate = true;
  }

  async function create() {
    if (!selectedTeamId) {
      createError = t('workspaceSettings.serviceLevels.selectTeamRequired');
      return;
    }
    creating = true;
    createError = null;
    try {
      await sla.createWorkspaceTeamBinding(workspaceId, selectedTeamId);
      successToast(t('workspaceSettings.serviceLevels.bindingCreated'));
      showCreate = false;
      await load();
    } catch (err) {
      // A 403 means the caller is not also a team admin; both sides must consent.
      createError =
        err?.code === 'FORBIDDEN' || err?.status === 403
          ? t('workspaceSettings.serviceLevels.bindingDenied')
          : err?.message || t('workspaceSettings.serviceLevels.saveFailed');
    } finally {
      creating = false;
    }
  }

  async function remove(binding) {
    const ok = await confirm({
      title: t('workspaceSettings.serviceLevels.removeBinding'),
      message: t('workspaceSettings.serviceLevels.removeBindingConfirm', {
        name: binding.team_name,
      }),
      confirmText: t('common.remove'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await sla.deleteWorkspaceTeamBinding(workspaceId, binding.id);
      successToast(t('workspaceSettings.serviceLevels.bindingRemoved'));
      await load();
    } catch (err) {
      errorToast(
        err?.code === 'CONFLICT' || err?.status === 409
          ? t('workspaceSettings.serviceLevels.removeBindingBlocked')
          : err?.message || t('workspaceSettings.serviceLevels.saveFailed')
      );
    }
  }

  const columns = $derived([
    { key: 'team_name', label: t('workspaceSettings.serviceLevels.boundTeam') },
    {
      key: 'calendars',
      label: t('workspaceSettings.serviceLevels.availableCalendars'),
      width: '160px',
      slot: 'calendars',
    },
    { key: 'actions', label: t('common.actions') },
  ]);

  function buildActions(binding) {
    return [
      {
        id: 'remove',
        testid: `sla-binding-remove-${binding.id}`,
        type: 'regular',
        icon: Trash2,
        title: t('common.remove'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => remove(binding),
      },
    ];
  }
</script>

<PageHeader
  icon={Handshake}
  title={t('workspaceSettings.serviceLevels.bindingsTitle')}
  subtitle={t('workspaceSettings.serviceLevels.bindingsSubtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={loading || teamOptions.length === 0}
      dataTestid="sla-binding-add"
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('serviceLevels', 'add'), guard: () => !showCreate }}
    >
      {t('workspaceSettings.serviceLevels.createBinding')}
    </Button>
  {/snippet}
</PageHeader>

{#if error}
  <div class="mb-3 text-sm" style="color: var(--ds-text-danger)" data-testid="sla-bindings-error">
    {error}
  </div>
{/if}

<DataTable
  columns={columns}
  data={bindings}
  keyField="id"
  {loading}
  emptyMessage={t('workspaceSettings.serviceLevels.noBindings')}
  emptyIcon={Handshake}
  actionItems={buildActions}
  actionTriggerTestid={(binding) => `sla-binding-actions-${binding.id}`}
  rowAttrs={(binding) => ({ 'data-testid': `sla-binding-row-${binding.id}` })}
>
  {#snippet calendars(binding)}
    <span class="text-xs" style="color: var(--ds-text-subtle)">
      {calendarsForTeam(binding.team_id)}
    </span>
  {/snippet}
</DataTable>

<Modal
  isOpen={showCreate}
  onclose={() => (showCreate = false)}
  maxWidth="max-w-lg"
  onSubmit={create}
  submitDisabled={creating}
  dataTestid="sla-binding-dialog"
>
  {#snippet children()}
    <ModalHeader title={t('workspaceSettings.serviceLevels.createBinding')} showCloseButton={false} />
    <div class="px-6 py-4">
      {#if createError}
        <div class="mb-3 text-sm" style="color: var(--ds-text-danger)" data-testid="sla-binding-error">
          {createError}
        </div>
      {/if}
      <div class="mb-3 flex items-start gap-2 text-xs" style="color: var(--ds-text-subtle)">
        <AlertCircle class="w-4 h-4 flex-shrink-0" />
        <span>{t('workspaceSettings.serviceLevels.bindingConsentHelp')}</span>
      </div>
      <div class="form-group">
        <label for="sla-binding-team">{t('workspaceSettings.serviceLevels.boundTeam')}</label>
        <Select
          id="sla-binding-team"
          bind:value={selectedTeamId}
          options={teamOptions}
          placeholder={t('workspaceSettings.serviceLevels.selectTeam')}
        />
      </div>
    </div>
    <DialogFooter
      onCancel={() => (showCreate = false)}
      onConfirm={create}
      confirmLabel={t('workspaceSettings.serviceLevels.createBinding')}
      loading={creating}
      showKeyboardHint
      confirmTestid="sla-binding-submit"
    />
  {/snippet}
</Modal>
