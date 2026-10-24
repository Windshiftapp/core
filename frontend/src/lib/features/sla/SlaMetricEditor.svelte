<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Button from '../../components/Button.svelte';
  import TextField from '../../components/TextField.svelte';
  import SelectField from '../../components/SelectField.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import Panel from '../../components/Panel.svelte';
  import Text from '../../components/Text.svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import CategoryMultiSelect from '../../pickers/CategoryMultiSelect.svelte';
  import QlQueryBar from '../shared/QlQueryBar.svelte';
  import { sla } from '../../api/sla.js';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    isOpen = $bindable(false),
    metric = null,
    workspaceId = null,
    calendars = [],
    statuses = [],
    statusCategories = [],
    onSave,
    onClose,
  } = $props();

  const PHASES = ['start', 'pause', 'stop'];
  const DURATION_UNITS = [
    { value: 'm', labelKey: 'workspaceSettings.serviceLevels.units.minutes' },
    { value: 'h', labelKey: 'workspaceSettings.serviceLevels.units.hours' },
    { value: 'd', labelKey: 'workspaceSettings.serviceLevels.units.days' },
  ];

  const CONDITION_TYPES = [
    { value: 'created', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.created' },
    { value: 'status_entered', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.statusEntered' },
    { value: 'status_exited', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.statusExited' },
    { value: 'status_current', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.statusCurrent' },
    { value: 'status_category_entered', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.categoryEntered' },
    { value: 'status_category_exited', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.categoryExited' },
    { value: 'status_category_current', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.categoryCurrent' },
    { value: 'assignee_set', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.assigneeSet' },
    { value: 'resolution_set', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.resolutionSet' },
    { value: 'comment_by_customer', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.commentByCustomer' },
    { value: 'comment_by_agent', labelKey: 'workspaceSettings.serviceLevels.conditionTypes.commentByAgent' },
  ];

  const STATUS_TYPES = new Set(['status_entered', 'status_exited', 'status_current']);
  const CATEGORY_TYPES = new Set([
    'status_category_entered',
    'status_category_exited',
    'status_category_current',
  ]);

  function conditionTypeLabel(value) {
    const found = CONDITION_TYPES.find((type) => type.value === value);
    return found ? t(found.labelKey) : value;
  }

  function blankTarget() {
    return { value: 4, unit: 'h', calendar_id: calendars[0]?.id ?? 0 };
  }

  function blankGoal() {
    return { ql_query: '', error: null, targets: [blankTarget()] };
  }

  function blankForm() {
    return {
      name: '',
      display_format: 'time',
      position: 0,
      is_active: true,
      conditions: {
        start: [],
        pause: [],
        stop: [],
      },
      goals: [],
    };
  }

  let formData = $state(blankForm());
  let saving = $state(false);
  let error = $state(null);

  $effect(() => {
    if (!isOpen) return;
    error = null;
    formData = normalize(metric);
  });

  function normalize(source) {
    const base = blankForm();
    if (!source) return base;
    const conditions = { start: [], pause: [], stop: [] };
    for (const condition of source.conditions ?? []) {
      if (!PHASES.includes(condition.phase)) continue;
      conditions[condition.phase].push({
        condition_type: condition.condition_type,
        status_ids: condition.config?.status_ids ?? [],
        category_ids: condition.config?.category_ids ?? [],
      });
    }
    const goals = (source.goals ?? []).map((goal) => ({
      ql_query: goal.ql_query ?? '',
      error: null,
      // One target per goal: priority-specific durations are separate goals.
      targets: (goal.targets ?? []).slice(0, 1).map((target) => ({
        ...msToUnit(target.target_ms),
        calendar_id: target.calendar_id,
      })),
    }));
    return {
      name: source.name ?? '',
      display_format: source.display_format ?? 'time',
      position: source.position ?? 0,
      is_active: source.is_active !== false,
      conditions,
      goals,
    };
  }

  function msToUnit(ms) {
    if (!ms || ms <= 0) return { value: 4, unit: 'h' };
    if (ms % 86400000 === 0) return { value: ms / 86400000, unit: 'd' };
    if (ms % 3600000 === 0) return { value: ms / 3600000, unit: 'h' };
    return { value: ms / 60000, unit: 'm' };
  }

  function unitToMs(value, unit) {
    const amount = Number(value) || 0;
    const factor = unit === 'd' ? 86400000 : unit === 'h' ? 3600000 : 60000;
    return Math.round(amount * factor);
  }

  function addCondition(phase) {
    formData.conditions[phase] = [
      ...formData.conditions[phase],
      { condition_type: 'status_entered', status_ids: [], category_ids: [] },
    ];
  }

  function removeCondition(phase, index) {
    formData.conditions[phase] = formData.conditions[phase].filter((_, i) => i !== index);
  }

  function addGoal() {
    formData.goals = [...formData.goals, blankGoal()];
  }

  function removeGoal(index) {
    formData.goals = formData.goals.filter((_, i) => i !== index);
  }

  function moveGoal(index, delta) {
    const next = index + delta;
    if (next < 0 || next >= formData.goals.length) return;
    const goals = [...formData.goals];
    [goals[index], goals[next]] = [goals[next], goals[index]];
    formData.goals = goals;
  }

  function buildPayload() {
    const conditions = [];
    for (const phase of PHASES) {
      formData.conditions[phase].forEach((condition, position) => {
        const config = {};
        if (STATUS_TYPES.has(condition.condition_type)) {
          config.status_ids = condition.status_ids;
        } else if (CATEGORY_TYPES.has(condition.condition_type)) {
          config.category_ids = condition.category_ids;
        }
        conditions.push({
          phase,
          position,
          condition_type: condition.condition_type,
          config,
        });
      });
    }
    const goals = formData.goals.map((goal, position) => ({
      position,
      ql_query: goal.ql_query.trim(),
      import_status: 'native',
      targets: goal.targets.slice(0, 1).map((target, targetPosition) => ({
        position: targetPosition,
        target_ms: unitToMs(target.value, target.unit),
        calendar_id: Number(target.calendar_id),
      })),
    }));
    return {
      name: formData.name.trim(),
      display_format: formData.display_format,
      position: Number(formData.position) || 0,
      is_active: formData.is_active,
      import_status: 'native',
      conditions,
      goals,
    };
  }

  async function validate() {
    if (!formData.name.trim()) return t('workspaceSettings.serviceLevels.nameRequired');
    if (formData.goals.length === 0) return t('workspaceSettings.serviceLevels.goalRequired');
    for (const goal of formData.goals) {
      goal.error = null;
      const target = goal.targets[0];
      if (!target) return t('workspaceSettings.serviceLevels.targetRequired');
      if (!target.calendar_id) return t('workspaceSettings.serviceLevels.targetCalendarRequired');
      if (unitToMs(target.value, target.unit) <= 0) {
        return t('workspaceSettings.serviceLevels.targetDurationRequired');
      }
    }
    // A blank query matches every item; only non-blank queries need parsing.
    for (const goal of formData.goals) {
      if (!goal.ql_query.trim()) continue;
      const queryError = await sla.validateGoalQuery(workspaceId, goal.ql_query);
      if (queryError) {
        goal.error = queryError;
        return t('workspaceSettings.serviceLevels.goalQueryInvalid');
      }
    }
    return null;
  }

  async function submit() {
    const validationError = await validate();
    if (validationError) {
      error = validationError;
      return;
    }
    saving = true;
    error = null;
    try {
      await onSave(buildPayload());
      isOpen = false;
    } catch (err) {
      error = err?.message || t('workspaceSettings.serviceLevels.saveFailed');
    } finally {
      saving = false;
    }
  }
</script>

<Modal
  {isOpen}
  onclose={onClose}
  maxWidth="max-w-4xl"
  onSubmit={submit}
  submitDisabled={saving}
  dataTestid="sla-metric-dialog"
>
  {#snippet children()}
    <ModalHeader
      title={metric
        ? t('workspaceSettings.serviceLevels.editMetric')
        : t('workspaceSettings.serviceLevels.createMetric')}
      showCloseButton={false}
    />

    <div class="px-6 py-4 max-h-[70vh] overflow-y-auto">
      {#if error}
        <div class="mb-4" data-testid="sla-metric-error">
          <AlertBox variant="error" message={error} />
        </div>
      {/if}

      <TextField
        class="mb-4"
        label={t('common.name')}
        labelColor="default"
        id="sla-metric-name"
        type="text"
        bind:value={formData.name}
        dataTestid="sla-metric-name"
      />

      <div class="grid grid-cols-2 gap-4 mb-4">
        <SelectField
          label={t('workspaceSettings.serviceLevels.displayFormat')}
          labelColor="default"
          id="sla-metric-format"
          bind:value={formData.display_format}
          options={[
            { value: 'time', label: t('workspaceSettings.serviceLevels.displayFormats.time') },
            { value: 'due_date', label: t('workspaceSettings.serviceLevels.displayFormats.dueDate') },
          ]}
        />
        <TextField
          label={t('workspaceSettings.serviceLevels.order')}
          labelColor="default"
          id="sla-metric-position"
          type="number"
          bind:value={formData.position}
          dataTestid="sla-metric-position"
        />
      </div>

      <div class="mb-4">
        <Toggle
          bind:checked={formData.is_active}
          label={t('workspaceSettings.serviceLevels.metricActive')}
          dataTestid="sla-metric-active"
        />
      </div>

      <fieldset class="mb-4" data-testid="sla-metric-conditions">
        <legend>
          <Text as="span" weight="semibold">{t('workspaceSettings.serviceLevels.conditions')}</Text>
        </legend>
        <Text as="p" variant="subtle" size="xs" class="mb-2">
          {t('workspaceSettings.serviceLevels.conditionsHelp')}
        </Text>
        {#each PHASES as phase (phase)}
          <div class="mb-3" data-testid="sla-metric-phase-{phase}">
            <Text as="p" size="sm" weight="medium" class="mb-1">
              {t(`workspaceSettings.serviceLevels.phases.${phase}`)}
            </Text>
            {#each formData.conditions[phase] as condition, index (index)}
              <div class="flex flex-wrap items-center gap-2 py-1">
                <Select
                  id="sla-metric-{phase}-{index}-type"
                  bind:value={condition.condition_type}
                  options={CONDITION_TYPES.map((type) => ({
                    value: type.value,
                    label: conditionTypeLabel(type.value),
                  }))}
                />
                {#if STATUS_TYPES.has(condition.condition_type)}
                  <div class="flex-1 min-w-48">
                    <CategoryMultiSelect
                      categories={statuses}
                      bind:selectedIds={condition.status_ids}
                      placeholder={t('workspaceSettings.serviceLevels.selectStatuses')}
                    />
                  </div>
                {:else if CATEGORY_TYPES.has(condition.condition_type)}
                  <div class="flex-1 min-w-48">
                    <CategoryMultiSelect
                      categories={statusCategories}
                      bind:selectedIds={condition.category_ids}
                      placeholder={t('workspaceSettings.serviceLevels.selectCategories')}
                    />
                  </div>
                {/if}
                <Button
                  variant="default"
                  size="small"
                  icon={Trash2}
                  onclick={() => removeCondition(phase, index)}
                  title={t('common.remove')}
                  dataTestid="sla-metric-condition-remove-{phase}-{index}"
                />
              </div>
            {/each}
            <Button
              variant="default"
              size="small"
              icon={Plus}
              onclick={() => addCondition(phase)}
              dataTestid="sla-metric-condition-add-{phase}"
            >
              {t('workspaceSettings.serviceLevels.addCondition')}
            </Button>
          </div>
        {/each}
      </fieldset>

      <fieldset class="mb-4" data-testid="sla-metric-goals">
        <legend>
          <Text as="span" weight="semibold">{t('workspaceSettings.serviceLevels.goals')}</Text>
        </legend>
        {#each formData.goals as goal, goalIndex (goalIndex)}
          <Panel padding="compact" class="mb-3">
            <div class="flex items-start gap-2">
              <div class="flex-1">
                <QlQueryBar
                  query={goal.ql_query}
                  mode="raw"
                  compact
                  editorTestId="sla-goal-query-{goalIndex}"
                  placeholder={t('workspaceSettings.serviceLevels.goalQueryPlaceholder')}
                  onquerychange={(value) => {
                    goal.ql_query = value;
                    goal.error = null;
                  }}
                />
                {#if goal.error}
                  <div class="mt-1" data-testid="sla-goal-error-{goalIndex}">
                    <Text as="p" variant="danger" size="xs">{goal.error}</Text>
                  </div>
                {:else}
                  <Text as="p" variant="subtle" size="xs" class="mt-1">
                    {t('workspaceSettings.serviceLevels.goalQueryOptionalHint')}
                  </Text>
                {/if}
              </div>
              <Button
                variant="default"
                size="small"
                icon={ArrowUp}
                disabled={goalIndex === 0}
                onclick={() => moveGoal(goalIndex, -1)}
                title={t('common.moveUp')}
                dataTestid="sla-goal-up-{goalIndex}"
              />
              <Button
                variant="default"
                size="small"
                icon={ArrowDown}
                disabled={goalIndex === formData.goals.length - 1}
                onclick={() => moveGoal(goalIndex, 1)}
                title={t('common.moveDown')}
                dataTestid="sla-goal-down-{goalIndex}"
              />
              <Button
                variant="default"
                size="small"
                icon={Trash2}
                onclick={() => removeGoal(goalIndex)}
                title={t('common.remove')}
                dataTestid="sla-goal-remove-{goalIndex}"
              />
            </div>
            {#if goal.targets[0]}
              <div class="flex flex-wrap items-center gap-2 py-1">
                <Input
                  type="number"
                  min="1"
                  bind:value={goal.targets[0].value}
                  ariaLabel={t('workspaceSettings.serviceLevels.duration')}
                  dataTestid="sla-target-value-{goalIndex}"
                />
                <Select
                  id="sla-target-unit-{goalIndex}"
                  bind:value={goal.targets[0].unit}
                  options={DURATION_UNITS.map((unit) => ({ value: unit.value, label: t(unit.labelKey) }))}
                />
                <Select
                  id="sla-target-calendar-{goalIndex}"
                  class="min-w-40"
                  menuWidth="16rem"
                  bind:value={goal.targets[0].calendar_id}
                  options={calendars.map((calendar) => ({ value: calendar.id, label: calendar.name }))}
                />
              </div>
            {/if}
          </Panel>
        {/each}
        <Button variant="default" size="small" icon={Plus} onclick={addGoal} dataTestid="sla-goal-add">
          {t('workspaceSettings.serviceLevels.addGoal')}
        </Button>
      </fieldset>
    </div>

    <DialogFooter
      onCancel={onClose}
      onConfirm={submit}
      confirmLabel={metric ? t('common.save') : t('common.create')}
      loading={saving}
      showKeyboardHint
      confirmTestid="sla-metric-submit"
    />
  {/snippet}
</Modal>
