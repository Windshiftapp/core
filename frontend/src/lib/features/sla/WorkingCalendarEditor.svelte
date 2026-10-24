<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';
  import Text from '../../components/Text.svelte';
  import TextField from '../../components/TextField.svelte';
  import TextareaField from '../../components/TextareaField.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import TimezonePicker from '../../pickers/TimezonePicker.svelte';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { t } from '../../stores/i18n.svelte.js';
  import { sla } from '../../api/sla.js';

  let {
    isOpen = $bindable(false),
    calendar = null,
    workspaceId = null,
    onSave,
    onClose,
  } = $props();

  const weekdays = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'];

  function blankIntervals() {
    return Object.fromEntries(weekdays.map((day) => [day, []]));
  }

  function blankForm() {
    return {
      name: '',
      description: '',
      timezone: 'UTC',
      is_default: false,
      weekly_intervals: blankIntervals(),
      holidays: [],
    };
  }

  let formData = $state(blankForm());
  let saving = $state(false);
  let error = $state(null);
  let coverage = $state(null);
  let coverageLoading = $state(false);
  let ongoingImpact = $state(null);

  $effect(() => {
    if (!isOpen) return;
    error = null;
    formData = normalize(calendar);
  });

  // Config-time coverage preview: only workspace-owned calendars have a
  // workspace-relative reference to compare against.
  $effect(() => {
    const id = calendar?.id;
    const ownedByWorkspace = !!calendar?.workspace_id;
    if (!isOpen || !id || !workspaceId || !ownedByWorkspace || !sla?.getCoveragePreview) {
      coverage = null;
      return;
    }
    let active = true;
    coverageLoading = true;
    sla
      .getCoveragePreview(workspaceId, id)
      .then((value) => {
        if (active) coverage = value;
      })
      .catch(() => {
        if (active) coverage = null;
      })
      .finally(() => {
        if (active) coverageLoading = false;
      });
    return () => {
      active = false;
    };
  });

  // Show the recalculation scope before the admin applies an edit to ongoing
  // cycles: a workspace calendar is scoped to this workspace, a team calendar
  // to every workspace bound to the team.
  $effect(() => {
    const id = calendar?.id;
    if (!isOpen || !id) {
      ongoingImpact = null;
      return;
    }
    const teamId = calendar?.team_id;
    let request;
    if (teamId) {
      request = sla?.getTeamCalendarImpact?.(teamId, id);
    } else if (workspaceId) {
      request = sla?.getCalendarImpact?.(workspaceId, id);
    }
    if (!request) {
      ongoingImpact = null;
      return;
    }
    let active = true;
    request
      .then((value) => {
        if (active) ongoingImpact = value;
      })
      .catch(() => {
        if (active) ongoingImpact = null;
      });
    return () => {
      active = false;
    };
  });

  const impactWorkspaces = $derived(ongoingImpact?.workspaces?.length ?? 0);
  const impactCycles = $derived(
    (ongoingImpact?.workspaces ?? []).reduce((sum, workspace) => sum + (workspace.ongoing_cycles ?? 0), 0)
  );

  function formatWeekly(ms) {
    if (ms == null || ms === 0) return '0m';
    const hours = Math.round(ms / 3600000);
    return `${hours}h`;
  }

  function normalize(source) {
    const base = blankForm();
    if (!source) return base;
    const weekly = blankIntervals();
    for (const day of weekdays) {
      const intervals = source.weekly_intervals?.[day];
      if (Array.isArray(intervals)) {
        weekly[day] = intervals.map((interval) => ({
          start: interval.start || '09:00',
          end: interval.end || '17:00',
        }));
      }
    }
    return {
      name: source.name || '',
      description: source.description || '',
      timezone: source.timezone || 'UTC',
      is_default: !!source.is_default,
      weekly_intervals: weekly,
      holidays: Array.isArray(source.holidays)
        ? source.holidays.map((holiday) => ({
            date: holiday.date || '',
            month_day: holiday.month_day || '',
            recurring: !!holiday.recurring,
            name: holiday.name || '',
          }))
        : [],
    };
  }

  function addInterval(day) {
    formData.weekly_intervals[day] = [
      ...formData.weekly_intervals[day],
      { start: '09:00', end: '17:00' },
    ];
  }

  function removeInterval(day, index) {
    formData.weekly_intervals[day] = formData.weekly_intervals[day].filter(
      (_, i) => i !== index
    );
  }

  function addHoliday() {
    formData.holidays = [...formData.holidays, { date: '', month_day: '', recurring: false, name: '' }];
  }

  function removeHoliday(index) {
    formData.holidays = formData.holidays.filter((_, i) => i !== index);
  }

  function buildPayload(applyToOngoing = false) {
    const weekly = {};
    for (const day of weekdays) {
      const intervals = formData.weekly_intervals[day].filter(
        (interval) => interval.start && interval.end && interval.end > interval.start
      );
      if (intervals.length > 0) weekly[day] = intervals;
    }
    return {
      name: formData.name.trim(),
      description: formData.description,
      timezone: formData.timezone,
      is_default: formData.is_default,
      apply_to_ongoing: applyToOngoing,
      weekly_intervals: weekly,
      holidays: formData.holidays
        .filter((holiday) => holiday.recurring ? holiday.month_day : holiday.date)
        .map((holiday) =>
          holiday.recurring
            ? { month_day: holiday.month_day, recurring: true, name: holiday.name }
            : { date: holiday.date, name: holiday.name }
        ),
    };
  }

  async function persist(applyToOngoing) {
    saving = true;
    error = null;
    try {
      await onSave(buildPayload(applyToOngoing));
      isOpen = false;
    } catch (err) {
      error = err?.message || t('workspaceSettings.serviceLevels.saveFailed');
    } finally {
      saving = false;
    }
  }

  function submit() {
    return persist(false);
  }

  function submitAndRecalculate() {
    return persist(true);
  }
