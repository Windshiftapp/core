<script>
  import { onMount } from 'svelte';
  import { IconPlus, IconBellRinging, IconCircle, IconEdit, IconTrash } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import Button from '../components/Button.svelte';
  import Card from '../components/Card.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import ScheduleEditor from './ScheduleEditor.svelte';
  import LayerEditor from './LayerEditor.svelte';
  import OverrideEditor from './OverrideEditor.svelte';

  let { team, canEdit } = $props();

  let schedules = $state([]);
  let currentByScheduleId = $state(new Map());
  let loading = $state(false);
  let error = $state('');

  let showScheduleEditor = $state(false);
  let editingSchedule = $state(null);
  let showOverrideEditor = $state(false);
  let overrideScheduleId = $state(null);
  let expandedScheduleId = $state(null);

  async function loadSchedules() {
    loading = true;
    try {
      schedules = (await api.onCallSchedules.listForTeam(team.id)) || [];
      // Hydrate full schedule (with layers) for each
      const detailed = await Promise.all(
        schedules.map(async (s) => {
          try {
            return await api.onCallSchedules.get(s.id);
          } catch {
            return s;
          }
        })
      );
      schedules = detailed;
      // Load current on-call for each
      const currentEntries = await Promise.all(
        schedules.map(async (s) => {
          try {
            const current = await api.onCallSchedules.getCurrent(s.id);
            return [s.id, current];
          } catch {
            return [s.id, null];
          }
        })
      );
      currentByScheduleId = new Map(currentEntries);
      error = '';
    } catch (err) {
      error = err.message || t('teams.oncall.failedToLoad');
    } finally {
      loading = false;
    }
  }

  function startCreate() {
    editingSchedule = null;
    showScheduleEditor = true;
  }

  function startEdit(schedule) {
    editingSchedule = schedule;
    showScheduleEditor = true;
  }

  async function deleteSchedule(schedule) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('teams.oncall.confirmDeleteSchedule', { name: schedule.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await api.onCallSchedules.delete(schedule.id);
      successToast(t('teams.oncall.scheduleDeleted'));
      await loadSchedules();
    } catch (err) {
      errorToast(err.message || t('teams.oncall.failedToDeleteSchedule'));
    }
  }

  function openOverrideEditor(scheduleId) {
    overrideScheduleId = scheduleId;
    showOverrideEditor = true;
  }

  async function onScheduleSaved() {
    showScheduleEditor = false;
    editingSchedule = null;
    await loadSchedules();
  }

  async function onLayersChanged() {
    await loadSchedules();
  }

  async function onOverrideCreated() {
    showOverrideEditor = false;
    overrideScheduleId = null;
    await loadSchedules();
  }

  onMount(() => {
    loadSchedules();
  });
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h3 class="text-base font-medium" style="color: var(--ds-text)">
      {t('teams.oncall.schedules')}
    </h3>
    {#if canEdit}
      <Button
        variant="primary"
        size="sm"
        icon={IconPlus}
        onclick={startCreate}
        keyboardHint="A"
        hotkeyConfig={{
          key: toHotkeyString('teamsOnCall', 'addSchedule'),
          guard: () => !showScheduleEditor && !showOverrideEditor,
        }}
        dataTestid="add-schedule"
      >
        {t('teams.oncall.addSchedule')}
      </Button>
    {/if}
  </div>

  {#if error}
    <AlertBox message={error} />
  {/if}

  {#if loading}
    <div class="text-center py-8" style="color: var(--ds-text-subtle)">
      {t('teams.loading')}
    </div>
  {:else if schedules.length === 0}
    <EmptyState icon={IconBellRinging} message={t('teams.oncall.noSchedules')} />
  {:else}
    <div class="space-y-4">
      {#each schedules as schedule (schedule.id)}
        {@const current = currentByScheduleId.get(schedule.id)}
        <Card>
          <div class="p-4 space-y-4" data-testid="schedule-row" data-schedule-id={schedule.id}>
            <div class="flex items-start justify-between">
              <div>
                <div class="flex items-center gap-2">
                  <h4 class="text-sm font-medium" style="color: var(--ds-text)">
                    {schedule.name}
                  </h4>
                  <Lozenge
                    color={schedule.is_active ? 'green' : 'gray'}
                    text={schedule.is_active ? t('teams.active') : t('teams.inactive')}
                  />
                  <span class="text-xs" style="color: var(--ds-text-subtle)">
                    {schedule.timezone}
                  </span>
                </div>
                {#if schedule.description}
                  <p class="text-sm mt-1" style="color: var(--ds-text-subtle)">
                    {schedule.description}
                  </p>
                {/if}
              </div>
              {#if canEdit}
                <div class="flex gap-2">
                  <Button variant="ghost" size="sm" icon={IconEdit} onclick={() => startEdit(schedule)}>
                    {t('common.edit')}
                  </Button>
                  <Button variant="ghost" size="sm" icon={IconTrash} onclick={() => deleteSchedule(schedule)}>
                    {t('common.delete')}
                  </Button>
                </div>
              {/if}
            </div>

            <!-- Currently on-call -->
            <div class="rounded p-3" style="background-color: var(--ds-background-neutral);" data-testid="current-oncall">
              <div class="text-xs uppercase tracking-wide mb-1" style="color: var(--ds-text-subtle)">
                {t('teams.oncall.currentlyOnCall')}
              </div>
              {#if current && current.on_call && current.on_call.length > 0}
                <div class="flex flex-wrap gap-2">
                  {#each current.on_call as entry}
                    <div class="flex items-center gap-2 text-sm" style="color: var(--ds-text)" data-testid="current-oncall-user">
                      <IconCircle class="w-3 h-3" style="color: var(--ds-accent-green);" fill="currentColor" />
                      {entry.user_name}
                      {#if entry.layer_name}
                        <span class="text-xs" style="color: var(--ds-text-subtle)">({entry.layer_name})</span>
                      {/if}
                      {#if entry.is_override}
                        <Lozenge color="orange" text={t('teams.oncall.override')} />
                      {/if}
                    </div>
                  {/each}
                </div>
              {:else}
                <div class="text-sm" style="color: var(--ds-text-subtle)">
                  {t('teams.oncall.nobody')}
                </div>
              {/if}
            </div>

            <LayerEditor
              schedule={schedule}
              {canEdit}
              onChange={onLayersChanged}
            />

            <div class="flex justify-end">
              {#if canEdit}
                <Button
                  variant="ghost"
                  size="sm"
                  icon={IconPlus}
                  onclick={() => openOverrideEditor(schedule.id)}
                  dataTestid="override-create"
                >
                  {t('teams.oncall.addOverride')}
                </Button>
              {/if}
            </div>
          </div>
        </Card>
      {/each}
    </div>
  {/if}
</div>

{#if showScheduleEditor}
  <ScheduleEditor
    teamId={team.id}
    schedule={editingSchedule}
    onSaved={onScheduleSaved}
    onCancel={() => { showScheduleEditor = false; editingSchedule = null; }}
  />
{/if}

{#if showOverrideEditor && overrideScheduleId}
  <OverrideEditor
    scheduleId={overrideScheduleId}
    onSaved={onOverrideCreated}
    onCancel={() => { showOverrideEditor = false; overrideScheduleId = null; }}
  />
{/if}