</script>

<Modal
  {isOpen}
  onclose={onClose}
  maxWidth="max-w-3xl"
  onSubmit={submit}
  submitDisabled={saving}
  dataTestid="sla-calendar-dialog"
>
  {#snippet children()}
    <ModalHeader
      title={calendar
        ? t('workspaceSettings.serviceLevels.editCalendar')
        : t('workspaceSettings.serviceLevels.createCalendar')}
      showCloseButton={false}
    />

    <div class="px-6 py-4 max-h-[70vh] overflow-y-auto">
      {#if error}
        <div class="mb-4 text-sm" style="color: var(--ds-text-danger)" data-testid="sla-calendar-error">
          {error}
        </div>
      {/if}

      <TextField
        class="mb-4"
        label={t('common.name')}
        labelColor="default"
        id="sla-calendar-name"
        type="text"
        bind:value={formData.name}
        placeholder={t('workspaceSettings.serviceLevels.namePlaceholder')}
        dataTestid="sla-calendar-name"
      />

      <TextareaField
        class="mb-4"
        label={t('common.description')}
        labelColor="default"
        id="sla-calendar-description"
        bind:value={formData.description}
        rows={2}
        placeholder={t('placeholders.optionalDescription')}
        dataTestid="sla-calendar-description"
      />

      <div class="mb-6">
        <Label for="sla-calendar-timezone" color="default" class="mb-2">
          {t('workspaceSettings.serviceLevels.timezone')}
        </Label>
        <TimezonePicker
          id="sla-calendar-timezone"
          bind:value={formData.timezone}
          inputTestid="sla-calendar-timezone"
          menuTestid="sla-calendar-timezone-listbox"
          optionTestid={() => 'sla-calendar-timezone-option'}
        />
      </div>

      <fieldset class="mb-6" data-testid="sla-calendar-weekly">
        <legend class="mb-3">
          <Text as="span" weight="semibold">{t('workspaceSettings.serviceLevels.weeklyHours')}</Text>
        </legend>
        <div class="space-y-3">
          {#each weekdays as day (day)}
            <div class="flex items-start gap-3" data-testid="sla-calendar-day-{day}">
              <div class="w-24 shrink-0 pt-2">
                <Label
                  color="subtle"
                  for={formData.weekly_intervals[day].length ? `sla-calendar-${day}-start-0` : null}
                >
                  {t(`workspaceSettings.serviceLevels.days.${day}`)}
                </Label>
              </div>
              <div class="flex-1 space-y-2">
                {#each formData.weekly_intervals[day] as interval, index}
                  <div class="flex items-center gap-2">
                    <div class="w-32">
                      <Input
                        type="time"
                        id="sla-calendar-{day}-start-{index}"
                        bind:value={interval.start}
                        ariaLabel={t('workspaceSettings.serviceLevels.startTime')}
                        dataTestid="sla-calendar-{day}-start-{index}"
                      />
                    </div>
                    <span style="color: var(--ds-text-subtle)">–</span>
                    <div class="w-32">
                      <Input
                        type="time"
                        id="sla-calendar-{day}-end-{index}"
                        bind:value={interval.end}
                        ariaLabel={t('workspaceSettings.serviceLevels.endTime')}
                        dataTestid="sla-calendar-{day}-end-{index}"
                      />
                    </div>
                    <Button
                      variant="default"
                      size="small"
                      icon={Trash2}
                      onclick={() => removeInterval(day, index)}
                      title={t('common.remove')}
                      dataTestid="sla-calendar-{day}-remove-{index}"
                    />
                  </div>
                {/each}
                <Button
                  variant="default"
                  size="small"
                  icon={Plus}
                  onclick={() => addInterval(day)}
                  dataTestid="sla-calendar-{day}-add"
                >
                  {t('workspaceSettings.serviceLevels.addInterval')}
                </Button>
              </div>
            </div>
          {/each}
        </div>
      </fieldset>

      <fieldset class="mb-6" data-testid="sla-calendar-holidays">
        <legend class="mb-3">
          <Text as="span" weight="semibold">{t('workspaceSettings.serviceLevels.holidays')}</Text>
        </legend>
        {#each formData.holidays as holiday, index}
          <div class="flex flex-wrap items-center gap-2 py-1">
            {#if holiday.recurring}
              <div class="w-28">
                <Input
                  type="text"
                  bind:value={holiday.month_day}
                  placeholder="MM-DD"
                  ariaLabel={t('workspaceSettings.serviceLevels.monthDay')}
                  dataTestid="sla-calendar-holiday-monthday-{index}"
                />
              </div>
            {:else}
              <div class="w-44">
                <Input
                  type="date"
                  bind:value={holiday.date}
                  ariaLabel={t('workspaceSettings.serviceLevels.date')}
                  dataTestid="sla-calendar-holiday-date-{index}"
                />
              </div>
            {/if}
            <div class="min-w-40 flex-1">
              <Input
                type="text"
                bind:value={holiday.name}
                placeholder={t('workspaceSettings.serviceLevels.holidayName')}
                ariaLabel={t('workspaceSettings.serviceLevels.holidayName')}
                dataTestid="sla-calendar-holiday-name-{index}"
              />
            </div>
            <Toggle
              bind:checked={holiday.recurring}
              label={t('workspaceSettings.serviceLevels.recurring')}
              dataTestid="sla-calendar-holiday-recurring-{index}"
            />
            <Button
              variant="default"
              size="small"
              icon={Trash2}
              onclick={() => removeHoliday(index)}
              title={t('common.remove')}
              dataTestid="sla-calendar-holiday-remove-{index}"
            />
          </div>
        {/each}
        <Button
          variant="default"
          size="small"
          icon={Plus}
          onclick={addHoliday}
          dataTestid="sla-calendar-holiday-add"
        >
          {t('workspaceSettings.serviceLevels.addHoliday')}
        </Button>
      </fieldset>

      <Toggle
        bind:checked={formData.is_default}
        label={t('workspaceSettings.serviceLevels.setDefault')}
        dataTestid="sla-calendar-default"
      />

      {#if calendar}
        <div
          class="mt-4 rounded border p-3"
          style="border-color: var(--ds-border)"
          data-testid="sla-calendar-ongoing"
        >
          <Text as="p" variant="subtle" size="xs">
            {calendar.team_id
              ? t('workspaceSettings.serviceLevels.applyToOngoingTeamHint')
              : t('workspaceSettings.serviceLevels.applyToOngoingHint')}
          </Text>
          {#if ongoingImpact}
            <div class="mt-1" data-testid="sla-calendar-ongoing-impact">
              <Text as="p" variant="subtle" size="xs">
                {impactWorkspaces > 0
                  ? t('workspaceSettings.serviceLevels.applyToOngoingImpact', {
                      cycles: impactCycles,
                      workspaces: impactWorkspaces,
                    })
                  : t('workspaceSettings.serviceLevels.applyToOngoingNoImpact')}
              </Text>
            </div>
          {/if}
        </div>
      {/if}

      {#if coverage && coverage.reference === 'team_service_hours'}
        <div
          class="mt-4 rounded border p-3"
          style="border-color: var(--ds-border)"
          data-testid="sla-calendar-coverage"
        >
          <Text as="p" size="sm" weight="medium" class="mb-1">
            {t('workspaceSettings.serviceLevels.coveragePreview')}
          </Text>
          <Text as="p" variant="subtle" size="xs">
            {t('workspaceSettings.serviceLevels.coverageWeekly', {
              sla: formatWeekly(coverage.sla_weekly_ms),
              team: formatWeekly(coverage.team_weekly_ms),
              overlap: formatWeekly(coverage.overlap_weekly_ms),
            })}
          </Text>
          {#if coverage.discrepancy && coverage.discrepancy !== 'aligned'}
            <Text as="p" variant="subtle" size="xs" class="mt-1">
              {t(`workspaceSettings.serviceLevels.coverageDiscrepancies.${coverage.discrepancy}`)}
            </Text>
          {/if}
        </div>
      {/if}
    </div>

    <DialogFooter
      onCancel={onClose}
      onConfirm={submit}
      confirmLabel={calendar ? t('common.save') : t('common.create')}
      loading={saving}
      showKeyboardHint
      confirmTestid="sla-calendar-submit"
    >
      {#snippet secondaryAction()}
        {#if calendar}
          <Button
            variant="default"
            onclick={submitAndRecalculate}
            disabled={saving}
            dataTestid="sla-calendar-submit-recalculate"
          >
            {t('workspaceSettings.serviceLevels.saveAndRecalculate')}
          </Button>
        {/if}
      {/snippet}
    </DialogFooter>
  {/snippet}
</Modal>
